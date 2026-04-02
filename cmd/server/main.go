package main

import (
	"context"
	"log"
	"log/slog"

	"github.com/hungnm98/seshat/internal/admin"
	"github.com/hungnm98/seshat/internal/api"
	"github.com/hungnm98/seshat/internal/app"
	"github.com/hungnm98/seshat/internal/config"
	"github.com/hungnm98/seshat/pkg/logger"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadServerFromEnv()
	logr := logger.New(slog.LevelInfo)

	services, err := app.NewServices(ctx, cfg, logr)
	if err != nil {
		log.Fatal(err)
	}
	router := api.NewRouter(ctx, api.Dependencies{
		Logger:       logr,
		Store:        services.Store,
		AuthService:  services.AuthService,
		Ingest:       services.Ingest,
		Query:        services.Query,
		AdminService: services.AdminService,
	})
	if cfg.Admin.UseGoAdmin {
		if err := admin.SetupGoAdmin(router, cfg.PostgresDSN, logr); err != nil {
			logr.Error("failed to initialize go-admin", "error", err)
		}
	}
	logr.Info("starting seshat server", "addr", cfg.HTTPAddr)
	if err := router.Run(cfg.HTTPAddr); err != nil {
		log.Fatal(err)
	}
}
