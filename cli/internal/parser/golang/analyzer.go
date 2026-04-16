package golang

import (
	"context"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hungnm98/seshat-cli/internal/parser"
	"github.com/hungnm98/seshat-cli/internal/parser/common"
	"github.com/hungnm98/seshat-cli/pkg/model"
)

type Analyzer struct{}

func New() *Analyzer {
	return &Analyzer{}
}

func (a *Analyzer) Language() string {
	return "go"
}

func (a *Analyzer) Analyze(ctx context.Context, input parser.Input) (model.AnalysisBatch, error) {
	_ = ctx
	files, err := common.CollectFilesFromCandidates(input.RepoPath, input.TargetFiles, input.IncludePaths, input.ExcludePaths, map[string]struct{}{".go": {}})
	if err != nil {
		return model.AnalysisBatch{}, err
	}
	fset := token.NewFileSet()
	batch := model.AnalysisBatch{
		Metadata: model.GraphMetadata{
			ProjectID:     input.ProjectID,
			CommitSHA:     input.CommitSHA,
			Branch:        input.Branch,
			SchemaVersion: input.SchemaVersion,
			GeneratedAt:   parserNow(),
			ScanMode:      input.ScanMode,
		},
	}
	symbols := make(map[string]model.Symbol)
	filesOut := make([]model.File, 0, len(files))
	type deferredRelation struct {
		from   string
		target string
		kind   model.RelationType
	}
	var deferred []deferredRelation

	for _, rel := range files {
		path := filepath.Join(input.RepoPath, rel)
		parsed, err := goparser.ParseFile(fset, path, nil, goparser.ParseComments)
		if err != nil {
			return model.AnalysisBatch{}, fmt.Errorf("parse %s: %w", rel, err)
		}
		checksum, err := common.FileChecksum(input.RepoPath, rel)
		if err != nil {
			return model.AnalysisBatch{}, err
		}
		fileID := "file:go:" + rel
		filesOut = append(filesOut, model.File{
			ID:       fileID,
			Path:     rel,
			Language: "go",
			Checksum: checksum,
		})

		packageID := "symbol:go:" + parsed.Name.Name + ":package"
		if _, ok := symbols[packageID]; !ok {
			symbols[packageID] = model.Symbol{
				ID:        packageID,
				FileID:    fileID,
				Kind:      "package",
				Name:      parsed.Name.Name,
				Language:  "go",
				Path:      rel,
				Signature: parsed.Name.Name,
				LineStart: lineOf(fset, parsed.Package),
				LineEnd:   lineOf(fset, parsed.Package),
			}
		}

		for _, imp := range parsed.Imports {
			name := importAlias(imp)
			importID := "symbol:go:import:" + parsed.Name.Name + ":" + name
			if _, ok := symbols[importID]; !ok {
				symbols[importID] = model.Symbol{
					ID:        importID,
					FileID:    fileID,
					Kind:      "import",
					Name:      name,
					Language:  "go",
					Path:      rel,
					Signature: strings.Trim(imp.Path.Value, "\""),
					LineStart: lineOf(fset, imp.Pos()),
					LineEnd:   lineOf(fset, imp.End()),
				}
				batch.Relations = append(batch.Relations, relation("imports", packageID, importID, model.RelationImports, input.ProjectID))
			}
		}

		ast.Inspect(parsed, func(node ast.Node) bool {
			switch decl := node.(type) {
			case *ast.FuncDecl:
				symbol := buildFuncSymbol(fset, fileID, rel, parsed.Name.Name, decl)
				symbols[symbol.ID] = symbol
				parent := packageID
				if symbol.ParentID != "" {
					parent = symbol.ParentID
				}
				batch.Relations = append(batch.Relations, relation("declared", symbol.ID, fileID, model.RelationDeclaredIn, input.ProjectID))
				batch.Relations = append(batch.Relations, relation("contains", parent, symbol.ID, model.RelationContains, input.ProjectID))
				if decl.Body != nil {
					callTargets := discoverCallTargets(parsed.Name.Name, decl.Body)
					for _, target := range callTargets {
						deferred = append(deferred, deferredRelation{from: symbol.ID, target: target, kind: model.RelationCalls})
					}
					references := discoverReferences(parsed.Name.Name, decl.Body)
					for _, target := range references {
						deferred = append(deferred, deferredRelation{from: symbol.ID, target: target, kind: model.RelationReferences})
					}
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					typeID := "symbol:go:" + parsed.Name.Name + ":type:" + typeSpec.Name.Name
					symbols[typeID] = model.Symbol{
						ID:        typeID,
						FileID:    fileID,
						Kind:      "type",
						Name:      typeSpec.Name.Name,
						Language:  "go",
						Path:      rel,
						Signature: renderNode(typeSpec.Type),
						LineStart: lineOf(fset, typeSpec.Pos()),
						LineEnd:   lineOf(fset, typeSpec.End()),
						ParentID:  packageID,
					}
					batch.Relations = append(batch.Relations, relation("declared", typeID, fileID, model.RelationDeclaredIn, input.ProjectID))
					batch.Relations = append(batch.Relations, relation("contains", packageID, typeID, model.RelationContains, input.ProjectID))
				}
			}
			return true
		})
	}

	for _, file := range filesOut {
		batch.Files = append(batch.Files, file)
	}
	for _, symbol := range symbols {
		batch.Symbols = append(batch.Symbols, symbol)
	}
	for _, item := range deferred {
		if targetID, ok := resolveDeferredTarget(symbols, item.from, item.target, item.kind); ok {
			prefix := "calls"
			if item.kind == model.RelationReferences {
				prefix = "refs"
			}
			batch.Relations = append(batch.Relations, relation(prefix, item.from, targetID, item.kind, input.ProjectID))
		}
	}
	dedupRelations(&batch.Relations)
	sort.Slice(batch.Files, func(i, j int) bool { return batch.Files[i].Path < batch.Files[j].Path })
	sort.Slice(batch.Symbols, func(i, j int) bool { return batch.Symbols[i].ID < batch.Symbols[j].ID })
	sort.Slice(batch.Relations, func(i, j int) bool { return batch.Relations[i].ID < batch.Relations[j].ID })
	return batch, nil
}

func buildFuncSymbol(fset *token.FileSet, fileID, rel, pkg string, decl *ast.FuncDecl) model.Symbol {
	kind := "function"
	id := "symbol:go:" + pkg + ":func:" + decl.Name.Name
	signature := decl.Name.Name
	parent := "symbol:go:" + pkg + ":package"
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		kind = "method"
		recv := renderNode(decl.Recv.List[0].Type)
		recv = strings.TrimPrefix(recv, "*")
		parent = "symbol:go:" + pkg + ":type:" + recv
		id = "symbol:go:" + pkg + ":method:" + recv + "." + decl.Name.Name
		signature = recv + "." + decl.Name.Name
	}
	return model.Symbol{
		ID:        id,
		FileID:    fileID,
		Kind:      kind,
		Name:      decl.Name.Name,
		Language:  "go",
		Path:      rel,
		Signature: signature,
		LineStart: lineOf(fset, decl.Pos()),
		LineEnd:   lineOf(fset, decl.End()),
		ParentID:  parent,
	}
}

func discoverCallTargets(pkg string, body *ast.BlockStmt) []string {
	targets := make(map[string]struct{})
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			targets[fun.Name] = struct{}{}
		case *ast.SelectorExpr:
			if ident, ok := fun.X.(*ast.Ident); ok {
				targets[ident.Name+"."+fun.Sel.Name] = struct{}{}
			}
		}
		return true
	})
	return sortKeys(targets)
}

func discoverReferences(pkg string, body *ast.BlockStmt) []string {
	refs := make(map[string]struct{})
	ast.Inspect(body, func(node ast.Node) bool {
		switch expr := node.(type) {
		case *ast.CompositeLit:
			switch typed := expr.Type.(type) {
			case *ast.Ident:
				refs[typed.Name] = struct{}{}
			case *ast.SelectorExpr:
				refs[typed.Sel.Name] = struct{}{}
			}
		}
		return true
	})
	return sortKeys(refs)
}

func importAlias(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	parts := strings.Split(strings.Trim(spec.Path.Value, "\""), "/")
	return parts[len(parts)-1]
}

func relation(prefix, from, to string, relationType model.RelationType, projectID string) model.Relation {
	id := fmt.Sprintf("relation:%s:%s:%s:%s", prefix, relationType, from, to)
	return model.Relation{
		ID:           id,
		ProjectID:    projectID,
		FromSymbolID: from,
		ToSymbolID:   to,
		Type:         relationType,
	}
}

func renderNode(node ast.Node) string {
	if node == nil {
		return ""
	}
	switch typed := node.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return "*" + renderNode(typed.X)
	case *ast.SelectorExpr:
		return renderNode(typed.X) + "." + typed.Sel.Name
	case *ast.ArrayType:
		return "[]" + renderNode(typed.Elt)
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	default:
		return fmt.Sprintf("%T", node)
	}
}

func lineOf(fset *token.FileSet, pos token.Pos) int {
	return fset.Position(pos).Line
}

func dedupRelations(relations *[]model.Relation) {
	index := make(map[string]model.Relation)
	for _, relation := range *relations {
		index[relation.ID] = relation
	}
	unique := make([]model.Relation, 0, len(index))
	for _, relation := range index {
		unique = append(unique, relation)
	}
	*relations = unique
}

func sortKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parserNow() time.Time {
	return time.Now().UTC()
}

func resolveDeferredTarget(symbols map[string]model.Symbol, fromID, target string, kind model.RelationType) (string, bool) {
	pkg := packageFromSymbolID(fromID)
	switch kind {
	case model.RelationCalls:
		if strings.Contains(target, ".") {
			parts := strings.SplitN(target, ".", 2)
			candidateSuffixes := []string{
				":method:" + parts[0] + "." + parts[1],
				":method:Repository." + parts[1],
				":method:Service." + parts[1],
			}
			for _, suffix := range candidateSuffixes {
				if id, ok := findBySuffix(symbols, pkg, suffix); ok {
					return id, true
				}
			}
			return "", false
		}
		return findBySuffix(symbols, pkg, ":func:"+target, ":method:Service."+target, ":method:"+target)
	case model.RelationReferences:
		return findBySuffix(symbols, pkg, ":type:"+target)
	default:
		return "", false
	}
}

func packageFromSymbolID(symbolID string) string {
	parts := strings.Split(symbolID, ":")
	if len(parts) > 2 {
		return parts[2]
	}
	return ""
}

func findBySuffix(symbols map[string]model.Symbol, pkg string, suffixes ...string) (string, bool) {
	for _, suffix := range suffixes {
		candidate := "symbol:go:" + pkg + suffix
		if _, ok := symbols[candidate]; ok {
			return candidate, true
		}
	}
	for id := range symbols {
		for _, suffix := range suffixes {
			if strings.HasSuffix(id, suffix) {
				return id, true
			}
		}
	}
	return "", false
}
