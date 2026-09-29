package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"pennant/backend/internals/config"
)

const refreshCookieName = "refresh_token"

type Handler struct {
	cfg *config.Config
	svc *Service
}

func NewHandler(cfg *config.Config, svc *Service) *Handler {
	return &Handler{cfg: cfg, svc: svc}
}

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgName  string `json:"org_name"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	res, err := h.svc.Register(c.Request.Context(), RegisterInput{
		Email:    req.Email,
		Password: req.Password,
		OrgName:  req.OrgName,
	})
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	h.setRefreshCookie(c, res.RefreshToken)
	c.JSON(http.StatusCreated, gin.H{
		"access_token": res.AccessToken,
		"user_id":      res.UserID,
		"org_id":       res.OrgID,
		"role":         res.Role,
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	res, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	h.setRefreshCookie(c, res.RefreshToken)
	c.JSON(http.StatusOK, gin.H{
		"access_token": res.AccessToken,
		"user_id":      res.UserID,
		"org_id":       res.OrgID,
		"role":         res.Role,
	})
}

func (h *Handler) Refresh(c *gin.Context) {
	token, err := c.Cookie(refreshCookieName)
	if err != nil || token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no refresh token"})
		return
	}

	res, err := h.svc.Refresh(c.Request.Context(), token)
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	h.setRefreshCookie(c, res.RefreshToken)
	c.JSON(http.StatusOK, gin.H{
		"access_token": res.AccessToken,
		"user_id":      res.UserID,
		"org_id":       res.OrgID,
		"role":         res.Role,
	})
}

func (h *Handler) Logout(c *gin.Context) {
	token, _ := c.Cookie(refreshCookieName)
	if token != "" {
		_ = h.svc.Logout(c.Request.Context(), token)
	}
	h.clearRefreshCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *Handler) setRefreshCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		refreshCookieName,
		token,
		int((7 * 24 * time.Hour).Seconds()),
		"/",
		h.cfg.CookieDomain,
		h.cfg.CookieSecure,
		true, // httpOnly
	)
}

func (h *Handler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, "", -1, "/", h.cfg.CookieDomain, h.cfg.CookieSecure, true)
}

func (h *Handler) writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, ErrEmailTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidEmail),
		errors.Is(err, ErrWeakPassword):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidToken),
		errors.Is(err, ErrTokenExpired),
		errors.Is(err, ErrTokenRevoked):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNoMembership):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

// SwitchOrg issues a new access token for the given organization.
// Refresh token remains the same — binds to user, not org.
func (s *Service) SwitchOrg(ctx context.Context, userID, orgID string) (string, string, error) {
	m, err := s.members.Find(ctx, s.pool, userID, orgID)
	if err != nil {
		return "", "", fmt.Errorf("find membership: %w", err)
	}
	if m == nil {
		return "", "", ErrNotMember
	}

	access, err := GenerateAccessToken(s.cfg.JWTSecret, userID, orgID, m.Role, s.cfg.AccessTTL)
	if err != nil {
		return "", "", err
	}
	return access, m.Role, nil
}

type switchOrgReq struct {
	OrgID string `json:"org_id"`
}

func (h *Handler) SwitchOrg(c *gin.Context) {
	userID := c.GetString("auth.user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req switchOrgReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	access, role, err := h.svc.SwitchOrg(c.Request.Context(), userID, req.OrgID)
	if err != nil {
		if errors.Is(err, ErrNotMember) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": access,
		"org_id":       req.OrgID,
		"role":         role,
	})
}
