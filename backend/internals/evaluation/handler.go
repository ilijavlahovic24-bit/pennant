package evaluation

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"pennant/backend/internals/auth"
)

type Handler struct {
	engine *Engine
}

func NewHandler(engine *Engine) *Handler {
	return &Handler{engine: engine}
}

// Evaluate — GET /v1/evaluate/:flag_key?env=production
// Header X-User-Context: {"user_id":"u-42","plan":"enterprise","country":"RS"}
func (h *Handler) Evaluate(c *gin.Context) {
	orgID := auth.OrgID(c)
	flagKey := c.Param("flag_key")
	envSlug := c.DefaultQuery("env", "production")

	user, err := parseUserContext(c, auth.UserID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	res, err := h.engine.Evaluate(c.Request.Context(), orgID, envSlug, flagKey, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "evaluation failed"})
		return
	}
	c.JSON(http.StatusOK, res)
}

// Batch — GET /v1/evaluate/batch?flags=a,b,c&env=production
func (h *Handler) Batch(c *gin.Context) {
	orgID := auth.OrgID(c)
	envSlug := c.DefaultQuery("env", "production")
	flagsParam := c.Query("flags")
	if flagsParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "flags query param required"})
		return
	}
	keys := strings.Split(flagsParam, ",")
	for i := range keys {
		keys[i] = strings.TrimSpace(keys[i])
	}

	user, err := parseUserContext(c, auth.UserID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]*Result, 0, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		res, err := h.engine.Evaluate(c.Request.Context(), orgID, envSlug, k, user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "evaluation failed"})
			return
		}
		results = append(results, res)
	}

	c.JSON(http.StatusOK, gin.H{"items": results})
}

func parseUserContext(c *gin.Context, fallbackUserID string) (UserContext, error) {
	raw := c.GetHeader("X-User-Context")
	if raw == "" {
		return UserContext{UserID: fallbackUserID}, nil
	}
	var ctx UserContext
	if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
		return UserContext{}, errors.New("invalid X-User-Context header")
	}
	if ctx.UserID == "" {
		ctx.UserID = fallbackUserID
	}
	return ctx, nil
}
