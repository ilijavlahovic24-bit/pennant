package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"pennant/backend/internals/auth"
)

func (s *Server) registerRoutes(r *gin.Engine) {
	// ---------- Health ----------
	r.GET("/health", s.healthLive)
	r.GET("/health/ready", s.healthReady)

	// ---------- Auth ----------
	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", s.authHandler.Register)
		authGroup.POST("/login", s.authHandler.Login)
		authGroup.POST("/refresh", s.authHandler.Refresh)
		authGroup.POST("/logout", s.authHandler.Logout)
	}

	// ---------- API v1 ----------
	v1 := r.Group("/v1", auth.RequireAuth(s.cfg))
	// All routes below /v1/orgs/:org_id require that org_id from the URL
	// matches org_id from JWT (RequireOrgAccess).
	org := v1.Group("/orgs/:org_id", auth.RequireOrgAccess())

	// --- Read-only (owner, editor, viewer) ---
	read := org.Group("", auth.RequireRole("owner", "editor", "viewer"))
	{
		read.GET("/flags", s.flagHandler.List)
		read.GET("/flags/:flag_id", s.flagHandler.Get)
		read.GET("/flags/:flag_id/environments/:env_id/rules", s.targetingHandler.List)
		read.GET("/audit", s.auditHandler.List)
	}

	// --- Write (owner, editor) ---
	write := org.Group("", auth.RequireRole("owner", "editor"))
	{
		// Flags
		write.POST("/flags", s.flagHandler.Create)
		write.PATCH("/flags/:flag_id", s.flagHandler.Update)
		write.DELETE("/flags/:flag_id", s.flagHandler.Archive)

		// Flag environment state
		write.PATCH("/flags/:flag_id/environments/:env_id", s.flagHandler.UpdateEnvState)

		// Targeting rules
		write.PUT("/flags/:flag_id/environments/:env_id/rules", s.targetingHandler.ReplaceAll)
		write.POST("/flags/:flag_id/environments/:env_id/rules", s.targetingHandler.Add)
		write.DELETE("/flags/:flag_id/environments/:env_id/rules/:rule_id", s.targetingHandler.Delete)
	}
}

// ---------- Health handlers ----------

func (s *Server) healthLive(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) healthReady(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	resp := gin.H{"status": "ok"}
	status := http.StatusOK

	if err := s.db.Ping(ctx); err != nil {
		resp["postgres"] = "down"
		resp["status"] = "degraded"
		status = http.StatusServiceUnavailable
	} else {
		resp["postgres"] = "ok"
	}

	if err := s.redis.Ping(ctx).Err(); err != nil {
		resp["redis"] = "down"
		resp["status"] = "degraded"
		status = http.StatusServiceUnavailable
	} else {
		resp["redis"] = "ok"
	}

	c.JSON(status, resp)
}
