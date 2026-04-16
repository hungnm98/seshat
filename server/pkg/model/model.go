package model

import "time"

type RelationType string

const (
	RelationDeclaredIn RelationType = "declared_in"
	RelationImports    RelationType = "imports"
	RelationContains   RelationType = "contains"
	RelationCalls      RelationType = "calls"
	RelationReferences RelationType = "references"
	RelationImplements RelationType = "implements"
)

type GraphMetadata struct {
	ProjectID     string    `json:"project_id" yaml:"project_id"`
	CommitSHA     string    `json:"commit_sha" yaml:"commit_sha"`
	Branch        string    `json:"branch" yaml:"branch"`
	SchemaVersion string    `json:"schema_version" yaml:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at" yaml:"generated_at"`
	ScanMode      string    `json:"scan_mode,omitempty" yaml:"scan_mode,omitempty"`
}

type File struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Language string `json:"language"`
	Checksum string `json:"checksum"`
}

type Symbol struct {
	ID        string `json:"id"`
	FileID    string `json:"file_id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Language  string `json:"language"`
	Path      string `json:"path"`
	Signature string `json:"signature,omitempty"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	ParentID  string `json:"parent_id,omitempty"`
}

type Relation struct {
	ID           string                 `json:"id"`
	ProjectID    string                 `json:"project_id"`
	FromSymbolID string                 `json:"from_symbol_id"`
	ToSymbolID   string                 `json:"to_symbol_id"`
	Type         RelationType           `json:"type"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type AnalysisBatch struct {
	Metadata  GraphMetadata `json:"metadata"`
	Files     []File        `json:"files"`
	Symbols   []Symbol      `json:"symbols"`
	Relations []Relation    `json:"relations"`
}

type Project struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DefaultBranch string    `json:"default_branch"`
	Description   string    `json:"description,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ProjectVersion struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	CommitSHA  string    `json:"commit_sha"`
	Branch     string    `json:"branch"`
	Status     string    `json:"status"`
	Schema     string    `json:"schema_version"`
	ScannedAt  time.Time `json:"scanned_at"`
	FilesCount int       `json:"files_count"`
	NodesCount int       `json:"nodes_count"`
	EdgesCount int       `json:"edges_count"`
}

type IngestionRun struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	ProjectVerID string    `json:"project_version_id"`
	CommitSHA    string    `json:"commit_sha"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	FinishedAt   time.Time `json:"finished_at"`
}

type ProjectToken struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	Description string     `json:"description"`
	TokenPrefix string     `json:"token_prefix"`
	TokenHash   string     `json:"token_hash,omitempty"`
	Status      string     `json:"status"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   string     `json:"created_by"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	RevokedBy   string     `json:"revoked_by,omitempty"`
}

type ProjectTokenSecret struct {
	Token ProjectToken `json:"token"`
	Plain string       `json:"plain"`
}

type AdminUser struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	LastLoginAt  time.Time `json:"last_login_at,omitempty"`
}

type AdminSession struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AuditLog struct {
	ID         string                 `json:"id"`
	ActorID    string                 `json:"actor_id"`
	ActorName  string                 `json:"actor_name"`
	Action     string                 `json:"action"`
	Resource   string                 `json:"resource"`
	ResourceID string                 `json:"resource_id"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

type QuerySymbolResult struct {
	Symbol   Symbol          `json:"symbol"`
	Inbound  []Relation      `json:"inbound"`
	Outbound []Relation      `json:"outbound"`
	Version  *ProjectVersion `json:"version,omitempty"`
}

type FileDependency struct {
	File      File           `json:"file"`
	Symbols   []Symbol       `json:"symbols"`
	Relations []Relation     `json:"relations"`
	Depth     int            `json:"depth"`
	Reasons   []RelationType `json:"reasons"`
}

type FileDependencyGraph struct {
	File       File             `json:"file"`
	Symbols    []Symbol         `json:"symbols"`
	DependsOn  []FileDependency `json:"depends_on"`
	Dependents []FileDependency `json:"dependents"`
	Relations  []Relation       `json:"relations"`
	Version    *ProjectVersion  `json:"version,omitempty"`
}

type SystemHealth struct {
	Storage     string            `json:"storage"`
	Cache       string            `json:"cache"`
	ObjectStore string            `json:"object_store"`
	MCP         string            `json:"mcp"`
	Details     map[string]string `json:"details"`
}
