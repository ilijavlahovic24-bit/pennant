package flags

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"pennant/backend/internals/auth"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type createReq struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

func (h *Handler) Create(c *gin.Context) {
	orgID := c.Param("org_id")
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	res, err := h.svc.Create(c.Request.Context(), CreateInput{
		OrgID:       orgID,
		ActorID:     auth.UserID(c),
		Key:         req.Key,
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

func (h *Handler) List(c *gin.Context) {
	orgID := c.Param("org_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	includeArchived := c.Query("include_archived") == "true"
	search := c.Query("search")

	res, err := h.svc.List(c.Request.Context(), ListParams{
		OrgID:           orgID,
		IncludeArchived: includeArchived,
		Search:          search,
		Page:            page,
		PageSize:        pageSize,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) Get(c *gin.Context) {
	orgID := c.Param("org_id")
	flagID := c.Param("flag_id")

	res, err := h.svc.Get(c.Request.Context(), orgID, flagID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type updateReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) Update(c *gin.Context) {
	orgID := c.Param("org_id")
	flagID := c.Param("flag_id")

	var req updateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	res, err := h.svc.Update(c.Request.Context(), UpdateInput{
		OrgID:       orgID,
		ActorID:     auth.UserID(c),
		FlagID:      flagID,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) Archive(c *gin.Context) {
	orgID := c.Param("org_id")
	flagID := c.Param("flag_id")

	if err := h.svc.Archive(c.Request.Context(), ArchiveInput{
		OrgID:   orgID,
		ActorID: auth.UserID(c),
		FlagID:  flagID,
	}); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type updateEnvReq struct {
	Enabled        *bool           `json:"enabled"`
	RolloutPercent *int            `json:"rollout_percent"`
	Value          json.RawMessage `json:"value"`
	ExpiresAt      *string         `json:"expires_at"`
	ClearExpiresAt bool            `json:"clear_expires_at"`
}

func (h *Handler) UpdateEnvState(c *gin.Context) {
	orgID := c.Param("org_id")
	flagID := c.Param("flag_id")
	envID := c.Param("env_id")

	body, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read body"})
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	var req updateEnvReq
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	_, valueSet := raw["value"]

	res, err := h.svc.UpdateEnvState(c.Request.Context(), UpdateEnvInput{
		OrgID:          orgID,
		ActorID:        auth.UserID(c),
		FlagID:         flagID,
		EnvID:          envID,
		Enabled:        req.Enabled,
		RolloutPercent: req.RolloutPercent,
		Value:          req.Value,
		ValueSet:       valueSet,
		ExpiresAt:      req.ExpiresAt,
		ClearExpiresAt: req.ClearExpiresAt,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrEnvNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrKeyTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidKey),
		errors.Is(err, ErrInvalidType),
		errors.Is(err, ErrInvalidValue),
		errors.Is(err, ErrInvalidRollout):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrArchivedNoUpdate):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
