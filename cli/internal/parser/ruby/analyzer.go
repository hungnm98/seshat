package ruby

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
	moduleRe      = regexp.MustCompile(`^\s*module\s+([A-Z][A-Za-z0-9_:]*)`)
	classRe       = regexp.MustCompile(`^\s*class\s+([A-Z][A-Za-z0-9_:]*)(?:\s*<\s*([A-Z][A-Za-z0-9_:]*(?:::[A-Z][A-Za-z0-9_:]*)*))?`)
	defRe         = regexp.MustCompile(`^\s*def\s+([a-zA-Z0-9_!?=.]+)`)
	requireRe     = regexp.MustCompile(`^\s*require(?:_relative)?\s+['"]([^'"]+)['"]`)
	includeRe     = regexp.MustCompile(`^\s*(?:include|extend|prepend)\s+([A-Z][A-Za-z0-9_:]*)`)
	attrRe        = regexp.MustCompile(`^\s*attr_(?:accessor|reader|writer)\s+(.+)`)
	attrSymbolRe  = regexp.MustCompile(`:([a-z_][a-zA-Z0-9_]*)`)
	callRe        = regexp.MustCompile(`\b([a-z_][a-zA-Z0-9_]*)\.([a-z_][a-zA-Z0-9_!?]*)`)
	newRe         = regexp.MustCompile(`\b([A-Z][A-Za-z0-9_:]*)\.new`)
	endLineRe     = regexp.MustCompile(`^\s*end\b`)
	blockOpenerRe = regexp.MustCompile(`^\s*(?:if|unless|while|until|for|begin|case)\b`)
	doBlockRe     = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_])do\s*(?:\|[^|]*\|)?\s*(?:#.*)?$`)
)

type scopeKind int

const (
	scopeModule scopeKind = iota
	scopeClass
	scopeDef
	scopeBlock
)

type scopeEntry struct {
	kind    scopeKind
	name    string
	id      string
	lineNum int
}

type Analyzer struct{}

func New() *Analyzer {
	return &Analyzer{}
}

func (a *Analyzer) Language() string {
	return "ruby"
}

func (a *Analyzer) Analyze(ctx context.Context, input parser.Input) (model.AnalysisBatch, error) {
	_ = ctx
	files, err := common.CollectFilesFromCandidates(input.RepoPath, input.TargetFiles, input.IncludePaths, input.ExcludePaths, map[string]struct{}{".rb": {}})
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

	results, err := analyzeFiles(ctx, input, files)
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

func analyzeFiles(ctx context.Context, input parser.Input, files []string) ([]fileResult, error) {
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
			r, err := analyzeFile(input, rel)
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
				r, err := analyzeFile(input, files[idx])
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

func analyzeFile(input parser.Input, rel string) (fileResult, error) {
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

	fileID := "file:ruby:" + rel
	result := fileResult{
		file: model.File{
			ID:       fileID,
			Path:     rel,
			Language: "ruby",
			Checksum: checksum,
		},
	}

	symbols := make(map[string]model.Symbol)
	var scopes []scopeEntry // scope stack

	// namespace returns the current class/module namespace path
	namespace := func() []string {
		var ns []string
		for _, s := range scopes {
			if s.kind == scopeModule || s.kind == scopeClass {
				ns = append(ns, s.name)
			}
		}
		return ns
	}

	// currentDefID returns the ID of the innermost def scope, or ""
	currentDefID := func() string {
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i].kind == scopeDef {
				return scopes[i].id
			}
		}
		return ""
	}

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()

		switch {
		case moduleRe.MatchString(line):
			name := moduleRe.FindStringSubmatch(line)[1]
			ns := namespace()
			ns = append(ns, name)
			id := "symbol:ruby:module:" + strings.Join(ns, "::")
			symbols[id] = model.Symbol{
				ID:        id,
				FileID:    fileID,
				Kind:      "module",
				Name:      name,
				Language:  "ruby",
				Path:      rel,
				Signature: strings.Join(ns, "::"),
				LineStart: lineNo,
				LineEnd:   lineNo,
			}
			result.relations = append(result.relations, rubyRelation("declared", id, fileID, model.RelationDeclaredIn, input.ProjectID))
			scopes = append(scopes, scopeEntry{kind: scopeModule, name: name, id: id, lineNum: lineNo})

		case classRe.MatchString(line):
			m := classRe.FindStringSubmatch(line)
			name := m[1]
			ns := namespace()
			ns = append(ns, name)
			id := "symbol:ruby:class:" + strings.Join(ns, "::")
			symbols[id] = model.Symbol{
				ID:        id,
				FileID:    fileID,
				Kind:      "class",
				Name:      name,
				Language:  "ruby",
				Path:      rel,
				Signature: strings.Join(ns, "::"),
				LineStart: lineNo,
				LineEnd:   lineNo,
			}
			result.relations = append(result.relations, rubyRelation("declared", id, fileID, model.RelationDeclaredIn, input.ProjectID))
			// Inheritance
			if len(m) > 2 && m[2] != "" {
				superFull := m[2]
				superName := superFull[strings.LastIndex(superFull, "::")+1:]
				if strings.Contains(superFull, "::") {
					superName = superFull
				}
				superID := "symbol:ruby:class:" + superFull
				result.relations = append(result.relations, rubyRelation("inherits", id, superID, model.RelationInherits, input.ProjectID))
				_ = superName
			}
			scopes = append(scopes, scopeEntry{kind: scopeClass, name: name, id: id, lineNum: lineNo})

		case defRe.MatchString(line):
			name := defRe.FindStringSubmatch(line)[1]
			ns := namespace()
			parentSignature := strings.Join(ns, "::")
			methodID := "symbol:ruby:method:" + parentSignature + "#" + name
			symbols[methodID] = model.Symbol{
				ID:        methodID,
				FileID:    fileID,
				Kind:      "method",
				Name:      name,
				Language:  "ruby",
				Path:      rel,
				Signature: parentSignature + "#" + name,
				LineStart: lineNo,
				LineEnd:   lineNo,
			}
			if parentSignature != "" {
				parentID := "symbol:ruby:class:" + parentSignature
				if _, ok := symbols[parentID]; !ok {
					parentID = "symbol:ruby:module:" + parentSignature
				}
				if _, ok := symbols[parentID]; ok {
					result.relations = append(result.relations, rubyRelation("contains", parentID, methodID, model.RelationContains, input.ProjectID))
				}
			}
			result.relations = append(result.relations, rubyRelation("declared", methodID, fileID, model.RelationDeclaredIn, input.ProjectID))
			scopes = append(scopes, scopeEntry{kind: scopeDef, name: name, id: methodID, lineNum: lineNo})
			// Check for one-liner: def foo; body; end — if `end` also appears on same line
			if endLineRe.MatchString(strings.SplitN(line, ";", 2)[len(strings.SplitN(line, ";", 2))-1]) {
				if sym, ok := symbols[methodID]; ok {
					sym.LineEnd = lineNo
					symbols[methodID] = sym
				}
				scopes = scopes[:len(scopes)-1]
			}

		case requireRe.MatchString(line):
			importPath := requireRe.FindStringSubmatch(line)[1]
			importID := "symbol:ruby:require:" + rel + ":" + importPath
			symbols[importID] = model.Symbol{
				ID:        importID,
				FileID:    fileID,
				Kind:      "import",
				Name:      importPath,
				Language:  "ruby",
				Path:      rel,
				Signature: importPath,
				LineStart: lineNo,
				LineEnd:   lineNo,
			}
			result.relations = append(result.relations, rubyRelation("requires", fileID, importID, model.RelationImports, input.ProjectID))

		case includeRe.MatchString(line):
			mixinName := includeRe.FindStringSubmatch(line)[1]
			ns := namespace()
			if len(ns) > 0 {
				hostID := "symbol:ruby:class:" + strings.Join(ns, "::")
				if _, ok := symbols[hostID]; !ok {
					hostID = "symbol:ruby:module:" + strings.Join(ns, "::")
				}
				mixinID := "symbol:ruby:module:" + mixinName
				result.relations = append(result.relations, rubyRelation("includes", hostID, mixinID, model.RelationReferences, input.ProjectID))
			}

		case attrRe.MatchString(line):
			attrArgs := attrRe.FindStringSubmatch(line)[1]
			ns := namespace()
			parentSignature := strings.Join(ns, "::")
			parentID := "symbol:ruby:class:" + parentSignature
			if _, ok := symbols[parentID]; !ok {
				parentID = "symbol:ruby:module:" + parentSignature
			}
			for _, m := range attrSymbolRe.FindAllStringSubmatch(attrArgs, -1) {
				attrName := m[1]
				methodID := "symbol:ruby:method:" + parentSignature + "#" + attrName
				symbols[methodID] = model.Symbol{
					ID:        methodID,
					FileID:    fileID,
					Kind:      "method",
					Name:      attrName,
					Language:  "ruby",
					Path:      rel,
					Signature: parentSignature + "#" + attrName,
					LineStart: lineNo,
					LineEnd:   lineNo,
				}
				if _, ok := symbols[parentID]; ok {
					result.relations = append(result.relations, rubyRelation("contains", parentID, methodID, model.RelationContains, input.ProjectID))
				}
				result.relations = append(result.relations, rubyRelation("declared", methodID, fileID, model.RelationDeclaredIn, input.ProjectID))
			}

		case endLineRe.MatchString(line):
			if len(scopes) > 0 {
				top := scopes[len(scopes)-1]
				scopes = scopes[:len(scopes)-1]
				if top.kind == scopeDef {
					if sym, ok := symbols[top.id]; ok {
						sym.LineEnd = lineNo
						symbols[top.id] = sym
					}
				} else if top.kind == scopeModule || top.kind == scopeClass {
					if sym, ok := symbols[top.id]; ok {
						sym.LineEnd = lineNo
						symbols[top.id] = sym
					}
				}
			}

		default:
			// Track generic block openers for accurate end-matching
			if blockOpenerRe.MatchString(line) || doBlockRe.MatchString(line) {
				scopes = append(scopes, scopeEntry{kind: scopeBlock, lineNum: lineNo})
				continue
			}
			// Call/reference detection — only inside a def
			fromID := currentDefID()
			if fromID == "" {
				continue
			}
			for _, match := range callRe.FindAllStringSubmatch(line, -1) {
				ns := namespace()
				toID := "symbol:ruby:method:" + strings.Join(ns, "::") + "#" + match[2]
				if _, ok := symbols[toID]; ok {
					result.relations = append(result.relations, rubyRelation("calls", fromID, toID, model.RelationCalls, input.ProjectID))
				}
			}
			for _, match := range newRe.FindAllStringSubmatch(line, -1) {
				classID := "symbol:ruby:class:" + match[1]
				if _, ok := symbols[classID]; ok {
					result.relations = append(result.relations, rubyRelation("refs", fromID, classID, model.RelationReferences, input.ProjectID))
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fileResult{}, err
	}

	for _, sym := range symbols {
		result.symbols = append(result.symbols, sym)
	}
	sort.Slice(result.symbols, func(i, j int) bool { return result.symbols[i].ID < result.symbols[j].ID })
	return result, nil
}

func rubyRelation(prefix, from, to string, relationType model.RelationType, projectID string) model.Relation {
	return model.Relation{
		ID:           fmt.Sprintf("relation:ruby:%s:%s:%s:%s", prefix, relationType, from, to),
		ProjectID:    projectID,
		FromSymbolID: from,
		ToSymbolID:   to,
		Type:         relationType,
	}
}
