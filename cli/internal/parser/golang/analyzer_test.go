package golang

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hungnm98/seshat-cli/internal/parser"
	"github.com/hungnm98/seshat-cli/pkg/model"
)

func TestAnalyzerExtractsSymbolsAndCalls(t *testing.T) {
	analyzer := New()
	batch, err := analyzer.Analyze(context.Background(), parser.Input{
		ProjectID:     "payment-service",
		RepoPath:      "../../../testdata/go_sample",
		IncludePaths:  []string{"cmd", "internal"},
		ExcludePaths:  []string{"vendor"},
		CommitSHA:     "abc123",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if len(batch.Symbols) < 4 {
		t.Fatalf("expected symbols to be extracted, got %d", len(batch.Symbols))
	}
	found := false
	for _, relation := range batch.Relations {
		if relation.Type == "calls" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected at least one call relation")
	}
}

func TestAnalyzerParallelMatchesSequentialOutput(t *testing.T) {
	analyzer := New()
	base := parser.Input{
		ProjectID:     "payment-service",
		RepoPath:      "../../../testdata/go_sample",
		IncludePaths:  []string{"cmd", "internal"},
		ExcludePaths:  []string{"vendor"},
		CommitSHA:     "abc123",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
		Parallelism:   1,
	}
	sequential, err := analyzer.Analyze(context.Background(), base)
	if err != nil {
		t.Fatalf("sequential Analyze returned error: %v", err)
	}
	base.Parallelism = 4
	parallel, err := analyzer.Analyze(context.Background(), base)
	if err != nil {
		t.Fatalf("parallel Analyze returned error: %v", err)
	}
	if !reflect.DeepEqual(fileIDs(sequential), fileIDs(parallel)) {
		t.Fatalf("parallel files differ:\nsequential=%v\nparallel=%v", fileIDs(sequential), fileIDs(parallel))
	}
	if !reflect.DeepEqual(symbolIDs(sequential), symbolIDs(parallel)) {
		t.Fatalf("parallel symbols differ:\nsequential=%v\nparallel=%v", symbolIDs(sequential), symbolIDs(parallel))
	}
	if !reflect.DeepEqual(relationIDs(sequential), relationIDs(parallel)) {
		t.Fatalf("parallel relations differ:\nsequential=%v\nparallel=%v", relationIDs(sequential), relationIDs(parallel))
	}
}

func TestAnalyzerResolvesNestedSelectorMethodHeuristically(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "services/order/create.go", `package order

type orderService struct{}

func (s *orderService) CreateOrder() {}
`)
	writeFile(t, repo, "controllers/order/place_order.go", `package order

type Services struct {
	OrderService any
}

type OrderController struct {
	services Services
}

func (c *OrderController) PlaceOrder() {
	c.services.OrderService.CreateOrder()
}
`)

	analyzer := New()
	batch, err := analyzer.Analyze(context.Background(), parser.Input{
		ProjectID:     "dax-be",
		RepoPath:      repo,
		IncludePaths:  []string{"services", "controllers"},
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	from := "symbol:go:order@controllers/order:method:OrderController.PlaceOrder"
	to := "symbol:go:order@services/order:method:orderService.CreateOrder"
	for _, relation := range batch.Relations {
		if relation.Type != model.RelationCalls || relation.FromSymbolID != from || relation.ToSymbolID != to {
			continue
		}
		if relation.Metadata["resolution"] != "heuristic_selector_method" {
			t.Fatalf("expected heuristic metadata, got %#v", relation.Metadata)
		}
		if relation.Metadata["selector"] != "c.services.OrderService.CreateOrder" {
			t.Fatalf("expected selector metadata, got %#v", relation.Metadata)
		}
		return
	}
	t.Fatalf("expected call relation %s -> %s, got %#v", from, to, batch.Relations)
}

func TestAnalyzerPrefersSelectorReceiverOverAmbiguousMethodName(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "services/order/create.go", `package order

type orderService struct {
	repositories struct {
		OrderRepository any
	}
}

func (s *orderService) CreateOrder() {
	s.repositories.OrderRepository.Create()
}
`)
	writeFile(t, repo, "repositories/order_repository.go", `package repositories

type orderRepository struct{}

func (r *orderRepository) Create() {}
`)
	writeFile(t, repo, "services/accounting_export_log/service.go", `package accounting_export_log

type accountingExportLogService struct{}

func (s *accountingExportLogService) Create() {}
`)

	analyzer := New()
	batch, err := analyzer.Analyze(context.Background(), parser.Input{
		ProjectID:     "dax-be",
		RepoPath:      repo,
		IncludePaths:  []string{"services", "repositories"},
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	from := "symbol:go:order@services/order:method:orderService.CreateOrder"
	want := "symbol:go:repositories@repositories:method:orderRepository.Create"
	wrong := "symbol:go:accounting_export_log@services/accounting_export_log:method:accountingExportLogService.Create"
	if !hasCallRelation(batch.Relations, from, want) {
		t.Fatalf("expected call relation %s -> %s, got %#v", from, want, batch.Relations)
	}
	if hasCallRelation(batch.Relations, from, wrong) {
		t.Fatalf("unexpected ambiguous call relation %s -> %s", from, wrong)
	}
}

func TestAnalyzerResolvesPackageQualifiedFunctionCalls(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "services/order/service.go", `package order

func NewOrderService() {}
func NewOrderServiceMock() {}
`)
	writeFile(t, repo, "cmd/app/services.go", `package app

import "example.com/dax/services/order"

func NewServices() {
	order.NewOrderService()
}
`)

	analyzer := New()
	batch, err := analyzer.Analyze(context.Background(), parser.Input{
		ProjectID:     "dax-be",
		RepoPath:      repo,
		IncludePaths:  []string{"services", "cmd"},
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	from := "symbol:go:app@cmd/app:func:NewServices"
	to := "symbol:go:order@services/order:func:NewOrderService"
	if !hasCallRelation(batch.Relations, from, to) {
		t.Fatalf("expected call relation %s -> %s, got %#v", from, to, batch.Relations)
	}
}

func TestAnalyzerResolvesImportAliasFunctionCalls(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "pkg/service_errors/errors.go", `package service_errors

func NewError() {}
`)
	writeFile(t, repo, "services/ownership_transfer_request/service.go", `package ownership_transfer_request

import serviceErrors "example.com/dax/pkg/service_errors"

type OwnershipTransferRequestService struct{}

func (s *OwnershipTransferRequestService) ApproveRequest() {
	serviceErrors.NewError()
}
`)

	analyzer := New()
	batch, err := analyzer.Analyze(context.Background(), parser.Input{
		ProjectID:     "dax-be",
		RepoPath:      repo,
		IncludePaths:  []string{"pkg", "services"},
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	from := "symbol:go:ownership_transfer_request@services/ownership_transfer_request:method:OwnershipTransferRequestService.ApproveRequest"
	to := "symbol:go:service_errors@pkg/service_errors:func:NewError"
	if !hasCallRelation(batch.Relations, from, to) {
		t.Fatalf("expected import alias call relation %s -> %s, got %#v", from, to, batch.Relations)
	}
}

func TestAnalyzerResolvesReceiverLocalMethodCalls(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "services/user/user_status.go", `package user

type userService struct{}

func (s *userService) Activate() {
	_ = func() error {
		return s.doActivateUser()
	}
	_ = s.doActivateUser()
}

func (s *userService) doActivateUser() error {
	return nil
}
`)
	writeFile(t, repo, "consumer/handler/partition_worker_dispatch.go", `package handler

type PartitionWorker struct{}

func (pw *PartitionWorker) processSingleEventWithPanicGuard() {
	_ = pw.processEvent()
}

func (pw *PartitionWorker) processEvent() error {
	return pw.processEventByType()
}

func (pw *PartitionWorker) processEventByType() error {
	return nil
}
`)

	analyzer := New()
	batch, err := analyzer.Analyze(context.Background(), parser.Input{
		ProjectID:     "dax-be",
		RepoPath:      repo,
		IncludePaths:  []string{"services", "consumer"},
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	tests := []struct {
		from string
		to   string
	}{
		{
			from: "symbol:go:user@services/user:method:userService.Activate",
			to:   "symbol:go:user@services/user:method:userService.doActivateUser",
		},
		{
			from: "symbol:go:handler@consumer/handler:method:PartitionWorker.processSingleEventWithPanicGuard",
			to:   "symbol:go:handler@consumer/handler:method:PartitionWorker.processEvent",
		},
		{
			from: "symbol:go:handler@consumer/handler:method:PartitionWorker.processEvent",
			to:   "symbol:go:handler@consumer/handler:method:PartitionWorker.processEventByType",
		},
	}
	for _, tt := range tests {
		if !hasCallRelation(batch.Relations, tt.from, tt.to) {
			t.Fatalf("expected receiver-local call relation %s -> %s, got %#v", tt.from, tt.to, batch.Relations)
		}
	}
}

func TestAnalyzerResolvesLocalModelMethodCalls(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "models/aml_transaction.go", `package models

type AmlTransaction struct{}

func (at *AmlTransaction) BuildAmlRef() string {
	return ""
}

func (at *AmlTransaction) MergeRawResponse() error {
	return nil
}
`)
	writeFile(t, repo, "services/aml_transaction/service.go", `package aml_transaction

import "example.com/dax/models"

type AmlTransactionService struct{}

func (s *AmlTransactionService) findOrCreateAmlTransaction() {
	amlTransaction := &models.AmlTransaction{}
	amlTransaction.BuildAmlRef()
}

func (s *AmlTransactionService) handleAmlStatusUpdate() {
	amlTransaction, _ := s.findOrCreateAmlTransactionResult()
	amlTransaction.MergeRawResponse()
}

func (s *AmlTransactionService) findOrCreateAmlTransactionResult() (*models.AmlTransaction, error) {
	return nil, nil
}
`)

	analyzer := New()
	batch, err := analyzer.Analyze(context.Background(), parser.Input{
		ProjectID:     "dax-be",
		RepoPath:      repo,
		IncludePaths:  []string{"models", "services"},
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	tests := []struct {
		from string
		to   string
	}{
		{
			from: "symbol:go:aml_transaction@services/aml_transaction:method:AmlTransactionService.findOrCreateAmlTransaction",
			to:   "symbol:go:models@models:method:AmlTransaction.BuildAmlRef",
		},
		{
			from: "symbol:go:aml_transaction@services/aml_transaction:method:AmlTransactionService.handleAmlStatusUpdate",
			to:   "symbol:go:models@models:method:AmlTransaction.MergeRawResponse",
		},
	}
	for _, tt := range tests {
		if !hasCallRelation(batch.Relations, tt.from, tt.to) {
			t.Fatalf("expected model method call relation %s -> %s, got %#v", tt.from, tt.to, batch.Relations)
		}
	}
}

func hasCallRelation(relations []model.Relation, from, to string) bool {
	for _, relation := range relations {
		if relation.Type == model.RelationCalls && relation.FromSymbolID == from && relation.ToSymbolID == to {
			return true
		}
	}
	return false
}

func fileIDs(batch model.AnalysisBatch) []string {
	ids := make([]string, 0, len(batch.Files))
	for _, file := range batch.Files {
		ids = append(ids, file.ID)
	}
	return ids
}

func symbolIDs(batch model.AnalysisBatch) []string {
	ids := make([]string, 0, len(batch.Symbols))
	for _, symbol := range batch.Symbols {
		ids = append(ids, symbol.ID)
	}
	return ids
}

func relationIDs(batch model.AnalysisBatch) []string {
	ids := make([]string, 0, len(batch.Relations))
	for _, relation := range batch.Relations {
		ids = append(ids, relation.ID)
	}
	return ids
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
