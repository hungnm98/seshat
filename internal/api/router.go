package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hungnm98/seshat/internal/admin"
	"github.com/hungnm98/seshat/internal/api/middleware"
	"github.com/hungnm98/seshat/internal/auth"
	"github.com/hungnm98/seshat/internal/ingestion"
	"github.com/hungnm98/seshat/internal/query"
	"github.com/hungnm98/seshat/internal/storage"
	"github.com/hungnm98/seshat/pkg/model"
)

type Dependencies struct {
	Logger       *slog.Logger
	Store        storage.Store
	AuthService  *auth.Service
	Ingest       *ingestion.Service
	Query        *query.Service
	AdminService *admin.Service
}

func NewRouter(ctx context.Context, deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/healthz", func(c *gin.Context) {
		health, err := deps.Store.SystemHealth(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, health)
	})

	api := router.Group("/api/v1")
	api.GET("/auth/verify", middleware.ProjectToken(deps.AuthService), func(c *gin.Context) {
		token := c.MustGet("project_token").(model.ProjectToken)
		c.JSON(http.StatusOK, gin.H{
			"project_id":   token.ProjectID,
			"token_id":     token.ID,
			"token_prefix": token.TokenPrefix,
			"status":       token.Status,
		})
	})

	projectRoutes := api.Group("/projects/:projectID")
	projectRoutes.Use(middleware.ProjectToken(deps.AuthService))
	projectRoutes.POST("/ingestions", func(c *gin.Context) {
		var batch model.AnalysisBatch
		if err := c.ShouldBindJSON(&batch); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if batch.Metadata.ProjectID != c.Param("projectID") {
			c.JSON(http.StatusForbidden, gin.H{"error": "project_id does not match token scope"})
			return
		}
		run, version, err := deps.Ingest.StoreBatch(c.Request.Context(), batch)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"run": run, "version": version})
	})
	projectRoutes.GET("/symbols", func(c *gin.Context) {
		results, version, err := deps.Query.FindSymbol(c.Request.Context(), c.Param("projectID"), c.Query("query"), c.Query("kind"), 50)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": results, "version": version})
	})
	projectRoutes.GET("/symbols/:symbolID", func(c *gin.Context) {
		result, ok, err := deps.Query.GetSymbolDetail(c.Request.Context(), c.Param("projectID"), c.Param("symbolID"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "symbol not found"})
			return
		}
		c.JSON(http.StatusOK, result)
	})
	projectRoutes.GET("/graph/callers/:symbolID", traverseHandler(deps.Query.FindCallers))
	projectRoutes.GET("/graph/callees/:symbolID", traverseHandler(deps.Query.FindCallees))

	deps.AdminService.RegisterRoutes(router, admin.RequireAdminMiddleware(deps.AuthService))
	return router
}

func traverseHandler(fn func(context.Context, string, string, int) ([]model.Symbol, []model.Relation, *model.ProjectVersion, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		depth, _ := strconv.Atoi(c.DefaultQuery("depth", "1"))
		results, relations, version, err := fn(c.Request.Context(), c.Param("projectID"), c.Param("symbolID"), depth)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": results, "relations": relations, "version": version, "generated_at": time.Now().UTC()})
	}
}
