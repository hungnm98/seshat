package javascript

import (
	"context"
	"os"
	"testing"

	"github.com/hungnm98/seshat-cli/internal/parser"
	"github.com/hungnm98/seshat-cli/pkg/model"
)

// --- helpers ---

func analyzeJS(t *testing.T, src string) model.AnalysisBatch {
	t.Helper()
	return analyzeSource(t, src, ".js", NewJS())
}

func analyzeTS(t *testing.T, src string) model.AnalysisBatch {
	t.Helper()
	return analyzeSource(t, src, ".ts", NewTS())
}

func analyzeSource(t *testing.T, src, ext string, a *Analyzer) model.AnalysisBatch {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/test" + ext
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	batch, err := a.Analyze(context.Background(), parser.Input{
		ProjectID:     "test",
		RepoPath:      dir,
		CommitSHA:     "test",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return batch
}

func symbolIDSet(batch model.AnalysisBatch) map[string]struct{} {
	m := make(map[string]struct{}, len(batch.Symbols))
	for _, sym := range batch.Symbols {
		m[sym.ID] = struct{}{}
	}
	return m
}

// --- JavaScript tests ---

func TestJSAnalyzesSampleDir(t *testing.T) {
	a := NewJS()
	batch, err := a.Analyze(context.Background(), parser.Input{
		ProjectID:     "js-app",
		RepoPath:      "../../../testdata/js_sample",
		IncludePaths:  []string{"src"},
		CommitSHA:     "abc123",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if len(batch.Symbols) == 0 {
		t.Fatal("expected JS symbols to be extracted")
	}
}

func TestJSClassAndMethods(t *testing.T) {
	batch := analyzeJS(t, `
class Animal {
  constructor(name) {
    this.name = name;
  }

  speak() {
    return this.name;
  }

  toString() {
    return this.speak();
  }
}
`)
	ids := symbolIDSet(batch)
	if _, ok := ids["symbol:javascript:class:test:Animal"]; !ok {
		t.Error("expected class Animal")
	}
	if _, ok := ids["symbol:javascript:method:test:Animal#constructor"]; !ok {
		t.Error("expected method constructor")
	}
	if _, ok := ids["symbol:javascript:method:test:Animal#speak"]; !ok {
		t.Error("expected method speak")
	}
	if _, ok := ids["symbol:javascript:method:test:Animal#toString"]; !ok {
		t.Error("expected method toString")
	}
}

func TestJSInheritance(t *testing.T) {
	batch := analyzeJS(t, `
class Animal {
  speak() {}
}
class Dog extends Animal {
  bark() {}
}
`)
	hasInherits := false
	for _, rel := range batch.Relations {
		if rel.Type == model.RelationInherits &&
			rel.FromSymbolID == "symbol:javascript:class:test:Dog" &&
			rel.ToSymbolID == "symbol:javascript:class:test:Animal" {
			hasInherits = true
		}
	}
	if !hasInherits {
		t.Error("expected inherits relation from Dog to Animal")
	}
}

func TestJSFunctionDeclarations(t *testing.T) {
	batch := analyzeJS(t, `
function greet(name) {
  return 'Hello ' + name;
}

async function fetchData(url) {
  return fetch(url);
}

const double = (x) => x * 2;

const triple = function(x) {
  return x * 3;
};
`)
	ids := symbolIDSet(batch)
	for _, name := range []string{"greet", "fetchData", "double", "triple"} {
		id := "symbol:javascript:func:test:" + name
		if _, ok := ids[id]; !ok {
			t.Errorf("expected function %s", name)
		}
	}
}

func TestJSImports(t *testing.T) {
	batch := analyzeJS(t, `
import React from 'react';
import { useState, useEffect } from 'react';
import './styles.css';
const fs = require('fs');
`)
	var importSymbols []model.Symbol
	for _, sym := range batch.Symbols {
		if sym.Kind == "import" {
			importSymbols = append(importSymbols, sym)
		}
	}
	if len(importSymbols) < 3 {
		t.Errorf("expected at least 3 import symbols, got %d", len(importSymbols))
	}
	hasImportRelation := false
	for _, rel := range batch.Relations {
		if rel.Type == model.RelationImports {
			hasImportRelation = true
			break
		}
	}
	if !hasImportRelation {
		t.Error("expected at least one imports relation")
	}
}

func TestJSLineEndTracking(t *testing.T) {
	batch := analyzeJS(t, `
class Foo {
  short() {
    return 1;
  }

  longer() {
    const x = 1;
    const y = 2;
    return x + y;
  }
}
`)
	for _, sym := range batch.Symbols {
		if sym.LineEnd > 0 && sym.LineEnd < sym.LineStart {
			t.Errorf("symbol %s has line_end %d < line_start %d", sym.ID, sym.LineEnd, sym.LineStart)
		}
		if sym.Kind == "method" && sym.Name == "longer" && sym.LineEnd <= sym.LineStart {
			t.Errorf("method longer should span multiple lines, got start=%d end=%d", sym.LineStart, sym.LineEnd)
		}
	}
}

func TestJSDeclaredInRelations(t *testing.T) {
	batch := analyzeJS(t, `
function foo() {}
class Bar {}
`)
	hasFuncDeclared := false
	hasClassDeclared := false
	for _, rel := range batch.Relations {
		if rel.Type == model.RelationDeclaredIn {
			if rel.FromSymbolID == "symbol:javascript:func:test:foo" {
				hasFuncDeclared = true
			}
			if rel.FromSymbolID == "symbol:javascript:class:test:Bar" {
				hasClassDeclared = true
			}
		}
	}
	if !hasFuncDeclared {
		t.Error("expected declared_in relation for function foo")
	}
	if !hasClassDeclared {
		t.Error("expected declared_in relation for class Bar")
	}
}

func TestJSContainsRelations(t *testing.T) {
	batch := analyzeJS(t, `
class Service {
  run() {}
  stop() {}
}
`)
	containsCount := 0
	for _, rel := range batch.Relations {
		if rel.Type == model.RelationContains &&
			rel.FromSymbolID == "symbol:javascript:class:test:Service" {
			containsCount++
		}
	}
	if containsCount < 2 {
		t.Errorf("expected at least 2 contains relations from Service, got %d", containsCount)
	}
}

func TestJSParallelMatchesSequential(t *testing.T) {
	input := parser.Input{
		ProjectID:     "js-app",
		RepoPath:      "../../../testdata/js_sample",
		IncludePaths:  []string{"src"},
		CommitSHA:     "abc123",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
		Parallelism:   1,
	}
	a := NewJS()
	seq, err := a.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("sequential: %v", err)
	}
	input.Parallelism = 4
	par, err := a.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("parallel: %v", err)
	}
	if len(seq.Symbols) != len(par.Symbols) {
		t.Errorf("symbol count differs: seq=%d par=%d", len(seq.Symbols), len(par.Symbols))
	}
	if len(seq.Relations) != len(par.Relations) {
		t.Errorf("relation count differs: seq=%d par=%d", len(seq.Relations), len(par.Relations))
	}
}

// --- TypeScript tests ---

func TestTSAnalyzesSampleDir(t *testing.T) {
	a := NewTS()
	batch, err := a.Analyze(context.Background(), parser.Input{
		ProjectID:     "ts-app",
		RepoPath:      "../../../testdata/ts_sample",
		IncludePaths:  []string{"src"},
		CommitSHA:     "abc123",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
	})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if len(batch.Symbols) == 0 {
		t.Fatal("expected TS symbols to be extracted")
	}
}

func TestTSInterfaceAndType(t *testing.T) {
	batch := analyzeTS(t, `
export interface Animal {
  speak(): string;
}

export type Color = 'red' | 'green' | 'blue';
`)
	ids := symbolIDSet(batch)
	if _, ok := ids["symbol:typescript:interface:test:Animal"]; !ok {
		t.Error("expected interface Animal")
	}
	if _, ok := ids["symbol:typescript:type:test:Color"]; !ok {
		t.Error("expected type Color")
	}
}

func TestTSEnum(t *testing.T) {
	batch := analyzeTS(t, `
export enum Status {
  Active = 'active',
  Inactive = 'inactive',
}

export const enum Direction {
  Up,
  Down,
}
`)
	ids := symbolIDSet(batch)
	if _, ok := ids["symbol:typescript:enum:test:Status"]; !ok {
		t.Error("expected enum Status")
	}
	if _, ok := ids["symbol:typescript:enum:test:Direction"]; !ok {
		t.Error("expected const enum Direction")
	}
}

func TestTSNamespace(t *testing.T) {
	batch := analyzeTS(t, `
export namespace Utils {
  export function helper() {}
}
`)
	ids := symbolIDSet(batch)
	if _, ok := ids["symbol:typescript:namespace:test:Utils"]; !ok {
		t.Error("expected namespace Utils")
	}
}

func TestTSClassWithModifiers(t *testing.T) {
	batch := analyzeTS(t, `
export class UserService {
  private name: string;

  constructor(name: string) {
    this.name = name;
  }

  public getName(): string {
    return this.name;
  }

  static create(name: string): UserService {
    return new UserService(name);
  }

  async fetchProfile(): Promise<void> {
    return Promise.resolve();
  }
}
`)
	ids := symbolIDSet(batch)
	if _, ok := ids["symbol:typescript:class:test:UserService"]; !ok {
		t.Error("expected class UserService")
	}
	for _, method := range []string{"constructor", "getName", "create", "fetchProfile"} {
		id := "symbol:typescript:method:test:UserService#" + method
		if _, ok := ids[id]; !ok {
			t.Errorf("expected method %s", method)
		}
	}
}

func TestTSThisCallRelation(t *testing.T) {
	batch := analyzeTS(t, `
class Counter {
  count(): number {
    return this.value();
  }

  value(): number {
    return 0;
  }
}
`)
	hasCall := false
	for _, rel := range batch.Relations {
		if rel.Type == model.RelationCalls &&
			rel.FromSymbolID == "symbol:typescript:method:test:Counter#count" &&
			rel.ToSymbolID == "symbol:typescript:method:test:Counter#value" {
			hasCall = true
		}
	}
	if !hasCall {
		t.Error("expected calls relation from count to value via this.value()")
	}
}

func TestTSGenericFunction(t *testing.T) {
	batch := analyzeTS(t, `
export function identity<T>(x: T): T {
  return x;
}
`)
	ids := symbolIDSet(batch)
	if _, ok := ids["symbol:typescript:func:test:identity"]; !ok {
		t.Error("expected generic function identity")
	}
}

func TestTSParallelMatchesSequential(t *testing.T) {
	input := parser.Input{
		ProjectID:     "ts-app",
		RepoPath:      "../../../testdata/ts_sample",
		IncludePaths:  []string{"src"},
		CommitSHA:     "abc123",
		Branch:        "main",
		SchemaVersion: "v1",
		ScanMode:      "full",
		Parallelism:   1,
	}
	a := NewTS()
	seq, err := a.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("sequential: %v", err)
	}
	input.Parallelism = 4
	par, err := a.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("parallel: %v", err)
	}
	if len(seq.Symbols) != len(par.Symbols) {
		t.Errorf("symbol count differs: seq=%d par=%d", len(seq.Symbols), len(par.Symbols))
	}
	if len(seq.Relations) != len(par.Relations) {
		t.Errorf("relation count differs: seq=%d par=%d", len(seq.Relations), len(par.Relations))
	}
}
