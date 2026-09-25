package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerRoutes(r *gin.Engine) {
	r.GET("/health", s.healthLive)
	r.GET("/health/ready", s.healthReady)

	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", s.authHandler.Register)
		authGroup.POST("/login", s.authHandler.Login)
		authGroup.POST("/refresh", s.authHandler.Refresh)
		authGroup.POST("/logout", s.authHandler.Logout)
	}

	api := r.Group("/v1")
	_ = api
}

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
