package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"nhooyr.io/websocket"
)

// Message is the JSON envelope sent to WebSocket clients.
type Message struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// Client wraps a single WebSocket connection.
type Client struct {
	conn *websocket.Conn
	send chan []byte
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, 256),
	}
}

// Run starts the hub's event loop. It blocks and should be called in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- msg:
				default:
					// Client too slow; drop it.
					go func(c *Client) {
						h.unregister <- c
					}(client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a JSON message to all connected clients.
func (h *Hub) Broadcast(event string, data any) {
	msg := Message{Event: event, Data: data}
	b, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ws: marshal broadcast: %v", err)
		return
	}
	h.broadcast <- b
}

// HandleWebSocket upgrades an HTTP request to a WebSocket connection and
// registers the client with the hub.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow any origin for local use.
	})
	if err != nil {
		log.Printf("ws: accept: %v", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 64),
	}
	h.register <- client

	go h.writePump(client)
	h.readPump(client)
}

// readPump reads from the WebSocket (to detect close) and discards messages.
func (h *Hub) readPump(c *Client) {
	defer func() {
		h.unregister <- c
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		_, _, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}
	}
}

// writePump sends queued messages to the WebSocket connection.
func (h *Hub) writePump(c *Client) {
	defer c.conn.Close(websocket.StatusNormalClosure, "")

	for msg := range c.send {
		err := c.conn.Write(context.Background(), websocket.MessageText, msg)
		if err != nil {
			return
		}
	}
}
