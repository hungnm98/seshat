package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/hungnm98/seshat-cli/internal/localquery"
	"github.com/hungnm98/seshat-cli/pkg/model"
)

const protocolVersion = "2025-06-18"

type Server struct {
	queryProvider func() (*localquery.Service, error)
}

func NewServer(query *localquery.Service) *Server {
	return NewServerWithProvider(func() (*localquery.Service, error) {
		return query, nil
	})
}

func NewServerWithProvider(provider func() (*localquery.Service, error)) *Server {
	return &Server{queryProvider: provider}
}

func (s *Server) Serve(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	encoder := json.NewEncoder(out)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = encoder.Encode(errorResponse(nil, -32700, "parse error", err.Error()))
			continue
		}
		if req.ID == nil && isNotification(req.Method) {
			continue
		}
		resp := s.handle(req)
		if resp != nil {
			if err := encoder.Encode(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

type request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (s *Server) handle(req request) *response {
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return errorResponse(req.ID, -32600, "invalid request", "jsonrpc must be 2.0")
	}
	switch req.Method {
	case "initialize":
		return resultResponse(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    "seshat-local",
				"version": "0.1.0",
			},
		})
	case "ping":
		return resultResponse(req.ID, map[string]any{})
	case "tools/list":
		return resultResponse(req.ID, map[string]any{"tools": tools()})
	case "tools/call":
		result, err := s.callTool(req.Params)
		if err != nil {
			return errorResponse(req.ID, -32602, "invalid tool call", err.Error())
		}
		return resultResponse(req.ID, result)
	default:
		return errorResponse(req.ID, -32601, "method not found", req.Method)
	}
}

func isNotification(method string) bool {
	return method == "notifications/initialized" || method == "initialized"
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) callTool(params json.RawMessage) (map[string]any, error) {
	var call toolCallParams
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, err
	}
	if call.Name == "" {
		return nil, fmt.Errorf("tool name is required")
	}
	args := make(map[string]any)
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, fmt.Errorf("parse arguments: %w", err)
		}
	}
	structured, toolErr, err := s.execute(call.Name, args)
	if err != nil {
		return nil, err
	}
	if toolErr != "" {
		return toolResult(map[string]any{"error": toolErr}, toolErr, true), nil
	}
	text, _ := json.MarshalIndent(structured, "", "  ")
	return toolResult(structured, string(text), false), nil
}

func (s *Server) execute(name string, args map[string]any) (any, string, error) {
	projectID, err := stringArg(args, "project_id", true)
	if err != nil {
		return nil, "", err
	}
	queryService, err := s.queryProvider()
	if err != nil {
		return nil, "", err
	}
	switch name {
	case "find_symbol":
		query, err := stringArg(args, "query", true)
		if err != nil {
			return nil, "", err
		}
		kind, _ := stringArg(args, "kind", false)
		limit := intArg(args, "limit", localquery.DefaultLimit)
		results, version, err := queryService.FindSymbol(projectID, query, kind, limit)
		if err != nil {
			return nil, "", err
		}
		return map[string]any{"results": results, "version": version}, "", nil
	case "get_symbol_detail":
		symbolID, err := stringArg(args, "symbol_id", true)
		if err != nil {
			return nil, "", err
		}
		result, ok, err := queryService.GetSymbolDetail(projectID, symbolID)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			return nil, "symbol not found", nil
		}
		return result, "", nil
	case "find_callers":
		symbolID, err := stringArg(args, "symbol_id", true)
		if err != nil {
			return nil, "", err
		}
		results, relations, version, err := queryService.FindCallers(projectID, symbolID, intArg(args, "depth", localquery.DefaultDepth))
		if err != nil {
			return nil, "", err
		}
		return map[string]any{"results": results, "relations": relations, "version": version}, "", nil
	case "find_callees":
		symbolID, err := stringArg(args, "symbol_id", true)
		if err != nil {
			return nil, "", err
		}
		results, relations, version, err := queryService.FindCallees(projectID, symbolID, intArg(args, "depth", localquery.DefaultDepth))
		if err != nil {
			return nil, "", err
		}
		return map[string]any{"results": results, "relations": relations, "version": version}, "", nil
	case "file_dependency_graph":
		filePath, err := stringArg(args, "file", true)
		if err != nil {
			return nil, "", err
		}
		graph, ok, err := queryService.FileDependencyGraph(projectID, filePath, intArg(args, "depth", localquery.DefaultDepth))
		if err != nil {
			return nil, "", err
		}
		if !ok {
			return nil, "file not found", nil
		}
		graph = trimGraph(graph, stringArgDefault(args, "direction", "both"), intArg(args, "max_files", 25), boolArg(args, "compact", true))
		return map[string]any{"graph": graph}, "", nil
	default:
		return nil, "", fmt.Errorf("unsupported tool %q", name)
	}
}

func resultResponse(id *json.RawMessage, result any) *response {
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id *json.RawMessage, code int, message string, data any) *response {
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message, Data: data}}
}

func toolResult(structured any, text string, isError bool) map[string]any {
	result := map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
	}
	if structured != nil {
		result["structuredContent"] = structured
	}
	if isError {
		result["isError"] = true
	}
	return result
}

func stringArg(args map[string]any, name string, required bool) (string, error) {
	value, ok := args[name]
	if !ok {
		if required {
			return "", fmt.Errorf("%s is required", name)
		}
		return "", nil
	}
	text, ok := value.(string)
	if !ok || text == "" {
		if required {
			return "", fmt.Errorf("%s must be a non-empty string", name)
		}
		return "", nil
	}
	return text, nil
}

func stringArgDefault(args map[string]any, name string, fallback string) string {
	value, err := stringArg(args, name, false)
	if err != nil || value == "" {
		return fallback
	}
	return value
}

func intArg(args map[string]any, name string, fallback int) int {
	value, ok := args[name]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		parsed, err := strconv.Atoi(typed)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func boolArg(args map[string]any, name string, fallback bool) bool {
	value, ok := args[name]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(typed)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "find_symbol",
			"title":       "Find Symbol",
			"description": "Find symbols in the local Seshat index by name, signature, or id.",
			"inputSchema": objectSchema(map[string]any{
				"project_id": stringSchema("Project id from .seshat/project.yaml."),
				"query":      stringSchema("Symbol name, signature, or id fragment."),
				"kind":       stringSchema("Optional symbol kind filter."),
				"limit":      integerSchema("Maximum results, capped by the local query engine."),
			}, []string{"project_id", "query"}),
		},
		{
			"name":        "get_symbol_detail",
			"title":       "Get Symbol Detail",
			"description": "Return symbol metadata with inbound and outbound relations.",
			"inputSchema": objectSchema(map[string]any{
				"project_id": stringSchema("Project id from .seshat/project.yaml."),
				"symbol_id":  stringSchema("Symbol id returned by find_symbol."),
			}, []string{"project_id", "symbol_id"}),
		},
		{
			"name":        "find_callers",
			"title":       "Find Callers",
			"description": "Return symbols that call a target symbol with bounded traversal depth.",
			"inputSchema": objectSchema(map[string]any{
				"project_id": stringSchema("Project id from .seshat/project.yaml."),
				"symbol_id":  stringSchema("Target symbol id."),
				"depth":      integerSchema("Traversal depth, capped at 3."),
			}, []string{"project_id", "symbol_id"}),
		},
		{
			"name":        "find_callees",
			"title":       "Find Callees",
			"description": "Return symbols called by a target symbol with bounded traversal depth.",
			"inputSchema": objectSchema(map[string]any{
				"project_id": stringSchema("Project id from .seshat/project.yaml."),
				"symbol_id":  stringSchema("Target symbol id."),
				"depth":      integerSchema("Traversal depth, capped at 3."),
			}, []string{"project_id", "symbol_id"}),
		},
		{
			"name":        "file_dependency_graph",
			"title":       "File Dependency Graph",
			"description": "Return file-level dependencies and dependents for a project-relative file.",
			"inputSchema": objectSchema(map[string]any{
				"project_id": stringSchema("Project id from .seshat/project.yaml."),
				"file":       stringSchema("Project-relative file path."),
				"depth":      integerSchema("Traversal depth, capped at 3."),
				"direction":  stringSchema("Optional direction: both, depends-on, dependents."),
				"max_files":  integerSchema("Maximum dependency/dependent files returned per response."),
				"compact":    booleanSchema("When true, omit nested symbols and relations from dependency entries."),
			}, []string{"project_id", "file"}),
		},
	}
}

func trimGraph(typed model.FileDependencyGraph, direction string, maxFiles int, compact bool) model.FileDependencyGraph {
	if maxFiles <= 0 || maxFiles > 100 {
		maxFiles = 25
	}
	switch direction {
	case "depends-on":
		typed.Dependents = nil
	case "dependents":
		typed.DependsOn = nil
	}
	if len(typed.DependsOn) > maxFiles {
		typed.DependsOn = typed.DependsOn[:maxFiles]
	}
	if len(typed.Dependents) > maxFiles {
		typed.Dependents = typed.Dependents[:maxFiles]
	}
	typed.Relations = nil
	if compact {
		typed.Symbols = nil
		compactDependencies(typed.DependsOn)
		compactDependencies(typed.Dependents)
	}
	return typed
}

func compactDependencies(dependencies []model.FileDependency) {
	for idx := range dependencies {
		dependencies[idx].Symbols = nil
		dependencies[idx].Relations = nil
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             required,
	}
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func booleanSchema(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}
