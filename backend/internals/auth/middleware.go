package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"pennant/backend/internals/config"
)

const (
	ctxUserID = "auth.user_id"
	ctxOrgID  = "auth.org_id"
	ctxRole   = "auth.role"
)

// RequireAuth parsira Bearer token i stavlja claims u context.
func RequireAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		claims, err := ParseAccessToken(cfg.JWTSecret, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxOrgID, claims.OrgID)
		c.Set(ctxRole, claims.Role)
		c.Next()
	}
}

// RequireRole zahteva jednu od dozvoljenih rola.
func RequireRole(allowed ...string) gin.HandlerFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, _ := c.Get(ctxRole)
		roleStr, _ := role.(string)
		if _, ok := set[roleStr]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
			return
		}
		c.Next()
	}
}

// RequireOrgAccess proverava da org_id iz URL-a odgovara onom iz tokena.
// Koristi se na rutama tipa /v1/orgs/:org_id/...
func RequireOrgAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		urlOrg := c.Param("org_id")
		ctxOrg, _ := c.Get(ctxOrgID)
		if urlOrg == "" || urlOrg != ctxOrg {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "org mismatch"})
			return
		}
		c.Next()
	}
}

func UserID(c *gin.Context) string {
	v, _ := c.Get(ctxUserID)
	s, _ := v.(string)
	return s
}

func OrgID(c *gin.Context) string {
	v, _ := c.Get(ctxOrgID)
	s, _ := v.(string)
	return s
}

func Role(c *gin.Context) string {
	v, _ := c.Get(ctxRole)
	s, _ := v.(string)
	return s
}
