package targeting

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) List(c *gin.Context) {
	orgID := c.Param("org_id")
	flagID := c.Param("flag_id")
	envID := c.Param("env_id")

	rules, err := h.svc.List(c.Request.Context(), orgID, flagID, envID)
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
	orgID := c.Param("org_id")
	flagID := c.Param("flag_id")
	envID := c.Param("env_id")

	var req replaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	rules, err := h.svc.ReplaceAll(c.Request.Context(), orgID, flagID, envID, req.Rules)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rules})
}

func (h *Handler) Add(c *gin.Context) {
	orgID := c.Param("org_id")
	flagID := c.Param("flag_id")
	envID := c.Param("env_id")

	var in RuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	rule, err := h.svc.Add(c.Request.Context(), orgID, flagID, envID, in)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (h *Handler) Delete(c *gin.Context) {
	orgID := c.Param("org_id")
	flagID := c.Param("flag_id")
	envID := c.Param("env_id")
	ruleID := c.Param("rule_id")

	if err := h.svc.Delete(c.Request.Context(), orgID, flagID, envID, ruleID); err != nil {
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
		errors.Is(err, ErrTooManyRules),
		errors.Is(err, ErrDuplicatePriority):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		// If the error is wrapped with fmt.Errorf("%w") in rule validation,
		// errors.Is will still work because we use sentinel errors.

		var ute interface{ Unwrap() error }
		if errors.As(err, &ute) {
			h.writeError(c, ute.Unwrap())
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
