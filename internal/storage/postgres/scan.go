package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hungnm98/seshat/pkg/model"
)

func scanAdminUser(scan func(dest ...interface{}) error) (model.AdminUser, error) {
	var user model.AdminUser
	var lastLogin sql.NullTime
	if err := scan(&user.ID, &user.Username, &user.Name, &user.PasswordHash, &user.CreatedAt, &lastLogin); err != nil {
		return model.AdminUser{}, err
	}
	if lastLogin.Valid {
		user.LastLoginAt = lastLogin.Time
	}
	return user, nil
}

func scanProject(scan func(dest ...interface{}) error) (model.Project, error) {
	var project model.Project
	var description sql.NullString
	if err := scan(&project.ID, &project.Name, &project.DefaultBranch, &description, &project.CreatedAt, &project.UpdatedAt); err != nil {
		return model.Project{}, err
	}
	if description.Valid {
		project.Description = description.String
	}
	return project, nil
}

func scanProjectVersion(scan func(dest ...interface{}) error) (model.ProjectVersion, error) {
	var version model.ProjectVersion
	if err := scan(&version.ID, &version.ProjectID, &version.CommitSHA, &version.Branch, &version.Status, &version.Schema, &version.ScannedAt, &version.FilesCount, &version.NodesCount, &version.EdgesCount); err != nil {
		return model.ProjectVersion{}, err
	}
	return version, nil
}

func scanIngestionRun(scan func(dest ...interface{}) error) (model.IngestionRun, error) {
	var run model.IngestionRun
	var errorMessage sql.NullString
	var finishedAt sql.NullTime
	if err := scan(&run.ID, &run.ProjectID, &run.ProjectVerID, &run.CommitSHA, &run.Status, &errorMessage, &run.CreatedAt, &finishedAt); err != nil {
		return model.IngestionRun{}, err
	}
	if errorMessage.Valid {
		run.ErrorMessage = errorMessage.String
	}
	if finishedAt.Valid {
		run.FinishedAt = finishedAt.Time
	}
	return run, nil
}

func scanProjectToken(scan func(dest ...interface{}) error) (model.ProjectToken, error) {
	var token model.ProjectToken
	var expiresAt sql.NullTime
	var lastUsedAt sql.NullTime
	var revokedAt sql.NullTime
	var revokedBy sql.NullString
	if err := scan(&token.ID, &token.ProjectID, &token.Description, &token.TokenPrefix, &token.TokenHash, &token.Status, &expiresAt, &lastUsedAt, &token.CreatedAt, &token.CreatedBy, &revokedAt, &revokedBy); err != nil {
		return model.ProjectToken{}, err
	}
	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		token.LastUsedAt = &lastUsedAt.Time
	}
	if revokedAt.Valid {
		token.RevokedAt = &revokedAt.Time
	}
	if revokedBy.Valid {
		token.RevokedBy = revokedBy.String
	}
	return token, nil
}

func scanAuditLog(scan func(dest ...interface{}) error) (model.AuditLog, error) {
	var log model.AuditLog
	var metadata []byte
	if err := scan(&log.ID, &log.ActorID, &log.ActorName, &log.Action, &log.Resource, &log.ResourceID, &metadata, &log.CreatedAt); err != nil {
		return model.AuditLog{}, err
	}
	if len(metadata) > 0 && string(metadata) != "null" {
		var decoded map[string]interface{}
		if err := json.Unmarshal(metadata, &decoded); err != nil {
			return model.AuditLog{}, fmt.Errorf("decode audit metadata: %w", err)
		}
		log.Metadata = decoded
	}
	return log, nil
}

func scanSymbol(scan func(dest ...interface{}) error) (model.Symbol, error) {
	var symbol model.Symbol
	var signature sql.NullString
	var parentID sql.NullString
	if err := scan(&symbol.ID, &symbol.FileID, &symbol.Kind, &symbol.Name, &signature, &symbol.Language, &symbol.Path, &symbol.LineStart, &symbol.LineEnd, &parentID); err != nil {
		return model.Symbol{}, err
	}
	if signature.Valid {
		symbol.Signature = signature.String
	}
	if parentID.Valid {
		symbol.ParentID = parentID.String
	}
	return symbol, nil
}

func scanRelation(scan func(dest ...interface{}) error) (model.Relation, error) {
	var relation model.Relation
	var metadata []byte
	if err := scan(&relation.ID, &relation.ProjectID, &relation.FromSymbolID, &relation.ToSymbolID, &relation.Type, &metadata); err != nil {
		return model.Relation{}, err
	}
	if len(metadata) > 0 && string(metadata) != "null" {
		var decoded map[string]interface{}
		if err := json.Unmarshal(metadata, &decoded); err != nil {
			return model.Relation{}, fmt.Errorf("decode relation metadata: %w", err)
		}
		relation.Metadata = decoded
	}
	return relation, nil
}

func marshalMetadata(metadata map[string]interface{}) ([]byte, error) {
	if metadata == nil {
		return []byte(`{}`), nil
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func versionID(projectID, commitSHA string) string {
	return projectID + ":" + commitSHA
}

func execTxHasProject(ctx context.Context, tx *sql.Tx, projectID string) (bool, error) {
	var id string
	err := tx.QueryRowContext(ctx, `select id from projects where id = $1`, projectID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("project %s not found", projectID)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
