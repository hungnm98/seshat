package main

import (
	"encoding/json"
	"fmt"
)

type tool struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Params      []string `json:"params"`
}

func main() {
	tools := []tool{
		{Name: "seshat_info", Description: "Explain what Seshat MCP does and how an AI should use it", Params: []string{}},
		{Name: "list_projects", Description: "List registered projects and project_id values", Params: []string{}},
		{Name: "find_symbol", Description: "Use after list_projects to find symbols by name or fuzzy query", Params: []string{"project_id", "query", "kind"}},
		{Name: "get_symbol_detail", Description: "Return symbol metadata plus inbound/outbound relations", Params: []string{"project_id", "symbol_id"}},
		{Name: "find_callers", Description: "Traverse incoming calls with bounded depth", Params: []string{"project_id", "symbol_id", "depth"}},
		{Name: "find_callees", Description: "Traverse outgoing calls with bounded depth", Params: []string{"project_id", "symbol_id", "depth"}},
		{Name: "file_dependency_graph", Description: "Use before editing a file to inspect depends_on and dependents impact", Params: []string{"project_id", "file", "depth"}},
	}
	payload, _ := json.MarshalIndent(map[string]interface{}{"tools": tools}, "", "  ")
	fmt.Println(string(payload))
}
