package members

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
	items, err := h.svc.ListMembers(c.Request.Context(), c.Param("org_id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type inviteReq struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *Handler) Invite(c *gin.Context) {
	var req inviteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	res, err := h.svc.Invite(c.Request.Context(), InviteInput{
		OrgID:   c.Param("org_id"),
		ActorID: auth.UserID(c),
		Email:   req.Email,
		Role:    req.Role,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

type updateRoleReq struct {
	Role string `json:"role"`
}

func (h *Handler) UpdateRole(c *gin.Context) {
	var req updateRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	if err := h.svc.UpdateRole(c.Request.Context(), UpdateRoleInput{
		OrgID:    c.Param("org_id"),
		ActorID:  auth.UserID(c),
		TargetID: c.Param("user_id"),
		NewRole:  req.Role,
	}); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Remove(c *gin.Context) {
	if err := h.svc.RemoveMember(c.Request.Context(), RemoveInput{
		OrgID:    c.Param("org_id"),
		ActorID:  auth.UserID(c),
		TargetID: c.Param("user_id"),
	}); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) ListInvitations(c *gin.Context) {
	items, err := h.svc.ListInvitations(c.Request.Context(), c.Param("org_id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) RevokeInvite(c *gin.Context) {
	if err := h.svc.RevokeInvite(c.Request.Context(), RevokeInviteInput{
		OrgID:    c.Param("org_id"),
		ActorID:  auth.UserID(c),
		InviteID: c.Param("invite_id"),
	}); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type acceptReq struct {
	Token string `json:"token"`
}

func (h *Handler) Accept(c *gin.Context) {
	var req acceptReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	res, err := h.svc.Accept(c.Request.Context(), AcceptInviteInput{
		Token:  req.Token,
		UserID: auth.UserID(c),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound),
		errors.Is(err, ErrInviteNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrLastOwner),
		errors.Is(err, ErrCannotModifySelf),
		errors.Is(err, ErrCannotRemoveSelf),
		errors.Is(err, ErrEmailMismatch),
		errors.Is(err, ErrInviteExpired),
		errors.Is(err, ErrInviteAlreadyUsed):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidRole),
		errors.Is(err, ErrInvalidEmail),
		errors.Is(err, ErrAlreadyMember):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
