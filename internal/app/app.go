package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hungnm98/seshat/internal/admin"
	"github.com/hungnm98/seshat/internal/auth"
	"github.com/hungnm98/seshat/internal/config"
	"github.com/hungnm98/seshat/internal/ingestion"
	"github.com/hungnm98/seshat/internal/query"
	"github.com/hungnm98/seshat/internal/storage"
	"github.com/hungnm98/seshat/internal/storage/memory"
	postgresstore "github.com/hungnm98/seshat/internal/storage/postgres"
)

type Services struct {
	Store        storage.Store
	AuthService  *auth.Service
	Ingest       *ingestion.Service
	Query        *query.Service
	AdminService *admin.Service
}

func NewServices(ctx context.Context, cfg config.ServerConfig, logger *slog.Logger) (Services, error) {
	var store storage.Store
	switch cfg.StoreKind {
	case "", "memory":
		store = memory.New()
	case "postgres":
		if cfg.PostgresDSN == "" {
			return Services{}, fmt.Errorf("SESHAT_POSTGRES_DSN is required when SESHAT_STORE_KIND=postgres")
		}
		pgStore, err := postgresstore.Open(ctx, cfg.PostgresDSN)
		if err != nil {
			return Services{}, err
		}
		if err := pgStore.EnsureSchema(ctx); err != nil {
			return Services{}, err
		}
		store = pgStore
	default:
		logger.Warn("unsupported store kind for MVP; falling back to memory", "kind", cfg.StoreKind)
		store = memory.New()
	}

	authSvc := auth.NewService(store, cfg.Admin.SessionTTL)
	if err := admin.SeedBootstrapAdmin(ctx, authSvc, cfg.Admin.Username, cfg.Admin.Password); err != nil {
		return Services{}, err
	}
	adminSvc, err := admin.NewService(store, authSvc)
	if err != nil {
		return Services{}, err
	}
	return Services{
		Store:        store,
		AuthService:  authSvc,
		Ingest:       ingestion.NewService(store),
		Query:        query.NewService(store),
		AdminService: adminSvc,
	}, nil
}
