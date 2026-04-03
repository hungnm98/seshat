package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/hungnm98/seshat/internal/storage"
	"github.com/hungnm98/seshat/pkg/model"
)

var (
	ErrNotImplemented = errors.New("postgres storage is unavailable because the database handle is not initialized")
)

type Store struct {
	db *sql.DB
}

var _ storage.Store = (*Store)(nil)

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	store := New(db)
	if err := store.Ping(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrNotImplemented
	}
	return s.db.PingContext(ctx)
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrNotImplemented
	}
	for _, stmt := range schemaStatements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply schema statement: %w", err)
		}
	}
	return nil
}

func (s *Store) BootstrapAdmin(ctx context.Context, username, name, passwordHash string) (model.AdminUser, error) {
	existing, err := s.getAdminByUsername(ctx, username)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return model.AdminUser{}, err
	}
	now := time.Now().UTC()
	user := model.AdminUser{
		ID:           "admin:" + username,
		Username:     username,
		Name:         name,
		PasswordHash: passwordHash,
		CreatedAt:    now,
	}
	_, err = s.db.ExecContext(ctx, `
		insert into admin_users (id, username, name, password, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (username) do nothing
	`, user.ID, user.Username, user.Name, user.PasswordHash, user.CreatedAt, user.CreatedAt)
	if err != nil {
		return model.AdminUser{}, err
	}
	return user, nil
}

func (s *Store) AuthenticateAdmin(ctx context.Context, username string) (model.AdminUser, bool, error) {
	user, err := s.getAdminByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return model.AdminUser{}, false, nil
	}
	if err != nil {
		return model.AdminUser{}, false, err
	}
	return user, true, nil
}

func (s *Store) UpdateAdminLastLogin(ctx context.Context, userID string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		update admin_users
		set last_login_at = $2, updated_at = $2
		where id = $1
	`, userID, at.UTC())
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("admin user %s not found", userID)
	}
	return nil
}

func (s *Store) CreateProject(ctx context.Context, project model.Project) (model.Project, error) {
	now := time.Now().UTC()
	project.CreatedAt = now
	project.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		insert into projects (id, name, default_branch, description, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6)
	`, project.ID, project.Name, project.DefaultBranch, project.Description, project.CreatedAt, project.UpdatedAt)
	if err != nil {
		return model.Project{}, err
	}
	return project, nil
}

func (s *Store) ListProjects(ctx context.Context) ([]model.Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, name, default_branch, description, created_at, updated_at
		from projects
		order by id asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []model.Project
	for rows.Next() {
		project, scanErr := scanProject(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, projectID string) (model.Project, bool, error) {
	project, err := s.getProjectByID(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Project{}, false, nil
	}
	if err != nil {
		return model.Project{}, false, err
	}
	return project, true, nil
}

func (s *Store) StoreBatch(ctx context.Context, batch model.AnalysisBatch, raw []byte) (model.IngestionRun, model.ProjectVersion, error) {
	if s == nil || s.db == nil {
		return model.IngestionRun{}, model.ProjectVersion{}, ErrNotImplemented
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = execTxHasProject(ctx, tx, batch.Metadata.ProjectID); err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, err
	}

	version := model.ProjectVersion{
		ID:         versionID(batch.Metadata.ProjectID, batch.Metadata.CommitSHA),
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

	if _, err = tx.ExecContext(ctx, `
		insert into project_versions (id, project_id, commit_sha, branch, status, schema_version, scanned_at, files_count, nodes_count, edges_count)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		on conflict (id) do update set
			project_id = excluded.project_id,
			commit_sha = excluded.commit_sha,
			branch = excluded.branch,
			status = excluded.status,
			schema_version = excluded.schema_version,
			scanned_at = excluded.scanned_at,
			files_count = excluded.files_count,
			nodes_count = excluded.nodes_count,
			edges_count = excluded.edges_count
	`, version.ID, version.ProjectID, version.CommitSHA, version.Branch, version.Status, version.Schema, version.ScannedAt, version.FilesCount, version.NodesCount, version.EdgesCount); err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, err
	}

	if _, err = tx.ExecContext(ctx, `delete from files where project_version_id = $1`, version.ID); err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, err
	}
	if _, err = tx.ExecContext(ctx, `delete from symbols where project_version_id = $1`, version.ID); err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, err
	}
	if _, err = tx.ExecContext(ctx, `delete from relations where project_version_id = $1`, version.ID); err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, err
	}

	for _, file := range batch.Files {
		if _, err = tx.ExecContext(ctx, `
			insert into files (id, project_version_id, path, language, checksum)
			values ($1, $2, $3, $4, $5)
		`, file.ID, version.ID, file.Path, file.Language, file.Checksum); err != nil {
			return model.IngestionRun{}, model.ProjectVersion{}, err
		}
	}
	for _, symbol := range batch.Symbols {
		if _, err = tx.ExecContext(ctx, `
			insert into symbols (id, project_version_id, file_id, kind, name, signature, language, path, line_start, line_end, parent_id)
			values ($1, $2, $3, $4, $5, nullif($6, ''), $7, $8, $9, $10, nullif($11, ''))
		`, symbol.ID, version.ID, symbol.FileID, symbol.Kind, symbol.Name, symbol.Signature, symbol.Language, symbol.Path, symbol.LineStart, symbol.LineEnd, symbol.ParentID); err != nil {
			return model.IngestionRun{}, model.ProjectVersion{}, err
		}
	}
	for _, relation := range batch.Relations {
		meta, err := marshalMetadata(relation.Metadata)
		if err != nil {
			return model.IngestionRun{}, model.ProjectVersion{}, err
		}
		if _, err = tx.ExecContext(ctx, `
			insert into relations (id, project_version_id, from_symbol_id, to_symbol_id, relation_type, metadata_json)
			values ($1, $2, $3, $4, $5, $6)
		`, relation.ID, version.ID, relation.FromSymbolID, relation.ToSymbolID, relation.Type, meta); err != nil {
			return model.IngestionRun{}, model.ProjectVersion{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `
		insert into analysis_batches (project_version_id, raw_payload, created_at)
		values ($1, $2, $3)
		on conflict (project_version_id) do update set raw_payload = excluded.raw_payload, created_at = excluded.created_at
	`, version.ID, raw, time.Now().UTC()); err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, err
	}

	run := model.IngestionRun{
		ID:           "ingestion:" + version.ID + ":" + fmt.Sprintf("%d", time.Now().UTC().UnixNano()),
		ProjectID:    batch.Metadata.ProjectID,
		ProjectVerID: version.ID,
		CommitSHA:    batch.Metadata.CommitSHA,
		Status:       "completed",
		CreatedAt:    time.Now().UTC(),
		FinishedAt:   time.Now().UTC(),
	}
	if _, err = tx.ExecContext(ctx, `
		insert into ingestion_runs (id, project_id, project_version_id, commit_sha, status, error_message, created_at, finished_at)
		values ($1, $2, $3, $4, $5, null, $6, $7)
	`, run.ID, run.ProjectID, run.ProjectVerID, run.CommitSHA, run.Status, run.CreatedAt, run.FinishedAt); err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, err
	}

	if err = tx.Commit(); err != nil {
		return model.IngestionRun{}, model.ProjectVersion{}, err
	}
	return run, version, nil
}

func (s *Store) ListIngestionRuns(ctx context.Context, projectID string) ([]model.IngestionRun, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, project_id, project_version_id, commit_sha, status, error_message, created_at, finished_at
		from ingestion_runs
		where project_id = $1
		order by created_at desc
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []model.IngestionRun
	for rows.Next() {
		run, scanErr := scanIngestionRun(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) ListProjectVersions(ctx context.Context, projectID string) ([]model.ProjectVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, project_id, commit_sha, branch, status, schema_version, scanned_at, files_count, nodes_count, edges_count
		from project_versions
		where project_id = $1
		order by scanned_at desc, id desc
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []model.ProjectVersion
	for rows.Next() {
		version, scanErr := scanProjectVersion(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Store) LatestProjectVersion(ctx context.Context, projectID string) (model.ProjectVersion, bool, error) {
	version, err := s.getLatestProjectVersion(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ProjectVersion{}, false, nil
	}
	if err != nil {
		return model.ProjectVersion{}, false, err
	}
	return version, true, nil
}

func (s *Store) CreateProjectToken(ctx context.Context, token model.ProjectToken) (model.ProjectToken, error) {
	_, err := s.db.ExecContext(ctx, `
		insert into project_tokens
			(id, project_id, description, token_prefix, token_hash, status, expires_at, last_used_at, created_at, created_by, revoked_at, revoked_by)
		values
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, token.ID, token.ProjectID, token.Description, token.TokenPrefix, token.TokenHash, token.Status, token.ExpiresAt, token.LastUsedAt, token.CreatedAt, token.CreatedBy, token.RevokedAt, token.RevokedBy)
	if err != nil {
		return model.ProjectToken{}, err
	}
	return token, nil
}

func (s *Store) ListProjectTokens(ctx context.Context, projectID string) ([]model.ProjectToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, project_id, description, token_prefix, token_hash, status, expires_at, last_used_at, created_at, created_by, revoked_at, revoked_by
		from project_tokens
		where project_id = $1
		order by created_at desc
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []model.ProjectToken
	for rows.Next() {
		token, scanErr := scanProjectToken(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func (s *Store) FindProjectTokenByHash(ctx context.Context, hash string) (model.ProjectToken, bool, error) {
	token, err := s.getTokenByHash(ctx, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ProjectToken{}, false, nil
	}
	if err != nil {
		return model.ProjectToken{}, false, err
	}
	return token, true, nil
}

func (s *Store) UpdateProjectToken(ctx context.Context, token model.ProjectToken) error {
	res, err := s.db.ExecContext(ctx, `
		update project_tokens
		set project_id = $2,
		    description = $3,
		    token_prefix = $4,
		    token_hash = $5,
		    status = $6,
		    expires_at = $7,
		    last_used_at = $8,
		    created_at = $9,
		    created_by = $10,
		    revoked_at = $11,
		    revoked_by = $12
		where id = $1
	`, token.ID, token.ProjectID, token.Description, token.TokenPrefix, token.TokenHash, token.Status, token.ExpiresAt, token.LastUsedAt, token.CreatedAt, token.CreatedBy, token.RevokedAt, token.RevokedBy)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("project token %s not found", token.ID)
	}
	return nil
}

func (s *Store) AddAuditLog(ctx context.Context, entry model.AuditLog) error {
	meta, err := marshalMetadata(entry.Metadata)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		insert into audit_logs (id, actor_id, actor_name, action, resource, resource_id, metadata_json, created_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
	`, entry.ID, entry.ActorID, entry.ActorName, entry.Action, entry.Resource, entry.ResourceID, meta, entry.CreatedAt)
	return err
}

func (s *Store) ListAuditLogs(ctx context.Context, limit int) ([]model.AuditLog, error) {
	query := `
		select id, actor_id, actor_name, action, resource, resource_id, metadata_json, created_at
		from audit_logs
		order by created_at desc
	`
	if limit > 0 {
		query += fmt.Sprintf(" limit %d", limit)
	}
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []model.AuditLog
	for rows.Next() {
		log, scanErr := scanAuditLog(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (s *Store) FindSymbols(ctx context.Context, projectID, queryStr, kind string, limit int) ([]model.Symbol, *model.ProjectVersion, error) {
	version, ok, err := s.LatestProjectVersion(ctx, projectID)
	if err != nil || !ok {
		return nil, nil, err
	}
	where := []string{"project_version_id = $1"}
	args := []interface{}{version.ID}
	if kind != "" {
		where = append(where, fmt.Sprintf("kind = $%d", len(args)+1))
		args = append(args, kind)
	}
	if queryStr != "" {
		where = append(where, fmt.Sprintf("(lower(name) like $%d or lower(path) like $%d or lower(coalesce(signature, '')) like $%d)", len(args)+1, len(args)+1, len(args)+1))
		args = append(args, "%"+strings.ToLower(queryStr)+"%")
	}
	sqlQuery := `
		select id, project_version_id, file_id, kind, name, signature, language, path, line_start, line_end, parent_id
		from symbols
		where ` + strings.Join(where, " and ") + `
		order by path asc, line_start asc, id asc`
	if limit > 0 {
		sqlQuery += fmt.Sprintf(" limit %d", limit)
	}
	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var symbols []model.Symbol
	for rows.Next() {
		symbol, scanErr := scanSymbol(rows.Scan)
		if scanErr != nil {
			return nil, nil, scanErr
		}
		symbols = append(symbols, symbol)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return symbols, &version, nil
}

func (s *Store) GetSymbol(ctx context.Context, projectID, symbolID string) (model.Symbol, []model.Relation, []model.Relation, *model.ProjectVersion, bool, error) {
	version, ok, err := s.LatestProjectVersion(ctx, projectID)
	if err != nil || !ok {
		return model.Symbol{}, nil, nil, nil, false, err
	}
	symbol, err := s.getSymbolByVersion(ctx, version.ID, symbolID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Symbol{}, nil, nil, &version, false, nil
	}
	if err != nil {
		return model.Symbol{}, nil, nil, nil, false, err
	}
	relations, err := s.listRelationsByVersion(ctx, version.ID)
	if err != nil {
		return model.Symbol{}, nil, nil, nil, false, err
	}
	var inbound []model.Relation
	var outbound []model.Relation
	for _, relation := range relations {
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
	relations, err := s.listRelationsByVersion(ctx, version.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	symbols, err := s.listSymbolsByVersion(ctx, version.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	symbolIndex := make(map[string]model.Symbol, len(symbols))
	for _, symbol := range symbols {
		symbolIndex[symbol.ID] = symbol
	}
	frontier := []string{symbolID}
	seen := map[string]struct{}{symbolID: {}}
	relationSeen := make(map[string]struct{})
	var outSymbols []model.Symbol
	var outRelations []model.Relation

	for level := 0; level < depth && len(frontier) > 0; level++ {
		var next []string
		for _, current := range frontier {
			for _, relation := range relations {
				if relation.Type != model.RelationCalls {
					continue
				}
				var candidate string
				switch direction {
				case "callers":
					if relation.ToSymbolID == current {
						candidate = relation.FromSymbolID
					}
				default:
					if relation.FromSymbolID == current {
						candidate = relation.ToSymbolID
					}
				}
				if candidate == "" {
					continue
				}
				if _, ok := relationSeen[relation.ID]; !ok {
					relationSeen[relation.ID] = struct{}{}
					outRelations = append(outRelations, relation)
				}
				if _, ok := seen[candidate]; ok {
					continue
				}
				seen[candidate] = struct{}{}
				if symbol, ok := symbolIndex[candidate]; ok {
					outSymbols = append(outSymbols, symbol)
					next = append(next, candidate)
				}
			}
		}
		frontier = next
	}
	return outSymbols, outRelations, &version, nil
}

func (s *Store) SystemHealth(ctx context.Context) (model.SystemHealth, error) {
	if err := s.Ping(ctx); err != nil {
		return model.SystemHealth{}, err
	}
	counts := []struct {
		key   string
		query string
	}{
		{"projects", `select count(*) from projects`},
		{"project_versions", `select count(*) from project_versions`},
		{"files", `select count(*) from files`},
		{"symbols", `select count(*) from symbols`},
		{"relations", `select count(*) from relations`},
		{"project_tokens", `select count(*) from project_tokens`},
		{"ingestion_runs", `select count(*) from ingestion_runs`},
		{"audit_logs", `select count(*) from audit_logs`},
	}
	details := make(map[string]string, len(counts))
	for _, item := range counts {
		var count int64
		if err := s.db.QueryRowContext(ctx, item.query).Scan(&count); err != nil {
			return model.SystemHealth{}, err
		}
		details[item.key] = fmt.Sprintf("%d", count)
	}
	return model.SystemHealth{
		Storage:     "ok",
		Cache:       "postgres",
		ObjectStore: "postgres",
		MCP:         "ready",
		Details:     details,
	}, nil
}

func (s *Store) getAdminByUsername(ctx context.Context, username string) (model.AdminUser, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, username, name, password, created_at, last_login_at
		from admin_users
		where username = $1
	`, username)
	return scanAdminUser(row.Scan)
}

func (s *Store) getProjectByID(ctx context.Context, projectID string) (model.Project, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, name, default_branch, description, created_at, updated_at
		from projects
		where id = $1
	`, projectID)
	return scanProject(row.Scan)
}

func (s *Store) getLatestProjectVersion(ctx context.Context, projectID string) (model.ProjectVersion, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, project_id, commit_sha, branch, status, schema_version, scanned_at, files_count, nodes_count, edges_count
		from project_versions
		where project_id = $1
		order by scanned_at desc, id desc
		limit 1
	`, projectID)
	return scanProjectVersion(row.Scan)
}

func (s *Store) getTokenByHash(ctx context.Context, hash string) (model.ProjectToken, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, project_id, description, token_prefix, token_hash, status, expires_at, last_used_at, created_at, created_by, revoked_at, revoked_by
		from project_tokens
		where token_hash = $1
	`, hash)
	return scanProjectToken(row.Scan)
}

func (s *Store) getSymbolByVersion(ctx context.Context, versionID, symbolID string) (model.Symbol, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, file_id, kind, name, signature, language, path, line_start, line_end, parent_id
		from symbols
		where project_version_id = $1 and id = $2
	`, versionID, symbolID)
	return scanSymbol(row.Scan)
}

func (s *Store) listSymbolsByVersion(ctx context.Context, versionID string) ([]model.Symbol, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, file_id, kind, name, signature, language, path, line_start, line_end, parent_id
		from symbols
		where project_version_id = $1
		order by path asc, line_start asc, id asc
	`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var symbols []model.Symbol
	for rows.Next() {
		symbol, scanErr := scanSymbol(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		symbols = append(symbols, symbol)
	}
	return symbols, rows.Err()
}

func (s *Store) listRelationsByVersion(ctx context.Context, versionID string) ([]model.Relation, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, project_version_id, from_symbol_id, to_symbol_id, relation_type, metadata_json
		from relations
		where project_version_id = $1
		order by id asc
	`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var relations []model.Relation
	for rows.Next() {
		relation, scanErr := scanRelation(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		relations = append(relations, relation)
	}
	return relations, rows.Err()
}
