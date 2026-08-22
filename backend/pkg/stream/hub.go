package stream

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"nexus-feed/backend/pkg/types"
)

const (
	writeWait            = 10 * time.Second
	pongWait             = 60 * time.Second
	pingPeriod           = (pongWait * 9) / 10
	maxMessageSize       = 512
	clientSendBufferSize = 256
)

// WSMessage is the standardized JSON envelope transmitted over the WebSocket.
type WSMessage struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

// ClientRequest represents incoming control commands sent from the client.
type ClientRequest struct {
	Action string `json:"action"`
	Symbol string `json:"symbol"`
}

// Client represents a single active WebSocket connection and its outbound message buffer.
type Client struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan []byte
	subscriptions map[string]bool
	mu            sync.RWMutex
}

// Hub maintains the set of active clients and broadcasts messages to clients.
type Hub struct {
	clients       map[*Client]bool
	register      chan *Client
	unregister    chan *Client
	mu            sync.RWMutex
	activeClients int64
	upgrader      websocket.Upgrader
}

// isSubscribed checks if the client is subscribed to a specific symbol or subscribed to ALL.
func (c *Client) isSubscribed(symbol string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.subscriptions["ALL"] {
		return true
	}
	return c.subscriptions[symbol]
}

// subscribe adds a symbol to the client's active subscriptions list.
func (c *Client) subscribe(symbol string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subscriptions[symbol] = true
}

// unsubscribe removes a symbol from the client's active subscriptions list.
func (c *Client) unsubscribe(symbol string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subscriptions, symbol)
}

// readPump reads incoming commands from the WebSocket connection into the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[Stream] WebSocket read error: %v", err)
			}
			break
		}

		var req ClientRequest
		if err := json.Unmarshal(message, &req); err == nil {
			switch req.Action {
			case "subscribe":
				if req.Symbol == "" {
					req.Symbol = "ALL"
				}
				c.subscribe(req.Symbol)

			case "unsubscribe":
				c.unsubscribe(req.Symbol)
			}
		}
	}
}

// writePump pumps messages from the send channel to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Coalesce queued messages into current frame to minimize syscalls
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// NewHub initializes and returns a new Hub pointer.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client, 32),
		unregister: make(chan *Client, 32),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			atomic.AddInt64(&h.activeClients, 1)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				atomic.AddInt64(&h.activeClients, -1)
			}
			h.mu.Unlock()
		}
	}
}

// BroadcastBooks sends a batch of consolidated order books to all subscribed clients.
func (h *Hub) BroadcastBooks(snapshots []types.ConsolidatedBook) {
	if len(snapshots) == 0 {
		return
	}

	nowMicros := time.Now().UnixNano() / 1000

	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.clients) == 0 {
		return
	}

	for _, book := range snapshots {
		msg := WSMessage{
			Type:      "book_update",
			Data:      book,
			Timestamp: nowMicros,
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			continue
		}

		for client := range h.clients {
			if client.isSubscribed(book.Symbol) {
				select {
				case client.send <- payload:
				default:
				}
			}
		}
	}
}

// BroadcastArbitrage sends detected arbitrage opportunities to all connected clients.
func (h *Hub) BroadcastArbitrage(arbs []*types.ArbitrageOpportunity) {
	if len(arbs) == 0 {
		return
	}

	nowMicros := time.Now().UnixNano() / 1000

	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.clients) == 0 {
		return
	}

	for _, arb := range arbs {
		msg := WSMessage{
			Type:      "arbitrage",
			Data:      arb,
			Timestamp: nowMicros,
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			continue
		}

		for client := range h.clients {
			select {
			case client.send <- payload:
			default:
			}
		}
	}
}

// HandleConflatedBatch connects the Conflator directly to the Hub's broadcast methods.
func (h *Hub) HandleConflatedBatch(snapshots []types.ConsolidatedBook, arbs []*types.ArbitrageOpportunity) {
	h.BroadcastBooks(snapshots)
	h.BroadcastArbitrage(arbs)
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Stream] Failed to upgrade WebSocket connection: %v", err)
		return
	}

	client := &Client{
		hub:           h,
		conn:          conn,
		send:          make(chan []byte, clientSendBufferSize),
		subscriptions: map[string]bool{"ALL": true},
	}

	h.register <- client
	go client.writePump()
	go client.readPump()
}

func (h *Hub) GetActiveClients() int {
	return int(atomic.LoadInt64(&h.activeClients))
}

