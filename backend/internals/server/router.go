package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"pennant/backend/internals/auth"
)

func (s *Server) registerRoutes(r *gin.Engine) {
	// --- Health ---
	r.GET("/health", s.healthLive)
	r.GET("/health/ready", s.healthReady)

	// --- WebSocket ---
	// Auth ide preko ?token=<jwt> query param, jer browser WebSocket API
	// ne može da pošalje Authorization header pri uspostavljanju konekcije.
	// Zato ova ruta NIJE u v1 grupi (koja ima RequireAuth middleware).
	r.GET("/v1/ws/flags", s.wsHandler.Serve)

	// --- Auth ---
	authGroup := r.Group("/auth")
	{
		// javne
		authGroup.POST("/register", s.authHandler.Register)
		authGroup.POST("/login", s.authHandler.Login)
		authGroup.POST("/refresh", s.authHandler.Refresh)
		authGroup.POST("/logout", s.authHandler.Logout)

		// zaštićene (Bearer token)
		authed := authGroup.Group("", auth.RequireAuth(s.cfg))
		authed.POST("/switch-org", s.authHandler.SwitchOrg)
	}

	// --- API v1 ---
	v1 := r.Group("/v1", auth.RequireAuth(s.cfg))

	// Evaluation (org iz JWT-a, ne iz URL-a)
	eval := v1.Group("/evaluate", auth.RequireRole("owner", "editor", "viewer"))
	{
		// /batch MORA biti registrovan pre /:flag_key
		eval.GET("/batch", s.evalHandler.Batch)
		eval.GET("/:flag_key", s.evalHandler.Evaluate)
	}

	// Prihvatanje pozivnice — samo auth, ne treba org access
	v1.POST("/invitations/accept", s.membersHandler.Accept)

	// --- Org-scoped rute ---
	org := v1.Group("/orgs/:org_id", auth.RequireOrgAccess())

	// Read (owner, editor, viewer)
	read := org.Group("", auth.RequireRole("owner", "editor", "viewer"))
	{
		read.GET("/flags", s.flagHandler.List)
		read.GET("/flags/:flag_id", s.flagHandler.Get)
		read.GET("/flags/:flag_id/environments/:env_id/rules", s.targetingHandler.List)
		read.GET("/audit", s.auditHandler.List)
		read.GET("/members", s.membersHandler.List)
	}

	// Write (owner, editor)
	write := org.Group("", auth.RequireRole("owner", "editor"))
	{
		write.POST("/flags", s.flagHandler.Create)
		write.PATCH("/flags/:flag_id", s.flagHandler.Update)
		write.DELETE("/flags/:flag_id", s.flagHandler.Archive)
		write.PATCH("/flags/:flag_id/environments/:env_id", s.flagHandler.UpdateEnvState)
		write.PUT("/flags/:flag_id/environments/:env_id/rules", s.targetingHandler.ReplaceAll)
		write.POST("/flags/:flag_id/environments/:env_id/rules", s.targetingHandler.Add)
		write.DELETE("/flags/:flag_id/environments/:env_id/rules/:rule_id", s.targetingHandler.Delete)
	}

	// Owner-only (member management + invitations)
	owner := org.Group("", auth.RequireRole("owner"))
	{
		owner.POST("/members/invite", s.membersHandler.Invite)
		owner.PATCH("/members/:user_id", s.membersHandler.UpdateRole)
		owner.DELETE("/members/:user_id", s.membersHandler.Remove)
		owner.GET("/invitations", s.membersHandler.ListInvitations)
		owner.DELETE("/invitations/:invite_id", s.membersHandler.RevokeInvite)
	}
}

// --- Health handlers ---

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
