package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func registerRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	api := r.Group("/v1")
	_ = api // routes added as features are being built
}
