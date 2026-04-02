package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hungnm98/seshat/internal/auth"
)

func ProjectToken(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(401, gin.H{"error": "missing bearer token"})
			return
		}
		projectID := c.Param("projectID")
		if projectID == "" {
			projectID = c.Query("project_id")
		}
		token, err := authSvc.VerifyProjectToken(c.Request.Context(), strings.TrimPrefix(header, "Bearer "), projectID)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": err.Error()})
			return
		}
		c.Set("project_token", token)
		c.Next()
	}
}
