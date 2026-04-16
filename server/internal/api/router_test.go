package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hungnm98/seshat-server/internal/admin"
	"github.com/hungnm98/seshat-server/internal/auth"
	"github.com/hungnm98/seshat-server/internal/ingestion"
	"github.com/hungnm98/seshat-server/internal/query"
	"github.com/hungnm98/seshat-server/internal/storage/memory"
	"github.com/hungnm98/seshat-server/pkg/logger"
	"github.com/hungnm98/seshat-server/pkg/model"
)

func TestAdminToIngestionAndQueryFlow(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	authSvc := auth.NewService(store, time.Hour)
	if err := admin.SeedBootstrapAdmin(ctx, authSvc, "admin", "admin123"); err != nil {
		t.Fatalf("SeedBootstrapAdmin: %v", err)
	}
	adminSvc, err := admin.NewService(store, authSvc)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	router := NewRouter(ctx, Dependencies{
		Logger:       logger.New(slog.LevelInfo),
		Store:        store,
		AuthService:  authSvc,
		Ingest:       ingestion.NewService(store),
		Query:        query.NewService(store),
		AdminService: adminSvc,
	})

	cookie := loginAdmin(t, router)
	projectID := "proj-omega"

	performRequest(t, router, http.MethodPost, "/admin/projects", url.Values{
		"project_id":     {projectID},
		"name":           {"Omega"},
		"default_branch": {"main"},
		"description":    {"Integration test project"},
	}, "", cookie, http.StatusFound)

	token := createProjectToken(t, router, cookie, projectID)
	verifyToken(t, router, projectID, token)

	analysisBatch := model.AnalysisBatch{
		Metadata: model.GraphMetadata{
			ProjectID:     projectID,
			CommitSHA:     "abc123",
			Branch:        "main",
			SchemaVersion: "v1",
			GeneratedAt:   time.Now().UTC(),
			ScanMode:      "full",
		},
		Files: []model.File{
			{ID: "file:go:internal/order/handler.go", Path: "internal/order/handler.go", Language: "go", Checksum: "sum-0"},
			{ID: "file:go:internal/order/service.go", Path: "internal/order/service.go", Language: "go", Checksum: "sum-1"},
			{ID: "file:go:internal/order/repository.go", Path: "internal/order/repository.go", Language: "go", Checksum: "sum-2"},
		},
		Symbols: []model.Symbol{
			{
				ID:        "symbol:go:order:func:HandleCreateOrder",
				FileID:    "file:go:internal/order/handler.go",
				Kind:      "function",
				Name:      "HandleCreateOrder",
				Language:  "go",
				Path:      "internal/order/handler.go",
				Signature: "HandleCreateOrder",
				LineStart: 5,
				LineEnd:   8,
			},
			{
				ID:        "symbol:go:order:func:CreateOrder",
				FileID:    "file:go:internal/order/service.go",
				Kind:      "function",
				Name:      "CreateOrder",
				Language:  "go",
				Path:      "internal/order/service.go",
				Signature: "CreateOrder",
				LineStart: 10,
				LineEnd:   14,
			},
			{
				ID:        "symbol:go:order:func:Validate",
				FileID:    "file:go:internal/order/service.go",
				Kind:      "function",
				Name:      "Validate",
				Language:  "go",
				Path:      "internal/order/service.go",
				Signature: "Validate",
				LineStart: 16,
				LineEnd:   18,
			},
			{
				ID:        "symbol:go:order:func:SaveOrder",
				FileID:    "file:go:internal/order/repository.go",
				Kind:      "function",
				Name:      "SaveOrder",
				Language:  "go",
				Path:      "internal/order/repository.go",
				Signature: "SaveOrder",
				LineStart: 4,
				LineEnd:   9,
			},
		},
		Relations: []model.Relation{
			{
				ID:           "relation:calls:handler:create-order",
				ProjectID:    projectID,
				FromSymbolID: "symbol:go:order:func:HandleCreateOrder",
				ToSymbolID:   "symbol:go:order:func:CreateOrder",
				Type:         model.RelationCalls,
			},
			{
				ID:           "relation:calls:create-order:validate",
				ProjectID:    projectID,
				FromSymbolID: "symbol:go:order:func:CreateOrder",
				ToSymbolID:   "symbol:go:order:func:Validate",
				Type:         model.RelationCalls,
			},
			{
				ID:           "relation:calls:validate:save-order",
				ProjectID:    projectID,
				FromSymbolID: "symbol:go:order:func:Validate",
				ToSymbolID:   "symbol:go:order:func:SaveOrder",
				Type:         model.RelationCalls,
			},
		},
	}
	ingestBatch(t, router, projectID, token, analysisBatch)

	findSymbolResp := performRequest(t, router, http.MethodGet, "/api/v1/projects/"+projectID+"/symbols?query=CreateOrder", nil, bearer(token), nil, http.StatusOK)
	var findSymbolBody struct {
		Results []model.Symbol `json:"results"`
	}
	decodeJSON(t, findSymbolResp, &findSymbolBody)
	if !hasSymbol(findSymbolBody.Results, "symbol:go:order:func:CreateOrder") {
		t.Fatalf("unexpected search results: %#v", findSymbolBody.Results)
	}

	detailResp := performRequest(t, router, http.MethodGet, "/api/v1/projects/"+projectID+"/symbols/symbol:go:order:func:CreateOrder", nil, bearer(token), nil, http.StatusOK)
	var detailBody model.QuerySymbolResult
	decodeJSON(t, detailResp, &detailBody)
	if detailBody.Symbol.ID != "symbol:go:order:func:CreateOrder" {
		t.Fatalf("unexpected detail symbol: %#v", detailBody.Symbol)
	}
	if len(detailBody.Outbound) != 1 || detailBody.Outbound[0].ToSymbolID != "symbol:go:order:func:Validate" {
		t.Fatalf("expected outbound calls to Validate, got %#v", detailBody.Outbound)
	}

	callersResp := performRequest(t, router, http.MethodGet, "/api/v1/projects/"+projectID+"/graph/callers/symbol:go:order:func:Validate?depth=1", nil, bearer(token), nil, http.StatusOK)
	var callersBody struct {
		Results []model.Symbol `json:"results"`
	}
	decodeJSON(t, callersResp, &callersBody)
	if len(callersBody.Results) != 1 || callersBody.Results[0].ID != "symbol:go:order:func:CreateOrder" {
		t.Fatalf("unexpected callers: %#v", callersBody.Results)
	}

	calleesResp := performRequest(t, router, http.MethodGet, "/api/v1/projects/"+projectID+"/graph/callees/symbol:go:order:func:CreateOrder?depth=1", nil, bearer(token), nil, http.StatusOK)
	var calleesBody struct {
		Results []model.Symbol `json:"results"`
	}
	decodeJSON(t, calleesResp, &calleesBody)
	if len(calleesBody.Results) != 1 || calleesBody.Results[0].ID != "symbol:go:order:func:Validate" {
		t.Fatalf("unexpected callees: %#v", calleesBody.Results)
	}

	dependenciesResp := performRequest(t, router, http.MethodGet, "/api/v1/projects/"+projectID+"/graph/dependencies?file="+url.QueryEscape("internal/order/service.go")+"&depth=1", nil, bearer(token), nil, http.StatusOK)
	var dependenciesBody struct {
		Graph model.FileDependencyGraph `json:"graph"`
	}
	decodeJSON(t, dependenciesResp, &dependenciesBody)
	if dependenciesBody.Graph.File.Path != "internal/order/service.go" {
		t.Fatalf("unexpected dependency root: %#v", dependenciesBody.Graph.File)
	}
	if len(dependenciesBody.Graph.DependsOn) != 1 || dependenciesBody.Graph.DependsOn[0].File.Path != "internal/order/repository.go" {
		t.Fatalf("unexpected depends_on graph: %#v", dependenciesBody.Graph.DependsOn)
	}
	if len(dependenciesBody.Graph.Dependents) != 1 || dependenciesBody.Graph.Dependents[0].File.Path != "internal/order/handler.go" {
		t.Fatalf("unexpected dependents graph: %#v", dependenciesBody.Graph.Dependents)
	}
}

func loginAdmin(t *testing.T, router *gin.Engine) *http.Cookie {
	t.Helper()
	form := url.Values{
		"username": {"admin"},
		"password": {"admin123"},
	}
	resp := performRequest(t, router, http.MethodPost, "/admin/login", form, "", nil, http.StatusFound)
	if len(resp.Result().Cookies()) == 0 {
		t.Fatalf("expected login cookie")
	}
	return resp.Result().Cookies()[0]
}

func createProjectToken(t *testing.T, router *gin.Engine, cookie *http.Cookie, projectID string) string {
	t.Helper()
	resp := performRequest(t, router, http.MethodPost, "/admin/tokens", url.Values{
		"project_id":  {projectID},
		"description": {"ci"},
	}, "", cookie, http.StatusFound)
	location := resp.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	token := parsed.Query().Get("token")
	if token == "" {
		t.Fatalf("expected token in redirect location, got %q", location)
	}
	return token
}

func verifyToken(t *testing.T, router *gin.Engine, projectID, token string) {
	t.Helper()
	resp := performRequest(t, router, http.MethodGet, "/api/v1/auth/verify?project_id="+projectID, nil, bearer(token), nil, http.StatusOK)
	var body struct {
		ProjectID   string `json:"project_id"`
		TokenID     string `json:"token_id"`
		TokenPrefix string `json:"token_prefix"`
		Status      string `json:"status"`
	}
	decodeJSON(t, resp, &body)
	if body.ProjectID != projectID || body.Status != "active" || body.TokenID == "" || body.TokenPrefix == "" {
		t.Fatalf("unexpected token verify payload: %#v", body)
	}
}

func hasSymbol(symbols []model.Symbol, id string) bool {
	for _, symbol := range symbols {
		if symbol.ID == id {
			return true
		}
	}
	return false
}

func ingestBatch(t *testing.T, router *gin.Engine, projectID, token string, batch model.AnalysisBatch) {
	t.Helper()
	resp := performRequest(t, router, http.MethodPost, "/api/v1/projects/"+projectID+"/ingestions", batch, bearer(token), nil, http.StatusCreated)
	var body struct {
		Run     model.IngestionRun   `json:"run"`
		Version model.ProjectVersion `json:"version"`
	}
	decodeJSON(t, resp, &body)
	if body.Run.ProjectID != projectID || body.Version.ProjectID != projectID {
		t.Fatalf("unexpected ingestion payload: %#v", body)
	}
}

func performRequest(t *testing.T, router *gin.Engine, method, path string, body interface{}, authHeader string, cookie *http.Cookie, expectedCode int) *httptest.ResponseRecorder {
	t.Helper()
	var payload string
	switch typed := body.(type) {
	case nil:
		payload = ""
	case string:
		payload = typed
	case url.Values:
		payload = typed.Encode()
	case model.AnalysisBatch:
		raw, err := json.Marshal(typed)
		if err != nil {
			t.Fatalf("marshal analysis batch: %v", err)
		}
		payload = string(raw)
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		payload = string(raw)
	}

	var reader *strings.Reader
	if payload != "" {
		reader = strings.NewReader(payload)
	} else {
		reader = strings.NewReader("")
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		switch body.(type) {
		case url.Values:
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		default:
			req.Header.Set("Content-Type", "application/json")
		}
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != expectedCode {
		t.Fatalf("%s %s expected %d, got %d with body %s", method, path, expectedCode, resp.Code, resp.Body.String())
	}
	return resp
}

func decodeJSON[T any](t *testing.T, resp *httptest.ResponseRecorder, out *T) {
	t.Helper()
	if err := json.Unmarshal(resp.Body.Bytes(), out); err != nil {
		t.Fatalf("decode json: %v body=%s", err, resp.Body.String())
	}
}

func bearer(token string) string {
	return "Bearer " + token
}
