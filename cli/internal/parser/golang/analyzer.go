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
	"sync"
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
	files, err := common.CollectFilesFromCandidates(input.RepoPath, input.TargetFiles, input.IncludePaths, input.ExcludePaths, map[string]struct{}{".go": {}})
	if err != nil {
		return model.AnalysisBatch{}, err
	}
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
	var deferred []deferredRelation

	results, err := analyzeFiles(ctx, input, files)
	if err != nil {
		return model.AnalysisBatch{}, err
	}
	for _, result := range results {
		filesOut = append(filesOut, result.file)
		for _, symbol := range result.symbols {
			symbols[symbol.ID] = symbol
		}
		batch.Relations = append(batch.Relations, result.relations...)
		deferred = append(deferred, result.deferred...)
	}

	for _, file := range filesOut {
		batch.Files = append(batch.Files, file)
	}
	for _, symbol := range symbols {
		batch.Symbols = append(batch.Symbols, symbol)
	}
	resolver := newSymbolResolver(symbols)
	for _, item := range deferred {
		if targetID, metadata, ok := resolveDeferredTarget(symbols, resolver, item.from, item.target, item.kind, item.metadata); ok {
			prefix := "calls"
			if item.kind == model.RelationReferences {
				prefix = "refs"
			}
			for key, value := range item.metadata {
				if strings.HasPrefix(key, "_") {
					continue
				}
				if metadata == nil {
					metadata = make(map[string]interface{})
				}
				metadata[key] = value
			}
			batch.Relations = append(batch.Relations, relationWithMetadata(prefix, item.from, targetID, item.kind, input.ProjectID, metadata))
		}
	}
	dedupRelations(&batch.Relations)
	sort.Slice(batch.Files, func(i, j int) bool { return batch.Files[i].Path < batch.Files[j].Path })
	sort.Slice(batch.Symbols, func(i, j int) bool { return batch.Symbols[i].ID < batch.Symbols[j].ID })
	sort.Slice(batch.Relations, func(i, j int) bool { return batch.Relations[i].ID < batch.Relations[j].ID })
	return batch, nil
}

type deferredRelation struct {
	from     string
	target   string
	kind     model.RelationType
	metadata map[string]interface{}
}

type fileAnalysis struct {
	index     int
	file      model.File
	symbols   []model.Symbol
	relations []model.Relation
	deferred  []deferredRelation
}

func analyzeFiles(ctx context.Context, input parser.Input, files []string) ([]fileAnalysis, error) {
	parallelism := input.Parallelism
	if parallelism <= 0 {
		parallelism = 1
	}
	if parallelism > len(files) && len(files) > 0 {
		parallelism = len(files)
	}
	if parallelism <= 1 {
		results := make([]fileAnalysis, 0, len(files))
		for i, rel := range files {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			result, err := analyzeFile(input, i, rel)
			if err != nil {
				return nil, err
			}
			results = append(results, result)
		}
		return results, nil
	}

	type job struct {
		index int
		rel   string
	}
	jobs := make(chan job)
	results := make([]fileAnalysis, len(files))
	var firstErr error
	var errMu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if err := ctx.Err(); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					continue
				}
				result, err := analyzeFile(input, item.index, item.rel)
				errMu.Lock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
				} else if firstErr == nil {
					results[item.index] = result
				}
				errMu.Unlock()
			}
		}()
	}
	for i, rel := range files {
		errMu.Lock()
		stopped := firstErr != nil
		errMu.Unlock()
		if stopped {
			break
		}
		jobs <- job{index: i, rel: rel}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

func analyzeFile(input parser.Input, index int, rel string) (fileAnalysis, error) {
	fset := token.NewFileSet()
	path := filepath.Join(input.RepoPath, rel)
	parsed, err := goparser.ParseFile(fset, path, nil, goparser.ParseComments)
	if err != nil {
		return fileAnalysis{}, fmt.Errorf("parse %s: %w", rel, err)
	}
	checksum, err := common.FileChecksum(input.RepoPath, rel)
	if err != nil {
		return fileAnalysis{}, err
	}
	fileID := "file:go:" + rel
	result := fileAnalysis{
		index: index,
		file: model.File{
			ID:       fileID,
			Path:     rel,
			Language: "go",
			Checksum: checksum,
		},
	}

	packageKey := packageKey(parsed.Name.Name, rel)
	packageID := "symbol:go:" + packageKey + ":package"
	imports := make(map[string]string)
	result.symbols = append(result.symbols, model.Symbol{
		ID:        packageID,
		FileID:    fileID,
		Kind:      "package",
		Name:      parsed.Name.Name,
		Language:  "go",
		Path:      rel,
		Signature: packageKey,
		LineStart: lineOf(fset, parsed.Package),
		LineEnd:   lineOf(fset, parsed.Package),
	})

	for _, imp := range parsed.Imports {
		name := importAlias(imp)
		importPath := strings.Trim(imp.Path.Value, "\"")
		imports[name] = importPath
		importID := "symbol:go:import:" + packageKey + ":" + name
		result.symbols = append(result.symbols, model.Symbol{
			ID:        importID,
			FileID:    fileID,
			Kind:      "import",
			Name:      name,
			Language:  "go",
			Path:      rel,
			Signature: importPath,
			LineStart: lineOf(fset, imp.Pos()),
			LineEnd:   lineOf(fset, imp.End()),
		})
		result.relations = append(result.relations, relation("imports", packageID, importID, model.RelationImports, input.ProjectID))
	}

	ast.Inspect(parsed, func(node ast.Node) bool {
		switch decl := node.(type) {
		case *ast.FuncDecl:
			symbol := buildFuncSymbol(fset, fileID, rel, packageKey, decl)
			result.symbols = append(result.symbols, symbol)
			parent := packageID
			if symbol.ParentID != "" {
				parent = symbol.ParentID
			}
			result.relations = append(result.relations, relation("declared", symbol.ID, fileID, model.RelationDeclaredIn, input.ProjectID))
			result.relations = append(result.relations, relation("contains", parent, symbol.ID, model.RelationContains, input.ProjectID))
			if decl.Body != nil {
				callTargets := discoverCallTargets(decl.Body, callContext{
					imports:      imports,
					localTypes:   discoverLocalTypes(decl.Body),
					receiverName: receiverName(decl),
					receiverType: receiverType(decl),
				})
				for _, target := range callTargets {
					result.deferred = append(result.deferred, deferredRelation{from: symbol.ID, target: target.target, kind: model.RelationCalls, metadata: target.metadata})
				}
				references := discoverReferences(decl.Body)
				for _, target := range references {
					result.deferred = append(result.deferred, deferredRelation{from: symbol.ID, target: target, kind: model.RelationReferences})
				}
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				typeID := "symbol:go:" + packageKey + ":type:" + typeSpec.Name.Name
				result.symbols = append(result.symbols, model.Symbol{
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
				})
				result.relations = append(result.relations, relation("declared", typeID, fileID, model.RelationDeclaredIn, input.ProjectID))
				result.relations = append(result.relations, relation("contains", packageID, typeID, model.RelationContains, input.ProjectID))
			}
		}
		return true
	})
	return result, nil
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

type callContext struct {
	imports      map[string]string
	localTypes   map[string]string
	receiverName string
	receiverType string
}

type callTarget struct {
	target   string
	metadata map[string]interface{}
}

func discoverCallTargets(body *ast.BlockStmt, ctx callContext) []callTarget {
	targets := make(map[string]callTarget)
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			targets[fun.Name] = callTarget{target: fun.Name}
		case *ast.SelectorExpr:
			if rendered := renderSelector(fun); rendered != "" {
				targets[rendered] = callTarget{target: rendered, metadata: callMetadata(rendered, ctx)}
			}
		}
		return true
	})
	keys := make([]string, 0, len(targets))
	for key := range targets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]callTarget, 0, len(keys))
	for _, key := range keys {
		out = append(out, targets[key])
	}
	return out
}

func discoverReferences(body *ast.BlockStmt) []string {
	refs := make(map[string]struct{})
	ast.Inspect(body, func(node ast.Node) bool {
		switch expr := node.(type) {
		case *ast.CompositeLit:
			switch typed := expr.Type.(type) {
			case *ast.Ident:
				refs[typed.Name] = struct{}{}
			case *ast.SelectorExpr:
				refs[renderNode(typed)] = struct{}{}
			}
		}
		return true
	})
	return sortKeys(refs)
}

func callMetadata(target string, ctx callContext) map[string]interface{} {
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		return nil
	}
	metadata := make(map[string]interface{})
	qualifier := parts[0]
	if importPath := ctx.imports[qualifier]; importPath != "" {
		metadata["_import_alias"] = qualifier
		metadata["_import_path"] = importPath
	}
	if ctx.receiverName != "" {
		metadata["_receiver_var"] = ctx.receiverName
		metadata["_receiver_type"] = ctx.receiverType
	}
	if receiverType := ctx.localTypes[qualifier]; receiverType != "" {
		metadata["_receiver_type_hint"] = receiverType
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func discoverLocalTypes(body *ast.BlockStmt) map[string]string {
	types := make(map[string]string)
	ast.Inspect(body, func(node ast.Node) bool {
		switch stmt := node.(type) {
		case *ast.AssignStmt:
			for i, lhs := range stmt.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name == "_" {
					continue
				}
				if i >= len(stmt.Rhs) {
					continue
				}
				if typeName := typeNameFromExpr(stmt.Rhs[i]); typeName != "" {
					types[ident.Name] = typeName
				}
			}
		case *ast.ValueSpec:
			for i, name := range stmt.Names {
				if name.Name == "_" {
					continue
				}
				if typeName := typeNameFromExpr(stmt.Type); typeName != "" {
					types[name.Name] = typeName
					continue
				}
				if i < len(stmt.Values) {
					if typeName := typeNameFromExpr(stmt.Values[i]); typeName != "" {
						types[name.Name] = typeName
					}
				}
			}
		}
		return true
	})
	return types
}

func typeNameFromExpr(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return typeNameFromExpr(typed.X)
	case *ast.SelectorExpr:
		return typed.Sel.Name
	case *ast.CompositeLit:
		return typeNameFromExpr(typed.Type)
	case *ast.UnaryExpr:
		return typeNameFromExpr(typed.X)
	default:
		return ""
	}
}

func importAlias(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	parts := strings.Split(strings.Trim(spec.Path.Value, "\""), "/")
	return parts[len(parts)-1]
}

func receiverName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 || len(decl.Recv.List[0].Names) == 0 {
		return ""
	}
	return decl.Recv.List[0].Names[0].Name
}

func receiverType(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return ""
	}
	return strings.TrimPrefix(renderNode(decl.Recv.List[0].Type), "*")
}

func relation(prefix, from, to string, relationType model.RelationType, projectID string) model.Relation {
	return relationWithMetadata(prefix, from, to, relationType, projectID, nil)
}

func relationWithMetadata(prefix, from, to string, relationType model.RelationType, projectID string, metadata map[string]interface{}) model.Relation {
	id := fmt.Sprintf("relation:%s:%s:%s:%s", prefix, relationType, from, to)
	return model.Relation{
		ID:           id,
		ProjectID:    projectID,
		FromSymbolID: from,
		ToSymbolID:   to,
		Type:         relationType,
		Metadata:     metadata,
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

func renderSelector(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		left := renderSelector(typed.X)
		if left == "" {
			return typed.Sel.Name
		}
		return left + "." + typed.Sel.Name
	default:
		return ""
	}
}

func packageKey(packageName, rel string) string {
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." || dir == "" {
		return packageName
	}
	return packageName + "@" + dir
}

func resolveDeferredTarget(symbols map[string]model.Symbol, resolver *symbolResolver, fromID, target string, kind model.RelationType, hints map[string]interface{}) (string, map[string]interface{}, bool) {
	pkg := packageFromSymbolID(fromID)
	switch kind {
	case model.RelationCalls:
		if strings.Contains(target, ".") {
			parts := strings.Split(target, ".")
			if len(parts) < 2 {
				return "", nil, false
			}
			receiver := parts[len(parts)-2]
			method := parts[len(parts)-1]
			candidateSuffixes := []string{
				":method:" + receiver + "." + method,
				":method:" + lowerFirst(receiver) + "." + method,
				":method:Repository." + method,
				":method:Service." + method,
			}
			for _, suffix := range candidateSuffixes {
				if id, ok := findBySuffix(symbols, pkg, suffix); ok {
					return id, nil, true
				}
			}
			if len(parts) == 2 && metadataString(hints, "_receiver_var") == parts[0] {
				if id, ok := findMethodInPackageByReceiver(symbols, pkg, metadataString(hints, "_receiver_type"), method); ok {
					return id, map[string]interface{}{
						"resolution": "receiver_method",
						"selector":   target,
						"receiver":   metadataString(hints, "_receiver_type"),
					}, true
				}
			}
			if len(parts) == 2 {
				if id, ok := resolver.findPackageFunctionByImportPath(metadataString(hints, "_import_path"), method); ok {
					return id, map[string]interface{}{
						"resolution": "heuristic_import_alias_function",
						"selector":   target,
						"package":    metadataString(hints, "_import_alias"),
					}, true
				}
				if id, ok := resolver.findMethodByExactReceiver(metadataString(hints, "_receiver_type_hint"), method); ok {
					return id, map[string]interface{}{
						"resolution": "heuristic_local_variable_method",
						"selector":   target,
						"receiver":   metadataString(hints, "_receiver_type_hint"),
					}, true
				}
				if looksLikeLowerCamel(parts[0]) {
					if id, ok := resolver.findMethodByExactReceiver(upperFirst(parts[0]), method); ok {
						return id, map[string]interface{}{
							"resolution": "heuristic_local_variable_method",
							"selector":   target,
							"receiver":   upperFirst(parts[0]),
						}, true
					}
				}
			}
			if id, ok := resolver.findMethodByReceiver(receiverCandidates(parts[:len(parts)-1]), method); ok {
				return id, map[string]interface{}{
					"resolution": "heuristic_selector_method",
					"selector":   target,
					"receiver":   receiver,
				}, true
			}
			if id, ok := resolver.findPackageFunctionTarget(parts[0], method); ok {
				return id, map[string]interface{}{
					"resolution": "heuristic_package_function",
					"selector":   target,
					"package":    parts[0],
				}, true
			}
			if len(parts) >= 3 {
				if id, ok := resolver.findHeuristicMethodTarget(method); ok {
					return id, map[string]interface{}{
						"resolution": "heuristic_selector_method",
						"selector":   target,
					}, true
				}
			}
			return "", nil, false
		}
		if id, ok := findBySuffix(symbols, pkg, ":func:"+target, ":method:Service."+target, ":method:"+target); ok {
			return id, nil, true
		}
		return "", nil, false
	case model.RelationReferences:
		if strings.Contains(target, ".") {
			return "", nil, false
		}
		if id, ok := findBySuffix(symbols, pkg, ":type:"+target); ok {
			return id, nil, true
		}
		return "", nil, false
	default:
		return "", nil, false
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
	return "", false
}

func findMethodInPackageByReceiver(symbols map[string]model.Symbol, pkg, receiver, method string) (string, bool) {
	if receiver == "" {
		return "", false
	}
	return findBySuffix(symbols, pkg, ":method:"+receiver+"."+method)
}

type symbolResolver struct {
	methodsByName         map[string][]model.Symbol
	methodsByReceiverName map[string][]model.Symbol
	functionsByName       map[string][]model.Symbol
}

func newSymbolResolver(symbols map[string]model.Symbol) *symbolResolver {
	resolver := &symbolResolver{
		methodsByName:         make(map[string][]model.Symbol),
		methodsByReceiverName: make(map[string][]model.Symbol),
		functionsByName:       make(map[string][]model.Symbol),
	}
	for _, symbol := range symbols {
		switch symbol.Kind {
		case "method":
			resolver.methodsByName[symbol.Name] = append(resolver.methodsByName[symbol.Name], symbol)
			if receiver, _, ok := strings.Cut(symbol.Signature, "."); ok {
				key := receiverMethodKey(receiver, symbol.Name)
				resolver.methodsByReceiverName[key] = append(resolver.methodsByReceiverName[key], symbol)
			}
		case "function":
			resolver.functionsByName[symbol.Name] = append(resolver.functionsByName[symbol.Name], symbol)
		}
	}
	for key := range resolver.methodsByName {
		sortSymbolSlice(resolver.methodsByName[key])
	}
	for key := range resolver.methodsByReceiverName {
		sortSymbolSlice(resolver.methodsByReceiverName[key])
	}
	for key := range resolver.functionsByName {
		sortSymbolSlice(resolver.functionsByName[key])
	}
	return resolver
}

func sortSymbolSlice(symbols []model.Symbol) {
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Path == symbols[j].Path {
			return symbols[i].ID < symbols[j].ID
		}
		return symbols[i].Path < symbols[j].Path
	})
}

func receiverMethodKey(receiver, method string) string {
	return receiver + "\x00" + method
}

func (r *symbolResolver) findMethodByExactReceiver(receiver, method string) (string, bool) {
	if receiver == "" {
		return "", false
	}
	for _, symbol := range r.methodsByReceiverName[receiverMethodKey(receiver, method)] {
		if excludedCallTarget(symbol) {
			continue
		}
		return symbol.ID, true
	}
	return "", false
}

func (r *symbolResolver) findMethodByReceiver(receivers []string, method string) (string, bool) {
	receiverSet := make(map[string]struct{}, len(receivers))
	for _, receiver := range receivers {
		if receiver == "" {
			continue
		}
		receiverSet[receiver] = struct{}{}
		receiverSet[lowerFirst(receiver)] = struct{}{}
	}
	var candidates []model.Symbol
	for _, symbol := range r.methodsByName[method] {
		if excludedCallTarget(symbol) {
			continue
		}
		receiver, _, ok := strings.Cut(symbol.Signature, ".")
		if !ok {
			continue
		}
		if _, ok := receiverSet[receiver]; !ok {
			continue
		}
		candidates = append(candidates, symbol)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	if len(candidates) == 0 {
		return "", false
	}
	return candidates[0].ID, true
}

func (r *symbolResolver) findPackageFunctionByImportPath(importPath, name string) (string, bool) {
	if importPath == "" {
		return "", false
	}
	var candidates []model.Symbol
	for _, symbol := range r.functionsByName[name] {
		if excludedCallTarget(symbol) {
			continue
		}
		if !importPathMatchesSymbol(importPath, symbol.ID) {
			continue
		}
		candidates = append(candidates, symbol)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	if len(candidates) == 0 {
		return "", false
	}
	return candidates[0].ID, true
}

func (r *symbolResolver) findPackageFunctionTarget(qualifier, name string) (string, bool) {
	var candidates []model.Symbol
	for _, symbol := range r.functionsByName[name] {
		if excludedCallTarget(symbol) {
			continue
		}
		if !packageQualifierMatchesSymbol(symbol.ID, qualifier) {
			continue
		}
		candidates = append(candidates, symbol)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	if len(candidates) == 0 {
		return "", false
	}
	return candidates[0].ID, true
}

func importPathMatchesSymbol(importPath, symbolID string) bool {
	pkg := packageFromSymbolID(symbolID)
	if pkg == "" {
		return false
	}
	name, dir, hasDir := strings.Cut(pkg, "@")
	if importPath == name || strings.HasSuffix(importPath, "/"+name) {
		return true
	}
	if hasDir {
		return importPath == dir || strings.HasSuffix(importPath, "/"+dir)
	}
	return false
}

func (r *symbolResolver) findHeuristicMethodTarget(method string) (string, bool) {
	var candidates []model.Symbol
	for _, symbol := range r.methodsByName[method] {
		if excludedCallTarget(symbol) {
			continue
		}
		candidates = append(candidates, symbol)
	}
	sort.Slice(candidates, func(i, j int) bool {
		iLower := receiverLooksPrivate(candidates[i].Signature)
		jLower := receiverLooksPrivate(candidates[j].Signature)
		if iLower != jLower {
			return iLower
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) == 0 {
		return "", false
	}
	return candidates[0].ID, true
}

func excludedCallTarget(symbol model.Symbol) bool {
	return strings.HasSuffix(symbol.Path, "_test.go") ||
		strings.Contains(symbol.Signature, "Mock") ||
		strings.Contains(symbol.ID, "_mock")
}

func receiverCandidates(parts []string) []string {
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		out = append(out, part)
		seen[part] = struct{}{}
	}
	return out
}

func lowerFirst(value string) string {
	if value == "" {
		return ""
	}
	first := value[0]
	if first < 'A' || first > 'Z' {
		return value
	}
	return string(first+'a'-'A') + value[1:]
}

func upperFirst(value string) string {
	if value == "" {
		return ""
	}
	first := value[0]
	if first < 'a' || first > 'z' {
		return value
	}
	return string(first-'a'+'A') + value[1:]
}

func looksLikeLowerCamel(value string) bool {
	if value == "" {
		return false
	}
	first := value[0]
	if first < 'a' || first > 'z' {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] >= 'A' && value[i] <= 'Z' {
			return true
		}
	}
	return false
}

func packageQualifierMatchesSymbol(symbolID, qualifier string) bool {
	pkg := packageFromSymbolID(symbolID)
	if pkg == "" || qualifier == "" {
		return false
	}
	name, dir, hasDir := strings.Cut(pkg, "@")
	if qualifier == name {
		return true
	}
	if hasDir && qualifier == filepath.Base(dir) {
		return true
	}
	return false
}

func metadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return value
}

func receiverLooksPrivate(signature string) bool {
	receiver, _, ok := strings.Cut(signature, ".")
	if !ok || receiver == "" {
		return false
	}
	first := receiver[0]
	return first >= 'a' && first <= 'z'
}
