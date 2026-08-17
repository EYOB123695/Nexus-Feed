package stream

import (
	// context manages cancellation and graceful shutdown of the Hub
	"context"
	// json provides encoding and decoding of WebSocket payloads
	"encoding/json"
	// log handles server error logging for disconnected sockets
	"log"

	// http provides HTTP request handling and response writing for the WebSocket endpoint
	"net/http"
	// sync provides mutexes for thread-safe client subscription management
	"sync"
	// atomic provides lock-free tracking of connected client counts
	"sync/atomic"
	// time manages write timeouts, ping intervals, and pong deadlines
	"time"
	// websocket imports the Gorilla WebSocket library for real-time streaming
	"github.com/gorilla/websocket"

	// types imports shared engine structs (ConsolidatedBook, ArbitrageOpportunity)
	"nexus-feed/backend/pkg/types"
)

const (
	// writeWait is the maximum duration allowed to write a message to the peer
	writeWait = 10 * time.Second
	// pongWait is the maximum duration allowed to read the next pong message from the peer
	pongWait = 60 * time.Second
	// pingPeriod is the interval to send pings to peer (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// maxMessageSize is the maximum permitted message size from the client in bytes
	maxMessageSize = 512
	// clientSendBufferSize is the buffer capacity of a single client's outbound channel
	clientSendBufferSize = 256
)

// WSMessage is the standardized JSON envelope transmitted over the WebSocket.
type WSMessage struct {
	// Type specifies the message category (e.g. "book_update", "arbitrage", "subscribed")
	Type string `json:"type"`
	// Data holds the dynamic JSON payload (e.g. ConsolidatedBook or ArbitrageOpportunity)
	Data interface{} `json:"data"`
	// Timestamp is the Unix microsecond timestamp when the message was packaged
	Timestamp int64 `json:"timestamp"`
}

// ClientRequest represents incoming control commands sent from the client.
type ClientRequest struct {
	// Action defines the operation (e.g. "subscribe" or "unsubscribe")
	Action string `json:"action"`
	// Symbol specifies the market pair (e.g. "BTC-USDT" or "ALL")
	Symbol string `json:"symbol"`
}

// Client represents a single active WebSocket connection and its outbound message buffer.
type Client struct {
	// hub is a pointer back to the central Stream Hub
	hub *Hub
	// conn is the underlying Gorilla WebSocket connection pointer
	conn *websocket.Conn
	// send is a buffered channel of pre-serialized JSON byte slices queued for writing
	send chan []byte
	// subscriptions tracks the symbols this client is actively subscribed to
	subscriptions map[string]bool
	// mu is a read-write mutex protecting concurrent access to the subscriptions map
	mu sync.RWMutex
}

// Hub maintains the set of active clients and broadcasts messages to clients.
type Hub struct {
	// clients maps active client pointers to presence booleans
	clients map[*Client]bool
	// register channel receives requests to register new clients
	register chan *Client
	// unregister channel receives requests to unregister and remove clients
	unregister chan *Client
	// mu protects concurrent access to the clients map
	mu sync.RWMutex
	// activeClients is an atomic counter of currently connected WebSocket clients
	activeClients int64
	// upgrader is configured to upgrade incoming HTTP requests to WebSocket connections
	upgrader websocket.Upgrader
}

// isSubscribed checks if the client is subscribed to a specific symbol or subscribed to ALL.
func (c *Client) isSubscribed(symbol string) bool {
	// Acquire read lock for safe map access without blocking other readers
	c.mu.RLock()
	// Defer releasing the read lock when the function returns
	defer c.mu.RUnlock()
	// If the client subscribed to "ALL", return true unconditionally
	if c.subscriptions["ALL"] {
		return true
	}
	// Check if the exact symbol key exists and is set to true
	return c.subscriptions[symbol]
}

// subscribe adds a symbol to the client's active subscriptions list.
func (c *Client) subscribe(symbol string) {
	// Acquire exclusive write lock to prevent race conditions
	c.mu.Lock()
	// Defer releasing the write lock
	defer c.mu.Unlock()
	// Set symbol subscription flag to true in the map
	c.subscriptions[symbol] = true
}

// unsubscribe removes a symbol from the client's active subscriptions list.
func (c *Client) unsubscribe(symbol string) {
	// Acquire exclusive write lock
	c.mu.Lock()
	// Defer releasing the write lock
	defer c.mu.Unlock()
	// Delete the symbol key from the map
	delete(c.subscriptions, symbol)
}

// readPump reads incoming commands from the WebSocket connection into the hub.
func (c *Client) readPump() {
	// Ensure the client is unregistered and the connection closed when reading terminates
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	// Configure the maximum incoming message size (512 bytes)
	c.conn.SetReadLimit(maxMessageSize)

	// Set initial read deadline for the first pong response
	c.conn.SetReadDeadline(time.Now().Add(pongWait))

	// Set pong handler to renew read deadline every time a pong is received from the client
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Continuously loop reading messages from the socket
	for {
		// Read the next raw message from the WebSocket connection
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// If error is an unexpected close error, log it
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[Stream] WebSocket read error: %v", err)
			}
			// Exit loop to trigger defer cleanup
			break
		}

		// Parse the client's request JSON
		var req ClientRequest
		if err := json.Unmarshal(message, &req); err == nil {
			// Handle the requested action
			switch req.Action {
			case "subscribe":
				// If symbol was empty, default to "ALL"
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
	// Initialize a periodic ticker for sending ping heartbeats
	ticker := time.NewTicker(pingPeriod)
	// Ensure ticker and connection are closed upon exit
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	// Continuously loop multiplexing between outbound data and ping pulses
	for {
		select {
		// Case 1: Outbound message ready in the client's send buffer
		case message, ok := <-c.send:
			// Set write deadline for the upcoming write operation (10s)
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel, send close message to peer
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Obtain the NextWriter for sending a TextMessage frame
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			// Write the primary JSON message
			w.Write(message)

			// Coalesce queued messages in channel into the current WebSocket frame to save network roundtrips
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			// Flush and close the frame writer
			if err := w.Close(); err != nil {
				return
			}

		// Case 2: Ping interval elapsed; send heartbeat ping
		case <-ticker.C:
			// Set write deadline for the ping packet
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			// Write ping control message down the socket
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// NewHub initializes and returns a new Hub pointer.
func NewHub() *Hub {
	return &Hub{
		// Initialize the clients map
		clients: make(map[*Client]bool),

		// Initialize the register channel with capacity for burst connections
		register: make(chan *Client, 32),
		// Initialize the unregister channel
		unregister: make(chan *Client, 32),

		// Configure the WebSocket Upgrader with open CORS policy for dev/production flexibility
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins (CORS)
				return true
			},
		}}
}

func (h *Hub) Run(ctx context.Context) {

	for {
		select {
		// Case 1: Context cancellation or server shutdown signal
		case <-ctx.Done():
			h.mu.Lock()
			// Close all active client send channels
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return
			// Case 2: A new client connects and requests registration
		case client := <-h.register:
			h.mu.Lock()
			// Register client in the map
			h.clients[client] = true
			h.mu.Unlock()
			// Atomically increment active client counter
			atomic.AddInt64(&h.activeClients, 1)

			// Case 3: A client disconnects or encounters an error
		case client := <-h.unregister:
			h.mu.Lock()
			// Check if client is present in map
			if _, ok := h.clients[client]; ok {
				// Remove client from map
				delete(h.clients, client)
				// Close client send channel to shut down its writePump
				close(client.send)
				// Atomically decrement active client counter
				atomic.AddInt64(&h.activeClients, -1)
			}
			h.mu.Unlock()
		}
	}
}

// BroadcastBooks sends a batch of consolidated order books to all subscribed clients.

func (h *Hub) BroadcastBooks(snapshots []types.ConsolidatedBook) {

	// If no snapshots are in the batch, return immediately
	if len(snapshots) == 0 {
		return
	}

	// Capture current timestamp in microseconds
	nowMicros := time.Now().UnixNano() / 1000

	// Acquire read lock to iterate over active clients safely
	h.mu.RLock()
	// Defer releasing the read lock
	defer h.mu.RUnlock()

	if len(h.clients) == 0 {
		return
	}
	// Iterate over each consolidated book snapshot in the batch
	for _, book := range snapshots {
		// Package the snapshot into the standardized envelope struct
		msg := WSMessage{
			Type:      "book_update",
			Data:      book,
			Timestamp: nowMicros,
		}
		// Serialize the envelope into JSON bytes
		payload, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		// Broadcast the payload to every client subscribed to this symbol

		for client := range h.clients {
			if client.isSubscribed(book.Symbol) {
				select {
				// Enqueue payload into client's outbound buffer
				case client.send <- payload:
				// If buffer is full, drop the stale frame to prevent blocking other clients
				default:
				}
			}
		}
	}
}

// BroadcastArbitrage sends detected arbitrage opportunities to all connected clients.
func (h *Hub) BroadcastArbitrage(arbs []*types.ArbitrageOpportunity) {
	// Return early if no arbitrage events exist
	if len(arbs) == 0 {
		return
	}
	// Capture timestamp in microseconds
	nowMicros := time.Now().UnixNano() / 1000
	// Acquire read lock to safely inspect clients
	h.mu.RLock()
	defer h.mu.RUnlock()
	// Skip if no clients are listening
	if len(h.clients) == 0 {
		return
	}
	// Iterate through each arbitrage opportunity in the batch
	for _, arb := range arbs {
		// Wrap in the WSMessage envelope
		msg := WSMessage{
			Type:      "arbitrage",
			Data:      arb,
			Timestamp: nowMicros,
		}
		// Marshal envelope to JSON
		payload, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		// Broadcast to all clients
		for client := range h.clients {
			select {
			case client.send <- payload:
			default:
			}
		}
	}
}

// HandleConflatedBatch connects the Conflator directly to the Hub's broadcast methods.
// This function matches the conflation.ConflationHandler signature perfectly

func (h *Hub) HandleConflatedBatch(snapshots []types.ConsolidatedBook, arbs []*types.ArbitrageOpportunity) {

	// Broadcast the consolidated book updates
	h.BroadcastBooks(snapshots)
	// Broadcast the arbitrage opportunities
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

