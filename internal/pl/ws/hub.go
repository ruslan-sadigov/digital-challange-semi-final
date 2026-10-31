// Package ws is the real-time WebSocket hub. It implements bl.Publisher and bl.Notifier.
package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"smartcity/internal/bl"
	"smartcity/internal/domain"
)

// Message is the envelope sent to clients.
type Message struct {
	Channel string `json:"channel"`
	Type    string `json:"type"`
	Data    any    `json:"data"`
}

type client struct {
	conn *websocket.Conn
	send chan []byte
	role domain.Role
	mu   sync.RWMutex
	subs map[string]bool
}

func (c *client) subscribed(ch string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.subs[ch]
}

type Hub struct {
	mu       sync.RWMutex
	clients  map[*client]struct{}
	upgrader websocket.Upgrader
}

func NewHub() *Hub {
	return &Hub{clients: map[*client]struct{}{}, upgrader: websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 4096}}
}

// CanSubscribe enforces channel permissions: operator:* channels are operator-only.
func CanSubscribe(role domain.Role, channel string) bool {
	if strings.HasPrefix(channel, "operator:") {
		return role == domain.RoleOperator
	}
	return channel == bl.ChannelBuses
}

// Publish implements bl.Publisher. Slow clients drop messages instead of blocking the server.
func (h *Hub) Publish(channel, msgType string, data any) {
	b, err := json.Marshal(Message{Channel: channel, Type: msgType, Data: data})
	if err != nil {
		log.Printf("ws: marshal %s/%s: %v", channel, msgType, err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.subscribed(channel) {
			select {
			case c.send <- b:
			default:
			}
		}
	}
}

// Notify implements bl.Notifier: an in-app alert on the operator alerts channel.
// The operator page turns it into a browser notification.
func (h *Hub) Notify(_ context.Context, a bl.Alert) error {
	h.Publish(bl.ChannelOperatorAlerts, "alert", a)
	return nil
}

// ClientCount is used by tests and health checks.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

type inbound struct {
	Action  string `json:"action"` // subscribe | unsubscribe
	Channel string `json:"channel"`
}

// Serve upgrades the request and runs the client until it disconnects.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, role domain.Role) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn, send: make(chan []byte, 64), role: role, subs: map[string]bool{}}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	go c.writeLoop()
	c.readLoop()

	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	close(c.send)
}

func (c *client) readLoop() {
	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	for {
		var m inbound
		if err := c.conn.ReadJSON(&m); err != nil {
			return
		}
		switch m.Action {
		case "subscribe":
			ok := CanSubscribe(c.role, m.Channel)
			if ok {
				c.mu.Lock()
				c.subs[m.Channel] = true
				c.mu.Unlock()
			}
			b, _ := json.Marshal(Message{Channel: m.Channel, Type: "subscribed", Data: map[string]bool{"ok": ok}})
			select {
			case c.send <- b:
			default:
			}
		case "unsubscribe":
			c.mu.Lock()
			delete(c.subs, m.Channel)
			c.mu.Unlock()
		}
	}
}

func (c *client) writeLoop() {
	ping := time.NewTicker(25 * time.Second)
	defer func() {
		ping.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case b, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		case <-ping.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
