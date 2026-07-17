package chat

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// sessions 存储 WebSocket 连接会话
var sessions = sync.Map{}

// ChatRequest REST 聊天请求参数
type ChatRequest struct {
	Message string `json:"message" binding:"required"`
}

// ChatHandler REST 聊天接口
// 接收用户消息，路由到 Agent 引擎处理，返回响应
func ChatHandler(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	reply := processMessage(req.Message)
	response.Success(c, gin.H{
		"reply":     reply,
		"timestamp": time.Now().Unix(),
	})
}

// WebSocketHandler WebSocket 聊天接口
// 升级 HTTP 连接为 WebSocket，维护会话，实现实时收发消息
func WebSocketHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		response.BadRequest(c, "WebSocket 升级失败")
		return
	}
	defer conn.Close()

	sessionID := c.Query("session_id")
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	sessions.Store(sessionID, conn)
	defer sessions.Delete(sessionID)

	// 发送欢迎消息
	welcome := model.ChatMessage{
		SessionID: sessionID,
		Role:      "assistant",
		Content:   "欢迎使用 HomeMate 智能助手！有什么可以帮您的吗？",
		Timestamp: time.Now().Unix(),
	}
	if err := conn.WriteJSON(welcome); err != nil {
		return
	}

	for {
		var msg model.ChatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		msg.SessionID = sessionID
		msg.Timestamp = time.Now().Unix()

		reply := model.ChatMessage{
			SessionID: sessionID,
			Role:      "assistant",
			Content:   processMessage(msg.Content),
			Timestamp: time.Now().Unix(),
		}

		if err := conn.WriteJSON(reply); err != nil {
			break
		}
	}
}

// processMessage 模拟 Agent 引擎处理消息
func processMessage(msg string) string {
	return fmt.Sprintf("【智能助手回复】收到您的消息：%s", msg)
}

// generateSessionID 生成会话 ID
func generateSessionID() string {
	return fmt.Sprintf("%d-%d", time.Now().Unix(), time.Now().UnixNano()%1000000)
}
