package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hungnm98/seshat/internal/admin"
	"github.com/hungnm98/seshat/internal/auth"
	"github.com/hungnm98/seshat/internal/ingestion"
	"github.com/hungnm98/seshat/internal/query"
	"github.com/hungnm98/seshat/internal/storage/memory"
	"github.com/hungnm98/seshat/pkg/logger"
)

func TestAdminLoginAndProtectedRoute(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	authSvc := auth.NewService(store, time.Hour)
	if err := admin.SeedBootstrapAdmin(ctx, authSvc, "admin", "admin123"); err != nil {
		t.Fatalf("SeedBootstrapAdmin: %v", err)
	}
	adminSvc, err := admin.NewService(store, authSvc)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	router := NewRouter(ctx, Dependencies{
		Logger:       logger.New(slog.LevelInfo),
		Store:        store,
		AuthService:  authSvc,
		Ingest:       ingestion.NewService(store),
		Query:        query.NewService(store),
		AdminService: adminSvc,
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusFound {
		t.Fatalf("expected redirect for anonymous admin route, got %d", resp.Code)
	}

	form := url.Values{
		"username": {"admin"},
		"password": {"admin123"},
	}
	loginReq := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusFound {
		t.Fatalf("expected login redirect, got %d", loginResp.Code)
	}
	cookie := loginResp.Result().Cookies()[0]

	protectedReq := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	protectedReq.AddCookie(cookie)
	protectedResp := httptest.NewRecorder()
	router.ServeHTTP(protectedResp, protectedReq)
	if protectedResp.Code != http.StatusOK {
		t.Fatalf("expected authenticated dashboard, got %d", protectedResp.Code)
	}
}
