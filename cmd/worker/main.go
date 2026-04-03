package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/hungnm98/seshat/internal/config"
	"github.com/hungnm98/seshat/internal/storage/objectstore"
	redisstore "github.com/hungnm98/seshat/internal/storage/redis"
	"github.com/hungnm98/seshat/pkg/logger"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadServerFromEnv()
	logr := logger.New(slog.LevelInfo)

	redisClient := redisstore.New(redisstore.Config{
		Addr:               cfg.RedisAddr,
		Password:           envOrDefault("SESHAT_REDIS_PASSWORD", ""),
		DB:                 envInt("SESHAT_REDIS_DB", 0),
		DialTimeout:        envDuration("SESHAT_REDIS_DIAL_TIMEOUT", 5*time.Second),
		ReadTimeout:        envDuration("SESHAT_REDIS_READ_TIMEOUT", 5*time.Second),
		WriteTimeout:       envDuration("SESHAT_REDIS_WRITE_TIMEOUT", 5*time.Second),
		UseTLS:             envBool("SESHAT_REDIS_TLS", false),
		InsecureSkipVerify: envBool("SESHAT_REDIS_TLS_INSECURE_SKIP_VERIFY", false),
	})
	objectClient, err := objectstore.New(objectstore.Config{
		Endpoint:              cfg.MinIOEndpoint,
		Bucket:                cfg.MinIOBucket,
		RequestTimeout:        envDuration("SESHAT_OBJECTSTORE_TIMEOUT", 10*time.Second),
		HealthPath:            envOrDefault("SESHAT_OBJECTSTORE_HEALTH_PATH", "/minio/health/ready"),
		ObjectPathPrefix:      envOrDefault("SESHAT_OBJECTSTORE_PREFIX", "worker"),
		UseTLS:                envBool("SESHAT_OBJECTSTORE_TLS", false),
		AllowUnsignedRequests: envBool("SESHAT_OBJECTSTORE_ALLOW_UNSIGNED", true),
		InsecureSkipTLSVerify: envBool("SESHAT_OBJECTSTORE_TLS_INSECURE_SKIP_VERIFY", false),
	})
	if err != nil {
		log.Fatal(err)
	}

	interval := envDuration("SESHAT_WORKER_INTERVAL", 30*time.Second)
	once := envBool("SESHAT_WORKER_ONCE", false)
	logr.Info("worker skeleton started", "mode", "follow-up", "interval", interval.String(), "once", once)

	runCycle := func() {
		runWorkerCycle(ctx, logr, redisClient, objectClient, cfg)
	}

	runCycle()
	if once {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		runCycle()
	}
}

func runWorkerCycle(ctx context.Context, logr *slog.Logger, redisClient *redisstore.Client, objectClient *objectstore.Client, cfg config.ServerConfig) {
	cycleAt := time.Now().UTC()
	redisProbe, redisErr := redisClient.Health(ctx)
	objectProbe, objectErr := objectClient.Health(ctx)

	status := map[string]interface{}{
		"checked_at":  cycleAt,
		"redis":       redisProbe,
		"objectstore": objectProbe,
		"message":     "worker follow-up cycle completed",
	}
	payload, _ := json.Marshal(status)

	if redisErr == nil {
		_ = redisClient.Set(ctx, "seshat:worker:last_cycle", string(payload), 5*time.Minute)
	}

	if objectErr == nil && envBool("SESHAT_OBJECTSTORE_WRITE_REPORT", false) {
		reportPrefix := envOrDefault("SESHAT_OBJECTSTORE_REPORT_PREFIX", "reports")
		_, putErr := objectClient.Put(ctx, fmt.Sprintf("%s/latest.json", reportPrefix), payload, "application/json")
		if putErr != nil {
			logr.Warn("object store report upload failed", "error", putErr)
		}
	}

	logr.Info("worker cycle finished",
		"redis_healthy", redisErr == nil,
		"objectstore_healthy", objectErr == nil,
		"redis_status", redisProbe.Status,
		"objectstore_endpoint", objectProbe.Endpoint,
		"bucket", cfg.MinIOBucket,
	)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}
