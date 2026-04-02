package admin

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hungnm98/seshat/internal/auth"
	"github.com/hungnm98/seshat/internal/storage"
	"github.com/hungnm98/seshat/pkg/model"
)

const SessionCookieName = "seshat_admin_session"

//go:embed templates/*.tmpl
var templateFS embed.FS

type Service struct {
	store     storage.Store
	auth      *auth.Service
	templates *template.Template
}

func NewService(store storage.Store, authSvc *auth.Service) (*Service, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, err
	}
	return &Service{
		store:     store,
		auth:      authSvc,
		templates: tmpl,
	}, nil
}

func (s *Service) RegisterRoutes(router *gin.Engine, requireAdmin gin.HandlerFunc) {
	admin := router.Group("/admin")
	admin.GET("/login", s.renderLoginPage)
	admin.POST("/login", s.handleLogin)
	admin.GET("/logout", s.handleLogout)

	protected := admin.Group("/")
	protected.Use(requireAdmin)
	protected.GET("/", s.dashboardPage)
	protected.GET("/projects", s.projectsPage)
	protected.POST("/projects", s.createProject)
	protected.GET("/tokens", s.tokensPage)
	protected.POST("/tokens", s.createToken)
	protected.POST("/tokens/:tokenID/revoke", s.revokeToken)
	protected.GET("/ingestions", s.ingestionsPage)
	protected.GET("/audit", s.auditPage)
}

func (s *Service) renderLoginPage(c *gin.Context) {
	s.render(c.Writer, "login.tmpl", gin.H{
		"Title": "Seshat Admin Login",
	})
}

func (s *Service) handleLogin(c *gin.Context) {
	session, _, err := s.auth.LoginAdmin(c.Request.Context(), c.PostForm("username"), c.PostForm("password"))
	if err != nil {
		s.render(c.Writer, "login.tmpl", gin.H{
			"Title": "Seshat Admin Login",
			"Error": "Invalid credentials",
		})
		return
	}
	c.SetCookie(SessionCookieName, session.ID, int(time.Until(session.ExpiresAt).Seconds()), "/", "", false, true)
	c.Redirect(http.StatusFound, "/admin/")
}

func (s *Service) handleLogout(c *gin.Context) {
	if sessionID, err := c.Cookie(SessionCookieName); err == nil {
		s.auth.Logout(sessionID)
	}
	c.SetCookie(SessionCookieName, "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/admin/login")
}

func (s *Service) dashboardPage(c *gin.Context) {
	projects, _ := s.store.ListProjects(c.Request.Context())
	auditLogs, _ := s.store.ListAuditLogs(c.Request.Context(), 10)
	health, _ := s.store.SystemHealth(c.Request.Context())
	var latestIngestions []model.IngestionRun
	for _, project := range projects {
		runs, _ := s.store.ListIngestionRuns(c.Request.Context(), project.ID)
		if len(runs) > 0 {
			latestIngestions = append(latestIngestions, runs[0])
		}
	}
	sort.Slice(latestIngestions, func(i, j int) bool { return latestIngestions[i].CreatedAt.After(latestIngestions[j].CreatedAt) })
	s.render(c.Writer, "dashboard.tmpl", gin.H{
		"Title":            "Seshat Admin Dashboard",
		"Projects":         projects,
		"RecentIngestions": latestIngestions,
		"AuditLogs":        auditLogs,
		"Health":           health,
	})
}

func (s *Service) projectsPage(c *gin.Context) {
	projects, _ := s.store.ListProjects(c.Request.Context())
	s.render(c.Writer, "projects.tmpl", gin.H{
		"Title":    "Projects",
		"Projects": projects,
	})
}

func (s *Service) createProject(c *gin.Context) {
	project := model.Project{
		ID:            c.PostForm("project_id"),
		Name:          c.PostForm("name"),
		DefaultBranch: c.PostForm("default_branch"),
		Description:   c.PostForm("description"),
	}
	if project.DefaultBranch == "" {
		project.DefaultBranch = "main"
	}
	created, err := s.store.CreateProject(c.Request.Context(), project)
	if err == nil {
		_ = s.store.AddAuditLog(c.Request.Context(), model.AuditLog{
			ID:         fmt.Sprintf("audit:%d", time.Now().UTC().UnixNano()),
			ActorID:    adminUserID(c),
			ActorName:  adminUsername(c),
			Action:     "project.create",
			Resource:   "project",
			ResourceID: created.ID,
			CreatedAt:  time.Now().UTC(),
		})
	}
	c.Redirect(http.StatusFound, "/admin/projects")
}

func (s *Service) tokensPage(c *gin.Context) {
	projects, _ := s.store.ListProjects(c.Request.Context())
	selectedProjectID := c.Query("project_id")
	var tokens []model.ProjectToken
	var showProject model.Project
	if selectedProjectID != "" {
		tokens, _ = s.store.ListProjectTokens(c.Request.Context(), selectedProjectID)
		showProject, _, _ = s.store.GetProject(c.Request.Context(), selectedProjectID)
	}
	s.render(c.Writer, "tokens.tmpl", gin.H{
		"Title":           "Project Tokens",
		"Projects":        projects,
		"Tokens":          tokens,
		"SelectedProject": showProject,
		"FlashToken":      c.Query("token"),
	})
}

func (s *Service) createToken(c *gin.Context) {
	projectID := c.PostForm("project_id")
	secret, err := s.auth.CreateProjectToken(c.Request.Context(), projectID, c.PostForm("description"), adminUsername(c), nil)
	if err == nil {
		_ = s.store.AddAuditLog(c.Request.Context(), model.AuditLog{
			ID:         fmt.Sprintf("audit:%d", time.Now().UTC().UnixNano()),
			ActorID:    adminUserID(c),
			ActorName:  adminUsername(c),
			Action:     "token.create",
			Resource:   "project_token",
			ResourceID: secret.Token.ID,
			CreatedAt:  time.Now().UTC(),
			Metadata: map[string]interface{}{
				"project_id": projectID,
				"prefix":     secret.Token.TokenPrefix,
			},
		})
		c.Redirect(http.StatusFound, "/admin/tokens?project_id="+projectID+"&token="+secret.Plain)
		return
	}
	c.Redirect(http.StatusFound, "/admin/tokens?project_id="+projectID)
}

func (s *Service) revokeToken(c *gin.Context) {
	tokenID := c.Param("tokenID")
	projectID := c.PostForm("project_id")
	if err := s.auth.RevokeProjectToken(c.Request.Context(), tokenID, adminUsername(c)); err == nil {
		_ = s.store.AddAuditLog(c.Request.Context(), model.AuditLog{
			ID:         fmt.Sprintf("audit:%d", time.Now().UTC().UnixNano()),
			ActorID:    adminUserID(c),
			ActorName:  adminUsername(c),
			Action:     "token.revoke",
			Resource:   "project_token",
			ResourceID: tokenID,
			CreatedAt:  time.Now().UTC(),
		})
	}
	c.Redirect(http.StatusFound, "/admin/tokens?project_id="+projectID)
}

func (s *Service) ingestionsPage(c *gin.Context) {
	projects, _ := s.store.ListProjects(c.Request.Context())
	var runs []model.IngestionRun
	for _, project := range projects {
		projectRuns, _ := s.store.ListIngestionRuns(c.Request.Context(), project.ID)
		runs = append(runs, projectRuns...)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	s.render(c.Writer, "ingestions.tmpl", gin.H{
		"Title": "Ingestion Runs",
		"Runs":  runs,
	})
}

func (s *Service) auditPage(c *gin.Context) {
	logs, _ := s.store.ListAuditLogs(c.Request.Context(), 50)
	s.render(c.Writer, "audit.tmpl", gin.H{
		"Title": "Audit Log",
		"Logs":  logs,
	})
}

func (s *Service) render(writer http.ResponseWriter, name string, data gin.H) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, "layout.tmpl", mergeData(data, gin.H{"BodyTemplate": name})); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = writer.Write(buf.Bytes())
}

func mergeData(values ...gin.H) gin.H {
	out := gin.H{}
	for _, value := range values {
		for key, item := range value {
			out[key] = item
		}
	}
	return out
}

func adminUsername(c *gin.Context) string {
	if value, ok := c.Get("admin_username"); ok {
		return value.(string)
	}
	return "unknown"
}

func adminUserID(c *gin.Context) string {
	if value, ok := c.Get("admin_user_id"); ok {
		return value.(string)
	}
	return "unknown"
}

func RequireAdminMiddleware(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie(SessionCookieName)
		if err != nil {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}
		session, ok := authSvc.GetSession(sessionID)
		if !ok {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}
		c.Set("admin_user_id", session.UserID)
		c.Set("admin_username", session.Username)
		c.Next()
	}
}

func SeedBootstrapAdmin(ctx context.Context, authSvc *auth.Service, username, password string) error {
	_, err := authSvc.BootstrapAdmin(ctx, username, "Seshat Administrator", password)
	return err
}
