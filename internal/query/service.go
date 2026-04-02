package query

import (
	"context"

	"github.com/hungnm98/seshat/internal/storage"
	"github.com/hungnm98/seshat/pkg/model"
)

type Service struct {
	store storage.Store
}

func NewService(store storage.Store) *Service {
	return &Service{store: store}
}

func (s *Service) FindSymbol(ctx context.Context, projectID, query, kind string, limit int) ([]model.Symbol, *model.ProjectVersion, error) {
	return s.store.FindSymbols(ctx, projectID, query, kind, limit)
}

func (s *Service) GetSymbolDetail(ctx context.Context, projectID, symbolID string) (model.QuerySymbolResult, bool, error) {
	symbol, inbound, outbound, version, ok, err := s.store.GetSymbol(ctx, projectID, symbolID)
	if err != nil || !ok {
		return model.QuerySymbolResult{}, ok, err
	}
	return model.QuerySymbolResult{
		Symbol:   symbol,
		Inbound:  inbound,
		Outbound: outbound,
		Version:  version,
	}, true, nil
}

func (s *Service) FindCallers(ctx context.Context, projectID, symbolID string, depth int) ([]model.Symbol, []model.Relation, *model.ProjectVersion, error) {
	return s.store.TraverseCalls(ctx, projectID, symbolID, depth, "callers")
}

func (s *Service) FindCallees(ctx context.Context, projectID, symbolID string, depth int) ([]model.Symbol, []model.Relation, *model.ProjectVersion, error) {
	return s.store.TraverseCalls(ctx, projectID, symbolID, depth, "callees")
}
