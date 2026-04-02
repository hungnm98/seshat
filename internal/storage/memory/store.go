package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hungnm98/seshat/internal/storage"
	"github.com/hungnm98/seshat/pkg/model"
)

type Store struct {
	mu                 sync.RWMutex
	admins             map[string]model.AdminUser
	projects           map[string]model.Project
	tokens             map[string]model.ProjectToken
	ingestionRuns      map[string][]model.IngestionRun
	projectVersions    map[string][]model.ProjectVersion
	filesByVersion     map[string][]model.File
	symbolsByVersion   map[string][]model.Symbol
	relationsByVersion map[string][]model.Relation
	rawPayloads        map[string][]byte
	auditLogs          []model.AuditLog
}

var _ storage.Store = (*Store)(nil)

func New() *Store {
	return &Store{
		admins:             make(map[string]model.AdminUser),
		projects:           make(map[string]model.Project),
		tokens:             make(map[string]model.ProjectToken),
		ingestionRuns:      make(map[string][]model.IngestionRun),
		projectVersions:    make(map[string][]model.ProjectVersion),
		filesByVersion:     make(map[string][]model.File),
		symbolsByVersion:   make(map[string][]model.Symbol),
		relationsByVersion: make(map[string][]model.Relation),
		rawPayloads:        make(map[string][]byte),
	}
}

func (s *Store) BootstrapAdmin(_ context.Context, username, name, passwordHash string) (model.AdminUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.admins[username]; ok {
		return existing, nil
	}
	user := model.AdminUser{
		ID:           "admin:" + username,
		Username:     username,
		Name:         name,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC(),
	}
	s.admins[username] = user
	return user, nil
}

func (s *Store) AuthenticateAdmin(_ context.Context, username string) (model.AdminUser, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.admins[username]
	return user, ok, nil
}

func (s *Store) UpdateAdminLastLogin(_ context.Context, userID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, user := range s.admins {
		if user.ID == userID {
			user.LastLoginAt = at.UTC()
			s.admins[key] = user
			return nil
		}
	}
	return fmt.Errorf("admin user %s not found", userID)
}

func (s *Store) CreateProject(_ context.Context, project model.Project) (model.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projects[project.ID]; exists {
		return model.Project{}, fmt.Errorf("project %s already exists", project.ID)
	}
	now := time.Now().UTC()
	project.CreatedAt = now
	project.UpdatedAt = now
	s.projects[project.ID] = project
	return project, nil
}

func (s *Store) ListProjects(_ context.Context) ([]model.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	projects := make([]model.Project, 0, len(s.projects))
	for _, project := range s.projects {
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].ID < projects[j].ID })
	return projects, nil
}

func (s *Store) GetProject(_ context.Context, projectID string) (model.Project, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	project, ok := s.projects[projectID]
	return project, ok, nil
}

func (s *Store) StoreBatch(_ context.Context, batch model.AnalysisBatch, raw []byte) (model.IngestionRun, model.ProjectVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[batch.Metadata.ProjectID]; !ok {
		return model.IngestionRun{}, model.ProjectVersion{}, fmt.Errorf("project %s not found", batch.Metadata.ProjectID)
	}
	versionID := fmt.Sprintf("%s:%s", batch.Metadata.ProjectID, batch.Metadata.CommitSHA)
	version := model.ProjectVersion{
		ID:         versionID,
		ProjectID:  batch.Metadata.ProjectID,
		CommitSHA:  batch.Metadata.CommitSHA,
		Branch:     batch.Metadata.Branch,
		Status:     "ready",
		Schema:     batch.Metadata.SchemaVersion,
		ScannedAt:  batch.Metadata.GeneratedAt.UTC(),
		FilesCount: len(batch.Files),
		NodesCount: len(batch.Symbols),
		EdgesCount: len(batch.Relations),
	}
	versions := s.projectVersions[batch.Metadata.ProjectID]
	replaced := false
	for idx := range versions {
		if versions[idx].ID == version.ID {
			versions[idx] = version
			replaced = true
		}
	}
	if !replaced {
		versions = append(versions, version)
	}
	s.projectVersions[batch.Metadata.ProjectID] = versions
	s.filesByVersion[version.ID] = append([]model.File(nil), batch.Files...)
	s.symbolsByVersion[version.ID] = append([]model.Symbol(nil), batch.Symbols...)
	s.relationsByVersion[version.ID] = append([]model.Relation(nil), batch.Relations...)
	s.rawPayloads[version.ID] = append([]byte(nil), raw...)

	run := model.IngestionRun{
		ID:           fmt.Sprintf("ingestion:%s:%d", batch.Metadata.ProjectID, time.Now().UTC().UnixNano()),
		ProjectID:    batch.Metadata.ProjectID,
		ProjectVerID: version.ID,
		CommitSHA:    batch.Metadata.CommitSHA,
		Status:       "completed",
		CreatedAt:    time.Now().UTC(),
		FinishedAt:   time.Now().UTC(),
	}
	s.ingestionRuns[batch.Metadata.ProjectID] = append(s.ingestionRuns[batch.Metadata.ProjectID], run)
	return run, version, nil
}

func (s *Store) ListIngestionRuns(_ context.Context, projectID string) ([]model.IngestionRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	runs := append([]model.IngestionRun(nil), s.ingestionRuns[projectID]...)
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	return runs, nil
}

func (s *Store) ListProjectVersions(_ context.Context, projectID string) ([]model.ProjectVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions := append([]model.ProjectVersion(nil), s.projectVersions[projectID]...)
	sort.Slice(versions, func(i, j int) bool { return versions[i].ScannedAt.After(versions[j].ScannedAt) })
	return versions, nil
}

func (s *Store) LatestProjectVersion(ctx context.Context, projectID string) (model.ProjectVersion, bool, error) {
	versions, err := s.ListProjectVersions(ctx, projectID)
	if err != nil || len(versions) == 0 {
		return model.ProjectVersion{}, false, err
	}
	return versions[0], true, nil
}

func (s *Store) CreateProjectToken(_ context.Context, token model.ProjectToken) (model.ProjectToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token.ID] = token
	return token, nil
}

func (s *Store) ListProjectTokens(_ context.Context, projectID string) ([]model.ProjectToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var tokens []model.ProjectToken
	for _, token := range s.tokens {
		if token.ProjectID == projectID {
			tokens = append(tokens, token)
		}
	}
	sort.Slice(tokens, func(i, j int) bool { return tokens[i].CreatedAt.After(tokens[j].CreatedAt) })
	return tokens, nil
}

func (s *Store) FindProjectTokenByHash(_ context.Context, hash string) (model.ProjectToken, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, token := range s.tokens {
		if token.TokenHash == hash {
			return token, true, nil
		}
	}
	return model.ProjectToken{}, false, nil
}

func (s *Store) UpdateProjectToken(_ context.Context, token model.ProjectToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token.ID] = token
	return nil
}

func (s *Store) AddAuditLog(_ context.Context, entry model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditLogs = append(s.auditLogs, entry)
	sort.Slice(s.auditLogs, func(i, j int) bool { return s.auditLogs[i].CreatedAt.After(s.auditLogs[j].CreatedAt) })
	return nil
}

func (s *Store) ListAuditLogs(_ context.Context, limit int) ([]model.AuditLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.auditLogs) {
		limit = len(s.auditLogs)
	}
	return append([]model.AuditLog(nil), s.auditLogs[:limit]...), nil
}

func (s *Store) FindSymbols(ctx context.Context, projectID, query, kind string, limit int) ([]model.Symbol, *model.ProjectVersion, error) {
	version, ok, err := s.LatestProjectVersion(ctx, projectID)
	if err != nil || !ok {
		return nil, nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	query = strings.ToLower(query)
	var results []model.Symbol
	for _, symbol := range s.symbolsByVersion[version.ID] {
		if kind != "" && symbol.Kind != kind {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(symbol.Name), query) || strings.Contains(strings.ToLower(symbol.Path), query) {
			results = append(results, symbol)
		}
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results, &version, nil
}

func (s *Store) GetSymbol(ctx context.Context, projectID, symbolID string) (model.Symbol, []model.Relation, []model.Relation, *model.ProjectVersion, bool, error) {
	version, ok, err := s.LatestProjectVersion(ctx, projectID)
	if err != nil || !ok {
		return model.Symbol{}, nil, nil, nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var symbol model.Symbol
	found := false
	for _, candidate := range s.symbolsByVersion[version.ID] {
		if candidate.ID == symbolID {
			symbol = candidate
			found = true
			break
		}
	}
	if !found {
		return model.Symbol{}, nil, nil, &version, false, nil
	}
	var inbound []model.Relation
	var outbound []model.Relation
	for _, relation := range s.relationsByVersion[version.ID] {
		if relation.FromSymbolID == symbolID {
			outbound = append(outbound, relation)
		}
		if relation.ToSymbolID == symbolID {
			inbound = append(inbound, relation)
		}
	}
	return symbol, inbound, outbound, &version, true, nil
}

func (s *Store) TraverseCalls(ctx context.Context, projectID, symbolID string, depth int, direction string) ([]model.Symbol, []model.Relation, *model.ProjectVersion, error) {
	version, ok, err := s.LatestProjectVersion(ctx, projectID)
	if err != nil || !ok {
		return nil, nil, nil, err
	}
	if depth <= 0 {
		depth = 1
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	symbolIndex := make(map[string]model.Symbol, len(s.symbolsByVersion[version.ID]))
	for _, symbol := range s.symbolsByVersion[version.ID] {
		symbolIndex[symbol.ID] = symbol
	}
	frontier := []string{symbolID}
	seen := map[string]struct{}{symbolID: {}}
	var results []model.Symbol
	var relations []model.Relation

	for level := 0; level < depth && len(frontier) > 0; level++ {
		var next []string
		for _, current := range frontier {
			for _, relation := range s.relationsByVersion[version.ID] {
				if relation.Type != model.RelationCalls {
					continue
				}
				var candidate string
				if direction == "callers" && relation.ToSymbolID == current {
					candidate = relation.FromSymbolID
				}
				if direction == "callees" && relation.FromSymbolID == current {
					candidate = relation.ToSymbolID
				}
				if candidate == "" {
					continue
				}
				relations = append(relations, relation)
				if _, ok := seen[candidate]; ok {
					continue
				}
				seen[candidate] = struct{}{}
				if symbol, ok := symbolIndex[candidate]; ok {
					results = append(results, symbol)
					next = append(next, candidate)
				}
			}
		}
		frontier = next
	}
	return results, relations, &version, nil
}

func (s *Store) SystemHealth(_ context.Context) (model.SystemHealth, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return model.SystemHealth{
		Storage:     "ok",
		Cache:       "stubbed",
		ObjectStore: "stubbed",
		MCP:         "ready",
		Details: map[string]string{
			"projects":        fmt.Sprintf("%d", len(s.projects)),
			"project_tokens":  fmt.Sprintf("%d", len(s.tokens)),
			"admin_users":     fmt.Sprintf("%d", len(s.admins)),
			"ingestion_runs":  fmt.Sprintf("%d", s.countIngestionRuns()),
			"stored_payloads": fmt.Sprintf("%d", len(s.rawPayloads)),
		},
	}, nil
}

func (s *Store) countIngestionRuns() int {
	total := 0
	for _, runs := range s.ingestionRuns {
		total += len(runs)
	}
	return total
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
