package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/hungnm98/seshat/internal/storage"
	"github.com/hungnm98/seshat/internal/storage/memory"
	"github.com/hungnm98/seshat/pkg/model"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrUnauthorized       = errors.New("unauthorized")
)

type Service struct {
	store      storage.Store
	sessionTTL time.Duration
	sessions   map[string]model.AdminSession
}

func NewService(store storage.Store, sessionTTL time.Duration) *Service {
	return &Service{
		store:      store,
		sessionTTL: sessionTTL,
		sessions:   make(map[string]model.AdminSession),
	}
}

func HashPassword(password string) (string, error) {
	data, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Service) BootstrapAdmin(ctx context.Context, username, name, password string) (model.AdminUser, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return model.AdminUser{}, err
	}
	return s.store.BootstrapAdmin(ctx, username, name, hash)
}

func (s *Service) LoginAdmin(ctx context.Context, username, password string) (model.AdminSession, model.AdminUser, error) {
	user, ok, err := s.store.AuthenticateAdmin(ctx, username)
	if err != nil {
		return model.AdminSession{}, model.AdminUser{}, err
	}
	if !ok || !CheckPassword(user.PasswordHash, password) {
		return model.AdminSession{}, model.AdminUser{}, ErrInvalidCredentials
	}
	now := time.Now().UTC()
	session := model.AdminSession{
		ID:        "sess:" + mustRandomHex(24),
		UserID:    user.ID,
		Username:  user.Username,
		CreatedAt: now,
		ExpiresAt: now.Add(s.sessionTTL),
	}
	s.sessions[session.ID] = session
	_ = s.store.UpdateAdminLastLogin(ctx, user.ID, now)
	return session, user, nil
}

func (s *Service) GetSession(sessionID string) (model.AdminSession, bool) {
	session, ok := s.sessions[sessionID]
	if !ok || session.ExpiresAt.Before(time.Now().UTC()) {
		return model.AdminSession{}, false
	}
	return session, true
}

func (s *Service) Logout(sessionID string) {
	delete(s.sessions, sessionID)
}

func (s *Service) CreateProjectToken(ctx context.Context, projectID, description, createdBy string, expiresAt *time.Time) (model.ProjectTokenSecret, error) {
	raw := "seshat_" + mustRandomHex(18)
	token := model.ProjectToken{
		ID:          "token:" + mustRandomHex(8),
		ProjectID:   projectID,
		Description: description,
		TokenPrefix: raw[:12],
		TokenHash:   memory.HashToken(raw),
		Status:      "active",
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   createdBy,
	}
	stored, err := s.store.CreateProjectToken(ctx, token)
	if err != nil {
		return model.ProjectTokenSecret{}, err
	}
	return model.ProjectTokenSecret{Token: stored, Plain: raw}, nil
}

func (s *Service) VerifyProjectToken(ctx context.Context, rawToken, projectID string) (model.ProjectToken, error) {
	hash := memory.HashToken(rawToken)
	token, ok, err := s.store.FindProjectTokenByHash(ctx, hash)
	if err != nil {
		return model.ProjectToken{}, err
	}
	if !ok {
		return model.ProjectToken{}, ErrInvalidToken
	}
	if token.ProjectID != projectID || token.Status != "active" {
		return model.ProjectToken{}, ErrUnauthorized
	}
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now().UTC()) {
		return model.ProjectToken{}, ErrUnauthorized
	}
	now := time.Now().UTC()
	token.LastUsedAt = &now
	_ = s.store.UpdateProjectToken(ctx, token)
	return token, nil
}

func (s *Service) RevokeProjectToken(ctx context.Context, tokenID, revokedBy string) error {
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, project := range projects {
		tokens, listErr := s.store.ListProjectTokens(ctx, project.ID)
		if listErr != nil {
			return listErr
		}
		for _, token := range tokens {
			if token.ID != tokenID {
				continue
			}
			now := time.Now().UTC()
			token.Status = "revoked"
			token.RevokedAt = &now
			token.RevokedBy = revokedBy
			return s.store.UpdateProjectToken(ctx, token)
		}
	}
	return fmt.Errorf("token %s not found", tokenID)
}

func mustRandomHex(size int) string {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}
