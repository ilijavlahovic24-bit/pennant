package ws

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"pennant/backend/internals/auth"
	"pennant/backend/internals/config"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In dev, allow all; in production, check Origin header.
		return true
	},
}

type Handler struct {
	hub *Hub
	cfg *config.Config
}

func NewHandler(hub *Hub, cfg *config.Config) *Handler {
	return &Handler{hub: hub, cfg: cfg}
}

// Serve — GET /v1/ws/flags?token=<jwt>
// Token goes in query param because WebSocket API in browser does not support custom headers when establishing connection.
// custom header during connection.
func (h *Handler) Serve(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		return
	}

	claims, err := auth.ParseAccessToken(h.cfg.JWTSecret, token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	if claims.OrgID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no org in token"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "err", err)
		return
	}

	client := NewClient(h.hub, conn, claims.OrgID)
	h.hub.Register(claims.OrgID, client)
	//Send welcome event to client to confirm connection.
	client.send <- Event{
		Type:    "connected",
		Payload: map[string]any{"org_id": claims.OrgID, "user_id": claims.UserID},
	}

	client.Run()
}

//ExtractBearer is helper if we would use header-based auth (example tests).

func ExtractBearer(h string) string {
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}
