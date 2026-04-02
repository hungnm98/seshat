package auth

import (
	"context"
	"testing"
	"time"

	"github.com/hungnm98/seshat/internal/storage/memory"
	"github.com/hungnm98/seshat/pkg/model"
)

func TestProjectTokenLifecycle(t *testing.T) {
	store := memory.New()
	authSvc := NewService(store, time.Hour)
	if _, err := store.CreateProject(context.Background(), model.Project{ID: "proj-a", Name: "A", DefaultBranch: "main"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	secret, err := authSvc.CreateProjectToken(context.Background(), "proj-a", "ci token", "admin", nil)
	if err != nil {
		t.Fatalf("CreateProjectToken: %v", err)
	}
	if _, err := authSvc.VerifyProjectToken(context.Background(), secret.Plain, "proj-a"); err != nil {
		t.Fatalf("VerifyProjectToken: %v", err)
	}
	if _, err := authSvc.VerifyProjectToken(context.Background(), secret.Plain, "proj-b"); err == nil {
		t.Fatalf("expected wrong-project token check to fail")
	}
	if err := authSvc.RevokeProjectToken(context.Background(), secret.Token.ID, "admin"); err != nil {
		t.Fatalf("RevokeProjectToken: %v", err)
	}
	if _, err := authSvc.VerifyProjectToken(context.Background(), secret.Plain, "proj-a"); err == nil {
		t.Fatalf("expected revoked token verification to fail")
	}
}
