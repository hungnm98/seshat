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
	"time"

	"github.com/hungnm98/seshat/internal/parser"
	"github.com/hungnm98/seshat/internal/parser/common"
	"github.com/hungnm98/seshat/pkg/model"
)

var (
	moduleRe = regexp.MustCompile(`^\s*module\s+([A-Z][A-Za-z0-9_:]*)`)
	classRe  = regexp.MustCompile(`^\s*class\s+([A-Z][A-Za-z0-9_:]*)`)
	defRe    = regexp.MustCompile(`^\s*def\s+([a-zA-Z0-9_!?=.]+)`)
	callRe   = regexp.MustCompile(`([a-z_][a-zA-Z0-9_]*)(?:\.|::)([a-z_][a-zA-Z0-9_!?]*)`)
	newRe    = regexp.MustCompile(`([A-Z][A-Za-z0-9_:]*)\.new`)
)

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
	symbols := make(map[string]model.Symbol)

	for _, rel := range files {
		path := filepath.Join(input.RepoPath, rel)
		fileHandle, err := os.Open(path)
		if err != nil {
			return model.AnalysisBatch{}, fmt.Errorf("open %s: %w", rel, err)
		}
		defer fileHandle.Close()

		checksum, err := common.FileChecksum(input.RepoPath, rel)
		if err != nil {
			return model.AnalysisBatch{}, err
		}
		fileID := "file:ruby:" + rel
		batch.Files = append(batch.Files, model.File{
			ID:       fileID,
			Path:     rel,
			Language: "ruby",
			Checksum: checksum,
		})

		var namespace []string
		scanner := bufio.NewScanner(fileHandle)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			switch {
			case moduleRe.MatchString(line):
				name := moduleRe.FindStringSubmatch(line)[1]
				namespace = append(namespace, name)
				id := "symbol:ruby:module:" + strings.Join(namespace, "::")
				symbols[id] = model.Symbol{
					ID:        id,
					FileID:    fileID,
					Kind:      "module",
					Name:      name,
					Language:  "ruby",
					Path:      rel,
					Signature: strings.Join(namespace, "::"),
					LineStart: lineNo,
					LineEnd:   lineNo,
				}
				batch.Relations = append(batch.Relations, rubyRelation("declared", id, fileID, model.RelationDeclaredIn, input.ProjectID))
			case classRe.MatchString(line):
				name := classRe.FindStringSubmatch(line)[1]
				namespace = append(namespace, name)
				id := "symbol:ruby:class:" + strings.Join(namespace, "::")
				symbols[id] = model.Symbol{
					ID:        id,
					FileID:    fileID,
					Kind:      "class",
					Name:      name,
					Language:  "ruby",
					Path:      rel,
					Signature: strings.Join(namespace, "::"),
					LineStart: lineNo,
					LineEnd:   lineNo,
				}
				batch.Relations = append(batch.Relations, rubyRelation("declared", id, fileID, model.RelationDeclaredIn, input.ProjectID))
			case defRe.MatchString(line):
				name := defRe.FindStringSubmatch(line)[1]
				parentSignature := strings.Join(namespace, "::")
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
						batch.Relations = append(batch.Relations, rubyRelation("contains", parentID, methodID, model.RelationContains, input.ProjectID))
					}
				}
				batch.Relations = append(batch.Relations, rubyRelation("declared", methodID, fileID, model.RelationDeclaredIn, input.ProjectID))
			case trimmed == "end":
				if len(namespace) > 0 {
					namespace = namespace[:len(namespace)-1]
				}
			default:
				for _, match := range callRe.FindAllStringSubmatch(line, -1) {
					if len(namespace) == 0 {
						continue
					}
					fromID := "symbol:ruby:class:" + strings.Join(namespace, "::")
					toID := "symbol:ruby:method:" + strings.Join(namespace, "::") + "#" + match[2]
					if _, ok := symbols[toID]; ok {
						batch.Relations = append(batch.Relations, rubyRelation("calls", fromID, toID, model.RelationCalls, input.ProjectID))
					}
				}
				for _, match := range newRe.FindAllStringSubmatch(line, -1) {
					classID := "symbol:ruby:class:" + match[1]
					if _, ok := symbols[classID]; ok && len(namespace) > 0 {
						fromID := "symbol:ruby:class:" + strings.Join(namespace, "::")
						batch.Relations = append(batch.Relations, rubyRelation("refs", fromID, classID, model.RelationReferences, input.ProjectID))
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return model.AnalysisBatch{}, err
		}
	}

	for _, symbol := range symbols {
		batch.Symbols = append(batch.Symbols, symbol)
	}
	sort.Slice(batch.Files, func(i, j int) bool { return batch.Files[i].Path < batch.Files[j].Path })
	sort.Slice(batch.Symbols, func(i, j int) bool { return batch.Symbols[i].ID < batch.Symbols[j].ID })
	sort.Slice(batch.Relations, func(i, j int) bool { return batch.Relations[i].ID < batch.Relations[j].ID })
	return batch, nil
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
