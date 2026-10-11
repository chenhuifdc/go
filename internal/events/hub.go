package events

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var (
	wsUpgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	wsHub = newWSHub()
)

// WSHub WebSocket 消息广播中心
type WSHub struct {
	mu      sync.RWMutex
	clients map[*wsClient]bool
}

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

func newWSHub() *WSHub {
	h := &WSHub{
		clients: make(map[*wsClient]bool),
	}
	go h.run()
	return h
}

func (h *WSHub) run() {
	for {
		time.Sleep(time.Hour)
	}
}

// Broadcast 向所有连接的客户端广播消息
func (h *WSHub) Broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		select {
		case client.send <- msg:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
}

// Register 注册客户端
func (h *WSHub) Register(client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = true
}

// Unregister 注销客户端
func (h *WSHub) Unregister(client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
}

// HandleWebSocket 处理 WebSocket 连接
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		zap.L().Error("WebSocket 升级失败", zap.Error(err))
		return
	}

	client := &wsClient{
		conn: conn,
		send: make(chan []byte, 256),
	}
	wsHub.Register(client)

	go func() {
		for msg := range client.send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				zap.L().Debug("WebSocket 写入失败", zap.Error(err))
				break
			}
		}
		conn.Close()
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}

	wsHub.Unregister(client)
}

// BroadcastEvent 广播事件到所有 WebSocket 客户端
func BroadcastEvent(eventType string, data interface{}) {
	msg, err := json.Marshal(map[string]interface{}{
		"type": eventType,
		"data": data,
		"time": time.Now().UnixMilli(),
	})
	if err != nil {
		zap.L().Error("序列化事件失败", zap.Error(err))
		return
	}
	wsHub.Broadcast(msg)
}