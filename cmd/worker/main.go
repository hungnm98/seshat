package main

import (
	"context"
	"log"
	"log/slog"
	"time"

	"github.com/hungnm98/seshat/internal/app"
	"github.com/hungnm98/seshat/internal/config"
	"github.com/hungnm98/seshat/pkg/logger"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadServerFromEnv()
	logr := logger.New(slog.LevelInfo)
	if _, err := app.NewServices(ctx, cfg, logr); err != nil {
		log.Fatal(err)
	}
	logr.Info("worker skeleton started", "mode", "no-op")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		logr.Info("worker heartbeat", "status", "idle")
	}
}
