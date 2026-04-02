package storage

import (
	"context"
	"time"

	"github.com/hungnm98/seshat/pkg/model"
)

type Store interface {
	BootstrapAdmin(ctx context.Context, username, name, passwordHash string) (model.AdminUser, error)
	AuthenticateAdmin(ctx context.Context, username string) (model.AdminUser, bool, error)
	UpdateAdminLastLogin(ctx context.Context, userID string, at time.Time) error

	CreateProject(ctx context.Context, project model.Project) (model.Project, error)
	ListProjects(ctx context.Context) ([]model.Project, error)
	GetProject(ctx context.Context, projectID string) (model.Project, bool, error)

	StoreBatch(ctx context.Context, batch model.AnalysisBatch, raw []byte) (model.IngestionRun, model.ProjectVersion, error)
	ListIngestionRuns(ctx context.Context, projectID string) ([]model.IngestionRun, error)
	ListProjectVersions(ctx context.Context, projectID string) ([]model.ProjectVersion, error)
	LatestProjectVersion(ctx context.Context, projectID string) (model.ProjectVersion, bool, error)

	CreateProjectToken(ctx context.Context, token model.ProjectToken) (model.ProjectToken, error)
	ListProjectTokens(ctx context.Context, projectID string) ([]model.ProjectToken, error)
	FindProjectTokenByHash(ctx context.Context, hash string) (model.ProjectToken, bool, error)
	UpdateProjectToken(ctx context.Context, token model.ProjectToken) error

	AddAuditLog(ctx context.Context, entry model.AuditLog) error
	ListAuditLogs(ctx context.Context, limit int) ([]model.AuditLog, error)

	FindSymbols(ctx context.Context, projectID, query, kind string, limit int) ([]model.Symbol, *model.ProjectVersion, error)
	GetSymbol(ctx context.Context, projectID, symbolID string) (model.Symbol, []model.Relation, []model.Relation, *model.ProjectVersion, bool, error)
	TraverseCalls(ctx context.Context, projectID, symbolID string, depth int, direction string) ([]model.Symbol, []model.Relation, *model.ProjectVersion, error)
	SystemHealth(ctx context.Context) (model.SystemHealth, error)
}
