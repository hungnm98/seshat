package javascript

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hungnm98/seshat-cli/internal/parser"
	"github.com/hungnm98/seshat-cli/internal/parser/common"
	"github.com/hungnm98/seshat-cli/pkg/model"
)

var (
	importRe    = regexp.MustCompile(`^\s*import\s+(?:[^'"]+from\s+)?['"]([^'"]+)['"]`)
	requireRe   = regexp.MustCompile(`\brequire\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	classRe     = regexp.MustCompile(`^\s*(?:export\s+(?:default\s+)?)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)(?:\s+extends\s+([A-Za-z_$][A-Za-z0-9_$.]*))?`)
	funcDeclRe  = regexp.MustCompile(`^\s*(?:export\s+(?:default\s+)?)?(?:async\s+)?function\s*\*?\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*[(<]`)
	arrowFuncRe = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s+)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>`)
	funcExprRe  = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s+)?function`)
	methodRe    = regexp.MustCompile(`^\s+(?:(?:async|static|get|set|private|public|protected|readonly|override|abstract|declare)\s+)*([A-Za-z_$#][A-Za-z0-9_$]*)\s*[(<]`)
	interfaceRe = regexp.MustCompile(`^\s*(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	typeAliasRe = regexp.MustCompile(`^\s*(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>=]*>)?\s*=`)
	enumRe      = regexp.MustCompile(`^\s*(?:export\s+)?(?:const\s+)?enum\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	namespaceRe = regexp.MustCompile(`^\s*(?:export\s+)?(?:namespace|module)\s+([A-Za-z_$][A-Za-z0-9_.]+)\s*\{`)
	memberCallRe = regexp.MustCompile(`\bthis\.([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
)

var jsKeywords = map[string]struct{}{
	"if": {}, "else": {}, "for": {}, "while": {}, "do": {}, "switch": {},
	"case": {}, "return": {}, "throw": {}, "try": {}, "catch": {}, "finally": {},
	"new": {}, "delete": {}, "typeof": {}, "instanceof": {}, "void": {},
	"break": {}, "continue": {}, "import": {}, "export": {}, "from": {},
	"async": {}, "await": {}, "yield": {}, "function": {}, "class": {},
	"const": {}, "let": {}, "var": {}, "of": {}, "in": {}, "with": {},
	"super": {}, "extends": {}, "implements": {}, "interface": {},
	"enum": {}, "type": {}, "namespace": {}, "module": {}, "declare": {},
	"abstract": {}, "override": {}, "static": {}, "get": {}, "set": {},
}

type scopeKind int

const (
	scopeClass scopeKind = iota
	scopeFunction
	scopeNamespace
	scopeBlock
)

type scopeEntry struct {
	kind    scopeKind
	name    string
	id      string
	lineNum int
	depth   int // brace depth before this scope's opening brace
}

// Analyzer handles JavaScript or TypeScript files.
type Analyzer struct {
	lang       string
	extensions map[string]struct{}
}

func NewJS() *Analyzer {
	return &Analyzer{
		lang:       "javascript",
		extensions: map[string]struct{}{".js": {}, ".jsx": {}},
	}
}

func NewTS() *Analyzer {
	return &Analyzer{
		lang:       "typescript",
		extensions: map[string]struct{}{".ts": {}, ".tsx": {}},
	}
}

func (a *Analyzer) Language() string {
	return a.lang
}

func (a *Analyzer) Analyze(ctx context.Context, input parser.Input) (model.AnalysisBatch, error) {
	_ = ctx
	files, err := common.CollectFilesFromCandidates(input.RepoPath, input.TargetFiles, input.IncludePaths, input.ExcludePaths, a.extensions)
	if err != nil {
		return model.AnalysisBatch{}, err
	}
	batch := model.AnalysisBatch{
		Metadata: model.GraphMetadata{
			ProjectID:     input.ProjectID,
			CommitSHA:     input.CommitSHA,
			Branch:        input.Branch,
			SchemaVersion: input.SchemaVersion,
			GeneratedAt:   time.Now().UTC(),
			ScanMode:      input.ScanMode,
		},
	}

	results, err := analyzeFiles(ctx, input, files, a.lang)
	if err != nil {
		return model.AnalysisBatch{}, err
	}

	symbolIndex := make(map[string]model.Symbol)
	for _, r := range results {
		batch.Files = append(batch.Files, r.file)
		for _, sym := range r.symbols {
			symbolIndex[sym.ID] = sym
		}
		batch.Relations = append(batch.Relations, r.relations...)
	}
	for _, sym := range symbolIndex {
		batch.Symbols = append(batch.Symbols, sym)
	}

	sort.Slice(batch.Files, func(i, j int) bool { return batch.Files[i].Path < batch.Files[j].Path })
	sort.Slice(batch.Symbols, func(i, j int) bool { return batch.Symbols[i].ID < batch.Symbols[j].ID })
	sort.Slice(batch.Relations, func(i, j int) bool { return batch.Relations[i].ID < batch.Relations[j].ID })
	return batch, nil
}

type fileResult struct {
	file      model.File
	symbols   []model.Symbol
	relations []model.Relation
}

func analyzeFiles(ctx context.Context, input parser.Input, files []string, lang string) ([]fileResult, error) {
	parallelism := input.Parallelism
	if parallelism <= 0 {
		parallelism = 1
	}
	if parallelism > len(files) && len(files) > 0 {
		parallelism = len(files)
	}

	if parallelism <= 1 {
		results := make([]fileResult, 0, len(files))
		for _, rel := range files {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			r, err := analyzeFile(input, rel, lang)
			if err != nil {
				return nil, err
			}
			results = append(results, r)
		}
		return results, nil
	}

	jobs := make(chan int, len(files))
	for i := range files {
		jobs <- i
	}
	close(jobs)

	out := make([]fileResult, len(files))
	errs := make([]error, len(files))
	var wg sync.WaitGroup
	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					errs[idx] = ctx.Err()
					continue
				}
				r, err := analyzeFile(input, files[idx], lang)
				out[idx] = r
				errs[idx] = err
			}
		}()
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func analyzeFile(input parser.Input, rel string, lang string) (fileResult, error) {
	path := filepath.Join(input.RepoPath, rel)
	f, err := os.Open(path)
	if err != nil {
		return fileResult{}, fmt.Errorf("open %s: %w", rel, err)
	}
	defer f.Close()

	checksum, err := common.FileChecksum(input.RepoPath, rel)
	if err != nil {
		return fileResult{}, err
	}

	fileID := "file:" + lang + ":" + rel
	ext := filepath.Ext(rel)
	modulePath := strings.TrimSuffix(filepath.ToSlash(rel), ext)

	result := fileResult{
		file: model.File{
			ID:       fileID,
			Path:     rel,
			Language: lang,
			Checksum: checksum,
		},
	}

	symbols := make(map[string]model.Symbol)
	var scopes []scopeEntry
	braceDepth := 0

	type deferredCall struct {
		fromID     string
		className  string
		methodName string
	}
	var deferredCalls []deferredCall

	// Returns the innermost class scope ID and name, and whether we are
	// exactly one brace level deep inside it (direct class body).
	innermostClass := func() (id string, name string, direct bool) {
		for i := len(scopes) - 1; i >= 0; i-- {
			s := scopes[i]
			if s.kind == scopeClass {
				return s.id, s.name, braceDepth == s.depth+1
			}
			if s.kind == scopeFunction || s.kind == scopeNamespace {
				return "", "", false
			}
		}
		return "", "", false
	}

	innermostFunc := func() string {
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i].kind == scopeFunction {
				return scopes[i].id
			}
		}
		return ""
	}

	// enclosingClass finds the nearest class scope, looking past function scopes.
	// Used for this.method() call attribution.
	enclosingClass := func() (id string, name string) {
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i].kind == scopeClass {
				return scopes[i].id, scopes[i].name
			}
		}
		return "", ""
	}

	addSymbol := func(sym model.Symbol) {
		symbols[sym.ID] = sym
		result.relations = append(result.relations,
			jsRelation("declared", sym.ID, fileID, model.RelationDeclaredIn, input.ProjectID, lang))
	}

	scanner := bufio.NewScanner(f)
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		stripped := stripLineComment(line)
		braceDelta := countNetBraces(stripped)
		scopeDepth := braceDepth // depth before processing this line

		matched := false

		switch {
		case importRe.MatchString(stripped):
			if m := importRe.FindStringSubmatch(stripped); m != nil {
				importPath := m[1]
				importID := "symbol:" + lang + ":import:" + rel + ":" + importPath
				sym := model.Symbol{
					ID: importID, FileID: fileID, Kind: "import",
					Name: importPath, Language: lang, Path: rel,
					Signature: importPath, LineStart: lineNo, LineEnd: lineNo,
				}
				symbols[importID] = sym
				result.relations = append(result.relations,
					jsRelation("imports", fileID, importID, model.RelationImports, input.ProjectID, lang))
			}
			matched = true

		case requireRe.MatchString(stripped) && !strings.HasPrefix(strings.TrimSpace(stripped), "import"):
			for _, m := range requireRe.FindAllStringSubmatch(stripped, -1) {
				importPath := m[1]
				importID := "symbol:" + lang + ":import:" + rel + ":" + importPath
				sym := model.Symbol{
					ID: importID, FileID: fileID, Kind: "import",
					Name: importPath, Language: lang, Path: rel,
					Signature: importPath, LineStart: lineNo, LineEnd: lineNo,
				}
				symbols[importID] = sym
				result.relations = append(result.relations,
					jsRelation("imports", fileID, importID, model.RelationImports, input.ProjectID, lang))
			}
			matched = true

		case classRe.MatchString(stripped):
			m := classRe.FindStringSubmatch(stripped)
			className := m[1]
			classID := "symbol:" + lang + ":class:" + modulePath + ":" + className
			sym := model.Symbol{
				ID: classID, FileID: fileID, Kind: "class",
				Name: className, Language: lang, Path: rel,
				Signature: modulePath + ":" + className, LineStart: lineNo, LineEnd: lineNo,
			}
			addSymbol(sym)
			if len(m) > 2 && m[2] != "" {
				superID := "symbol:" + lang + ":class:" + modulePath + ":" + m[2]
				result.relations = append(result.relations,
					jsRelation("inherits", classID, superID, model.RelationInherits, input.ProjectID, lang))
			}
			scopes = append(scopes, scopeEntry{kind: scopeClass, name: className, id: classID, lineNum: lineNo, depth: scopeDepth})
			matched = true

		case funcDeclRe.MatchString(stripped):
			m := funcDeclRe.FindStringSubmatch(stripped)
			funcName := m[1]
			funcID := "symbol:" + lang + ":func:" + modulePath + ":" + funcName
			sym := model.Symbol{
				ID: funcID, FileID: fileID, Kind: "function",
				Name: funcName, Language: lang, Path: rel,
				Signature: modulePath + ":" + funcName, LineStart: lineNo, LineEnd: lineNo,
			}
			addSymbol(sym)
			scopes = append(scopes, scopeEntry{kind: scopeFunction, name: funcName, id: funcID, lineNum: lineNo, depth: scopeDepth})
			matched = true

		case arrowFuncRe.MatchString(stripped):
			m := arrowFuncRe.FindStringSubmatch(stripped)
			funcName := m[1]
			funcID := "symbol:" + lang + ":func:" + modulePath + ":" + funcName
			sym := model.Symbol{
				ID: funcID, FileID: fileID, Kind: "function",
				Name: funcName, Language: lang, Path: rel,
				Signature: modulePath + ":" + funcName, LineStart: lineNo, LineEnd: lineNo,
			}
			addSymbol(sym)
			if braceDelta > 0 {
				scopes = append(scopes, scopeEntry{kind: scopeFunction, name: funcName, id: funcID, lineNum: lineNo, depth: scopeDepth})
			}
			matched = true

		case funcExprRe.MatchString(stripped):
			m := funcExprRe.FindStringSubmatch(stripped)
			funcName := m[1]
			funcID := "symbol:" + lang + ":func:" + modulePath + ":" + funcName
			sym := model.Symbol{
				ID: funcID, FileID: fileID, Kind: "function",
				Name: funcName, Language: lang, Path: rel,
				Signature: modulePath + ":" + funcName, LineStart: lineNo, LineEnd: lineNo,
			}
			addSymbol(sym)
			if braceDelta > 0 {
				scopes = append(scopes, scopeEntry{kind: scopeFunction, name: funcName, id: funcID, lineNum: lineNo, depth: scopeDepth})
			}
			matched = true

		case interfaceRe.MatchString(stripped):
			m := interfaceRe.FindStringSubmatch(stripped)
			name := m[1]
			ifaceID := "symbol:" + lang + ":interface:" + modulePath + ":" + name
			sym := model.Symbol{
				ID: ifaceID, FileID: fileID, Kind: "interface",
				Name: name, Language: lang, Path: rel,
				Signature: modulePath + ":" + name, LineStart: lineNo, LineEnd: lineNo,
			}
			addSymbol(sym)
			if braceDelta > 0 {
				scopes = append(scopes, scopeEntry{kind: scopeBlock, name: name, id: ifaceID, lineNum: lineNo, depth: scopeDepth})
			}
			matched = true

		case typeAliasRe.MatchString(stripped):
			m := typeAliasRe.FindStringSubmatch(stripped)
			name := m[1]
			typeID := "symbol:" + lang + ":type:" + modulePath + ":" + name
			sym := model.Symbol{
				ID: typeID, FileID: fileID, Kind: "type",
				Name: name, Language: lang, Path: rel,
				Signature: modulePath + ":" + name, LineStart: lineNo, LineEnd: lineNo,
			}
			addSymbol(sym)
			matched = true

		case enumRe.MatchString(stripped):
			m := enumRe.FindStringSubmatch(stripped)
			name := m[1]
			enumID := "symbol:" + lang + ":enum:" + modulePath + ":" + name
			sym := model.Symbol{
				ID: enumID, FileID: fileID, Kind: "enum",
				Name: name, Language: lang, Path: rel,
				Signature: modulePath + ":" + name, LineStart: lineNo, LineEnd: lineNo,
			}
			addSymbol(sym)
			if braceDelta > 0 {
				scopes = append(scopes, scopeEntry{kind: scopeBlock, name: name, id: enumID, lineNum: lineNo, depth: scopeDepth})
			}
			matched = true

		case namespaceRe.MatchString(stripped):
			m := namespaceRe.FindStringSubmatch(stripped)
			name := m[1]
			nsID := "symbol:" + lang + ":namespace:" + modulePath + ":" + name
			sym := model.Symbol{
				ID: nsID, FileID: fileID, Kind: "namespace",
				Name: name, Language: lang, Path: rel,
				Signature: modulePath + ":" + name, LineStart: lineNo, LineEnd: lineNo,
			}
			addSymbol(sym)
			scopes = append(scopes, scopeEntry{kind: scopeNamespace, name: name, id: nsID, lineNum: lineNo, depth: scopeDepth})
			matched = true
		}

		// Method detection: only when we are directly inside a class body.
		if !matched {
			classID, className, direct := innermostClass()
			if direct && methodRe.MatchString(stripped) {
				methodName := extractMethodName(stripped)
				if methodName != "" && !isKeyword(methodName) {
					methodID := "symbol:" + lang + ":method:" + modulePath + ":" + className + "#" + methodName
					sym := model.Symbol{
						ID: methodID, FileID: fileID, Kind: "method",
						Name: methodName, Language: lang, Path: rel,
						Signature: modulePath + ":" + className + "#" + methodName,
						LineStart: lineNo, LineEnd: lineNo,
					}
					symbols[methodID] = sym
					result.relations = append(result.relations,
						jsRelation("declared", methodID, fileID, model.RelationDeclaredIn, input.ProjectID, lang))
					result.relations = append(result.relations,
						jsRelation("contains", classID, methodID, model.RelationContains, input.ProjectID, lang))
					if braceDelta > 0 {
						scopes = append(scopes, scopeEntry{kind: scopeFunction, name: methodName, id: methodID, lineNum: lineNo, depth: scopeDepth})
					}
				}
			}
		}

		// this.method() call detection inside function scopes — deferred to resolve
		// forward references (callee may be declared later in the same file).
		if fromID := innermostFunc(); fromID != "" {
			_, className := enclosingClass()
			if className != "" {
				for _, m := range memberCallRe.FindAllStringSubmatch(stripped, -1) {
					deferredCalls = append(deferredCalls, deferredCall{
						fromID:     fromID,
						className:  className,
						methodName: m[1],
					})
				}
			}
		}

		// Update brace depth, then close any scopes whose depth is now reached.
		braceDepth += braceDelta
		for len(scopes) > 0 && braceDepth <= scopes[len(scopes)-1].depth {
			top := scopes[len(scopes)-1]
			scopes = scopes[:len(scopes)-1]
			if sym, ok := symbols[top.id]; ok {
				sym.LineEnd = lineNo
				symbols[top.id] = sym
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fileResult{}, err
	}

	// Resolve deferred this.method() calls now that all symbols are known.
	seen := make(map[string]struct{})
	for _, dc := range deferredCalls {
		toID := "symbol:" + lang + ":method:" + modulePath + ":" + dc.className + "#" + dc.methodName
		if _, ok := symbols[toID]; !ok {
			continue
		}
		key := dc.fromID + "→" + toID
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		result.relations = append(result.relations,
			jsRelation("calls", dc.fromID, toID, model.RelationCalls, input.ProjectID, lang))
	}

	for _, sym := range symbols {
		result.symbols = append(result.symbols, sym)
	}
	sort.Slice(result.symbols, func(i, j int) bool { return result.symbols[i].ID < result.symbols[j].ID })
	return result, nil
}

func jsRelation(prefix, from, to string, relationType model.RelationType, projectID, lang string) model.Relation {
	return model.Relation{
		ID:           fmt.Sprintf("relation:%s:%s:%s:%s:%s", lang, prefix, relationType, from, to),
		ProjectID:    projectID,
		FromSymbolID: from,
		ToSymbolID:   to,
		Type:         relationType,
	}
}

// stripLineComment removes everything after // that is outside a string literal.
func stripLineComment(line string) string {
	inStr := rune(0)
	for i := 0; i < len(line); i++ {
		ch := rune(line[i])
		if inStr != 0 {
			if ch == inStr && (i == 0 || line[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			inStr = ch
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return line[:i]
			}
		}
	}
	return line
}

// countNetBraces counts { minus } in a line, skipping string literals and // comments.
func countNetBraces(line string) int {
	inStr := rune(0)
	count := 0
	for i := 0; i < len(line); i++ {
		ch := rune(line[i])
		if inStr != 0 {
			if ch == inStr && (i == 0 || line[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			inStr = ch
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return count
			}
		case '{':
			count++
		case '}':
			count--
		}
	}
	return count
}

var modifierPrefixes = []string{
	"async", "static", "get", "set", "private", "public", "protected",
	"readonly", "override", "abstract", "declare",
}

// extractMethodName strips leading whitespace and modifier keywords, returning
// the first identifier before ( or <.
func extractMethodName(line string) string {
	s := strings.TrimSpace(line)
	for {
		found := false
		for _, mod := range modifierPrefixes {
			prefix := mod + " "
			if strings.HasPrefix(s, prefix) {
				s = strings.TrimSpace(s[len(prefix):])
				found = true
			}
		}
		if !found {
			break
		}
	}
	re := regexp.MustCompile(`^#?([A-Za-z_$][A-Za-z0-9_$]*)`)
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func isKeyword(name string) bool {
	_, ok := jsKeywords[name]
	return ok
}
