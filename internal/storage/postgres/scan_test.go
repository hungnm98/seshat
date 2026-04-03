package postgres

import (
	"database/sql"
	"testing"
	"time"
)

func TestVersionID(t *testing.T) {
	got := versionID("project-a", "abc123")
	if got != "project-a:abc123" {
		t.Fatalf("unexpected version id: %s", got)
	}
}

func TestMarshalMetadata(t *testing.T) {
	got, err := marshalMetadata(nil)
	if err != nil {
		t.Fatalf("marshalMetadata(nil): %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("unexpected metadata payload: %s", string(got))
	}
}

func TestScanProjectToken(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	revokedAt := now.Add(time.Minute)
	scan := func(dest ...interface{}) error {
		if len(dest) != 12 {
			t.Fatalf("unexpected scan arity: %d", len(dest))
		}
		*(dest[0].(*string)) = "token-1"
		*(dest[1].(*string)) = "project-1"
		*(dest[2].(*string)) = "CI token"
		*(dest[3].(*string)) = "prefix-1"
		*(dest[4].(*string)) = "hash-1"
		*(dest[5].(*string)) = "active"
		*(dest[6].(*sql.NullTime)) = sql.NullTime{Time: now.Add(time.Hour), Valid: true}
		*(dest[7].(*sql.NullTime)) = sql.NullTime{Time: now.Add(2 * time.Hour), Valid: true}
		*(dest[8].(*time.Time)) = now
		*(dest[9].(*string)) = "admin"
		*(dest[10].(*sql.NullTime)) = sql.NullTime{Time: revokedAt, Valid: true}
		*(dest[11].(*sql.NullString)) = sql.NullString{}
		return nil
	}
	token, err := scanProjectToken(scan)
	if err != nil {
		t.Fatalf("scanProjectToken: %v", err)
	}
	if token.ID != "token-1" || token.ProjectID != "project-1" || token.TokenHash != "hash-1" {
		t.Fatalf("unexpected token: %#v", token)
	}
	if token.ExpiresAt == nil || !token.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected expires_at: %#v", token.ExpiresAt)
	}
	if token.LastUsedAt == nil || !token.LastUsedAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("unexpected last_used_at: %#v", token.LastUsedAt)
	}
	if token.RevokedAt == nil || !token.RevokedAt.Equal(revokedAt) {
		t.Fatalf("unexpected revoked_at: %#v", token.RevokedAt)
	}
	if token.RevokedBy != "" {
		t.Fatalf("unexpected revoked_by: %q", token.RevokedBy)
	}
}
