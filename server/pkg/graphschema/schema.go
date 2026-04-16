package graphschema

import (
	"fmt"

	"github.com/hungnm98/seshat-server/pkg/model"
)

const Version = "v1"

var allowedRelations = map[model.RelationType]struct{}{
	model.RelationDeclaredIn: {},
	model.RelationImports:    {},
	model.RelationContains:   {},
	model.RelationCalls:      {},
	model.RelationReferences: {},
	model.RelationImplements: {},
}

func Validate(batch model.AnalysisBatch) error {
	if batch.Metadata.ProjectID == "" {
		return fmt.Errorf("metadata.project_id is required")
	}
	if batch.Metadata.SchemaVersion == "" {
		return fmt.Errorf("metadata.schema_version is required")
	}
	fileIDs := make(map[string]struct{}, len(batch.Files))
	for _, file := range batch.Files {
		if file.ID == "" || file.Path == "" || file.Language == "" {
			return fmt.Errorf("file must include id, path and language")
		}
		fileIDs[file.ID] = struct{}{}
	}
	symbolIDs := make(map[string]struct{}, len(batch.Symbols))
	for _, symbol := range batch.Symbols {
		if symbol.ID == "" || symbol.Name == "" || symbol.Kind == "" || symbol.FileID == "" {
			return fmt.Errorf("symbol must include id, name, kind and file_id")
		}
		if _, ok := fileIDs[symbol.FileID]; !ok {
			return fmt.Errorf("symbol %s references unknown file %s", symbol.ID, symbol.FileID)
		}
		symbolIDs[symbol.ID] = struct{}{}
	}
	for _, relation := range batch.Relations {
		if relation.ID == "" || relation.FromSymbolID == "" || relation.ToSymbolID == "" {
			return fmt.Errorf("relation must include id, from_symbol_id and to_symbol_id")
		}
		if _, ok := allowedRelations[relation.Type]; !ok {
			return fmt.Errorf("relation %s has unsupported type %s", relation.ID, relation.Type)
		}
		if _, ok := symbolIDs[relation.FromSymbolID]; !ok {
			return fmt.Errorf("relation %s references unknown from_symbol_id %s", relation.ID, relation.FromSymbolID)
		}
		if _, ok := symbolIDs[relation.ToSymbolID]; !ok {
			return fmt.Errorf("relation %s references unknown to_symbol_id %s", relation.ID, relation.ToSymbolID)
		}
	}
	return nil
}
