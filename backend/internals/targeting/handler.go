package targeting

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"pennant/backend/internals/auth"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) List(c *gin.Context) {
	rules, err := h.svc.List(c.Request.Context(),
		c.Param("org_id"), c.Param("flag_id"), c.Param("env_id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rules})
}

type replaceReq struct {
	Rules []RuleInput `json:"rules"`
}

func (h *Handler) ReplaceAll(c *gin.Context) {
	var req replaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	rules, err := h.svc.ReplaceAll(c.Request.Context(), ReplaceInput{
		OrgID:   c.Param("org_id"),
		ActorID: auth.UserID(c),
		FlagID:  c.Param("flag_id"),
		EnvID:   c.Param("env_id"),
		Rules:   req.Rules,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rules})
}

func (h *Handler) Add(c *gin.Context) {
	var in RuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	rule, err := h.svc.Add(c.Request.Context(), AddInput{
		OrgID:   c.Param("org_id"),
		ActorID: auth.UserID(c),
		FlagID:  c.Param("flag_id"),
		EnvID:   c.Param("env_id"),
		Rule:    in,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), DeleteInput{
		OrgID:   c.Param("org_id"),
		ActorID: auth.UserID(c),
		FlagID:  c.Param("flag_id"),
		EnvID:   c.Param("env_id"),
		RuleID:  c.Param("rule_id"),
	}); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidAttribute),
		errors.Is(err, ErrInvalidOperator),
		errors.Is(err, ErrInvalidAction),
		errors.Is(err, ErrInvalidValue),
		errors.Is(err, ErrInvalidActionValue),
		errors.Is(err, ErrTooManyRules):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
