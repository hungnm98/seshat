package graphrender

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hungnm98/seshat-cli/pkg/model"
)

type Format string

const (
	FormatMermaid Format = "mermaid"
	FormatDOT     Format = "dot"
	FormatJSON    Format = "json"
)

type Direction string

const (
	DirectionBoth       Direction = "both"
	DirectionDependsOn  Direction = "depends-on"
	DirectionDependents Direction = "dependents"
)

type Options struct {
	Format    Format
	Direction Direction
	MaxNodes  int
}

func Render(graph model.FileDependencyGraph, options Options) (string, error) {
	if options.Format == "" {
		options.Format = FormatMermaid
	}
	if options.Direction == "" {
		options.Direction = DirectionBoth
	}
	if options.MaxNodes <= 0 {
		options.MaxNodes = 25
	}
	view := buildView(graph, options)
	switch options.Format {
	case FormatMermaid:
		return renderMermaid(view), nil
	case FormatDOT:
		return renderDOT(view), nil
	case FormatJSON:
		data, err := json.MarshalIndent(view, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data) + "\n", nil
	default:
		return "", fmt.Errorf("unsupported graph format %q", options.Format)
	}
}

type View struct {
	Root      string `json:"root"`
	Nodes     []Node `json:"nodes"`
	Edges     []Edge `json:"edges"`
	Truncated bool   `json:"truncated"`
}

type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Root  bool   `json:"root,omitempty"`
}

type Edge struct {
	From  string   `json:"from"`
	To    string   `json:"to"`
	Label string   `json:"label,omitempty"`
	Types []string `json:"types,omitempty"`
}

func buildView(graph model.FileDependencyGraph, options Options) View {
	rootID := nodeID(graph.File.Path)
	nodes := map[string]Node{
		rootID: {ID: rootID, Label: graph.File.Path, Root: true},
	}
	edges := make([]Edge, 0)
	addDeps := func(deps []model.FileDependency, reverse bool) {
		for _, dep := range deps {
			if len(nodes) >= options.MaxNodes && nodes[nodeID(dep.File.Path)].ID == "" {
				continue
			}
			id := nodeID(dep.File.Path)
			nodes[id] = Node{ID: id, Label: dep.File.Path}
			label, types := relationLabel(dep.Reasons)
			if reverse {
				edges = append(edges, Edge{From: id, To: rootID, Label: label, Types: types})
			} else {
				edges = append(edges, Edge{From: rootID, To: id, Label: label, Types: types})
			}
		}
	}
	switch options.Direction {
	case DirectionBoth:
		addDeps(graph.DependsOn, false)
		addDeps(graph.Dependents, true)
	case DirectionDependsOn:
		addDeps(graph.DependsOn, false)
	case DirectionDependents:
		addDeps(graph.Dependents, true)
	}
	nodeList := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		nodeList = append(nodeList, node)
	}
	sort.Slice(nodeList, func(i, j int) bool {
		if nodeList[i].Root != nodeList[j].Root {
			return nodeList[i].Root
		}
		return nodeList[i].Label < nodeList[j].Label
	})
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})
	return View{
		Root:      rootID,
		Nodes:     nodeList,
		Edges:     edges,
		Truncated: len(nodes) >= options.MaxNodes && len(graph.DependsOn)+len(graph.Dependents)+1 > len(nodes),
	}
}

func renderMermaid(view View) string {
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	for _, node := range view.Nodes {
		shapeOpen, shapeClose := "[", "]"
		if node.Root {
			shapeOpen, shapeClose = "[[", "]]"
		}
		fmt.Fprintf(&b, "  %s%s\"%s\"%s\n", node.ID, shapeOpen, escapeMermaid(node.Label), shapeClose)
	}
	for _, edge := range view.Edges {
		if edge.Label == "" {
			fmt.Fprintf(&b, "  %s --> %s\n", edge.From, edge.To)
		} else {
			fmt.Fprintf(&b, "  %s -->|%s| %s\n", edge.From, escapeMermaid(edge.Label), edge.To)
		}
	}
	if view.Truncated {
		b.WriteString("  truncated[\"graph truncated by --max-nodes\"]\n")
	}
	return b.String()
}

func renderDOT(view View) string {
	var b strings.Builder
	b.WriteString("digraph seshat_dependencies {\n")
	b.WriteString("  rankdir=LR;\n")
	for _, node := range view.Nodes {
		shape := "box"
		if node.Root {
			shape = "doubleoctagon"
		}
		fmt.Fprintf(&b, "  %s [label=%q shape=%s];\n", node.ID, node.Label, shape)
	}
	for _, edge := range view.Edges {
		if edge.Label == "" {
			fmt.Fprintf(&b, "  %s -> %s;\n", edge.From, edge.To)
		} else {
			fmt.Fprintf(&b, "  %s -> %s [label=%q];\n", edge.From, edge.To, edge.Label)
		}
	}
	b.WriteString("}\n")
	return b.String()
}

func relationLabel(reasons []model.RelationType) (string, []string) {
	types := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		if reason == model.RelationContains || reason == model.RelationDeclaredIn {
			continue
		}
		types = append(types, string(reason))
	}
	sort.Strings(types)
	if len(types) == 0 {
		return "", types
	}
	return strings.Join(types, ","), types
}

var nonID = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func nodeID(path string) string {
	id := nonID.ReplaceAllString(path, "_")
	id = strings.Trim(id, "_")
	if id == "" {
		return "node"
	}
	if id[0] >= '0' && id[0] <= '9' {
		id = "n_" + id
	}
	return id
}

func escapeMermaid(value string) string {
	return strings.ReplaceAll(value, "\"", "'")
}
