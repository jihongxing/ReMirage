package mirage

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type WebSocket struct {
	url      string
	token    string
	conn     *websocket.Conn
	handlers map[string]func(map[string]interface{})
	mu       sync.RWMutex
	done     chan struct{}
}

func NewWebSocket(url string, opts ...Option) (*WebSocket, error) {
	cfg := &clientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return &WebSocket{
		url:      url,
		token:    cfg.token,
		handlers: make(map[string]func(map[string]interface{})),
		done:     make(chan struct{}),
	}, nil
}

func (ws *WebSocket) On(event string, handler func(map[string]interface{})) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.handlers[event] = handler
}

func (ws *WebSocket) Connect() error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+ws.token)

	conn, _, err := websocket.DefaultDialer.Dial(ws.url, header)
	if err != nil {
		return err
	}
	ws.conn = conn

	for {
		select {
		case <-ws.done:
			return nil
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				return err
			}
			ws.handleMessage(message)
		}
	}
}

func (ws *WebSocket) handleMessage(message []byte) {
	var msg struct {
		Event string                 `json:"event"`
		Data  map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	ws.mu.RLock()
	handler, ok := ws.handlers[msg.Event]
	ws.mu.RUnlock()

	if ok {
		handler(msg.Data)
	}
}

func (ws *WebSocket) Send(event string, data map[string]interface{}) error {
	msg := map[string]interface{}{"event": event, "data": data}
	return ws.conn.WriteJSON(msg)
}

func (ws *WebSocket) Close() error {
	close(ws.done)
	if ws.conn != nil {
		return ws.conn.Close()
	}
	return nil
}
