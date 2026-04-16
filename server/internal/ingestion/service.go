package ingestion

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hungnm98/seshat-server/internal/storage"
	"github.com/hungnm98/seshat-server/pkg/graphschema"
	"github.com/hungnm98/seshat-server/pkg/model"
)

type Service struct {
	store storage.Store
}

func NewService(store storage.Store) *Service {
	return &Service{store: store}
}

func (s *Service) StoreBatch(ctx context.Context, batch model.AnalysisBatch) (model.IngestionRun, model.ProjectVersion, error) {
	if batch.Metadata.SchemaVersion == "" {
		batch.Metadata.SchemaVersion = graphschema.Version
	}
	if err := graphschema.Validate(batch); err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, fmt.Errorf("validate graph batch: %w", err)
	}
	raw, err := json.Marshal(batch)
	if err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, err
	}
	return s.store.StoreBatch(ctx, batch, raw)
}
