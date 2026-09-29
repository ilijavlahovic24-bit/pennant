package ws

import (
	"log/slog"
	"sync"
	"time"
)

type Event struct {
	Type      string `json:"type"`
	Payload   any    `json:"payload"`
	Timestamp string `json:"timestamp"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]struct{} // org_id → set of clients
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string]map[*Client]struct{})}
}

func (h *Hub) Register(orgID string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[orgID] == nil {
		h.clients[orgID] = make(map[*Client]struct{})
	}
	h.clients[orgID][c] = struct{}{}
	slog.Info("ws client registered", "org_id", orgID, "total_in_org", len(h.clients[orgID]))
}

func (h *Hub) Unregister(orgID string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.clients[orgID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, orgID)
		}
	}
	slog.Info("ws client unregistered", "org_id", orgID)
}

// BroadcastToOrg šalje event svim klijentima date organizacije.
func (h *Hub) BroadcastToOrg(orgID string, event Event) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	h.mu.RLock()
	set := h.clients[orgID]
	// Napravi snapshot da ne držimo lock tokom slanja.
	targets := make([]*Client, 0, len(set))
	for c := range set {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	if len(targets) == 0 {
		return
	}

	slog.Debug("ws broadcast", "org_id", orgID, "clients", len(targets), "type", event.Type)

	for _, c := range targets {
		select {
		case c.send <- event:
		default:
			// Client is slow — skip it, writePump will kill it when
			// the send buffer fills up beyond capacity.
			slog.Warn("ws client slow, dropping event", "org_id", orgID)
		}
	}
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, set := range h.clients {
		n += len(set)
	}
	return n
}
