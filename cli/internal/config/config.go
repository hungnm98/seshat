package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	AppName       string
	HTTPAddr      string
	BaseURL       string
	StoreKind     string
	PostgresDSN   string
	RedisAddr     string
	MinIOEndpoint string
	MinIOBucket   string
	Admin         AdminConfig
}

type AdminConfig struct {
	Username   string
	Password   string
	SessionTTL time.Duration
	UseGoAdmin bool
}

type CLIProjectConfig struct {
	ProjectID       string   `yaml:"project_id"`
	RepoPath        string   `yaml:"repo_path"`
	LanguageTargets []string `yaml:"language_targets"`
	IncludePaths    []string `yaml:"include_paths"`
	ExcludePaths    []string `yaml:"exclude_paths"`
	ServerEndpoint  string   `yaml:"server_endpoint"`
	TokenEnvVar     string   `yaml:"token_env_var"`
}

func LoadServerFromEnv() ServerConfig {
	cfg := ServerConfig{
		AppName:       envOrDefault("SESHAT_APP_NAME", "seshat"),
		HTTPAddr:      envOrDefault("SESHAT_HTTP_ADDR", ":8080"),
		BaseURL:       envOrDefault("SESHAT_BASE_URL", "http://localhost:8080"),
		StoreKind:     envOrDefault("SESHAT_STORE_KIND", "memory"),
		PostgresDSN:   os.Getenv("SESHAT_POSTGRES_DSN"),
		RedisAddr:     envOrDefault("SESHAT_REDIS_ADDR", "localhost:6379"),
		MinIOEndpoint: envOrDefault("SESHAT_MINIO_ENDPOINT", "http://localhost:9000"),
		MinIOBucket:   envOrDefault("SESHAT_MINIO_BUCKET", "seshat"),
		Admin: AdminConfig{
			Username:   envOrDefault("SESHAT_ADMIN_USERNAME", "admin"),
			Password:   envOrDefault("SESHAT_ADMIN_PASSWORD", "admin123"),
			SessionTTL: envDuration("SESHAT_ADMIN_SESSION_TTL", 12*time.Hour),
			UseGoAdmin: strings.EqualFold(envOrDefault("SESHAT_GOADMIN_ENABLED", "false"), "true"),
		},
	}
	return cfg
}

func LoadCLIProject(path string) (CLIProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CLIProjectConfig{}, fmt.Errorf("read project config: %w", err)
	}
	var cfg CLIProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return CLIProjectConfig{}, fmt.Errorf("parse project config: %w", err)
	}
	if cfg.RepoPath == "" {
		cfg.RepoPath = filepath.Dir(path)
	}
	if cfg.TokenEnvVar == "" {
		cfg.TokenEnvVar = "SESHAT_PROJECT_TOKEN"
	}
	if cfg.ServerEndpoint == "" {
		cfg.ServerEndpoint = "http://localhost:8080"
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
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
