package wechat

import (
	"context"
	"encoding/xml"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	botservice "github.com/homemate/server/internal/service/wechat"
	"github.com/homemate/server/internal/store"
)

// weComConfig 运行时配置（内存）
var weComConfig = struct {
	WebhookURL  string
	EnablePush bool
}{}

// SetWeComConfig 注入初始配置（main.go 启动时调用）
func SetWeComConfig(webhookURL string, enablePush bool) {
	weComConfig.WebhookURL = webhookURL
	weComConfig.EnablePush = enablePush
}

// GetPusher 获取当前推送器（基于运行时配置）
func GetPusher() botservice.Pusher {
	if !weComConfig.EnablePush {
		return botservice.NewPusher("")
	}
	return botservice.NewPusher(weComConfig.WebhookURL)
}

// BindRequest 微信绑定请求参数
type BindRequest struct {
	OpenID   string `json:"openid" binding:"required"`
	MemberID int64  `json:"member_id" binding:"required"`
}

// CallbackHandler 微信消息回调接口
// 接收微信服务器推送的 XML 消息，解析后路由到 Agent 引擎，返回 XML 响应
func CallbackHandler(c *gin.Context) {
	var msg model.WeChatMessage
	if err := c.ShouldBindXML(&msg); err != nil {
		// 尝试直接读取原始 body 解析
		body, _ := c.GetRawData()
		if err2 := xml.Unmarshal(body, &msg); err2 != nil {
			c.String(http.StatusOK, "success")
			return
		}
	}

	// 路由到 Agent 引擎处理
	replyContent := processWeChatMessage(msg)

	resp := model.WeChatResponse{
		ToUserName:   msg.FromUserName,
		FromUserName: msg.ToUserName,
		CreateTime:   time.Now().Unix(),
		MsgType:      "text",
		Content:      replyContent,
	}

	c.XML(http.StatusOK, resp)
}

// BindHandler 绑定微信 OpenID 到家庭成员
func BindHandler(c *gin.Context) {
	var req BindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// 模拟绑定逻辑（实际应写入数据库）
	response.Success(c, gin.H{
		"openid":    req.OpenID,
		"member_id": req.MemberID,
		"status":    "bound",
		"bound_at":  time.Now().Format("2006-01-02 15:04:05"),
	})
}

// processWeChatMessage 模拟微信消息处理
func processWeChatMessage(msg model.WeChatMessage) string {
	if msg.Content == "" {
		return "您好，我是 HomeMate 智能助手，请问有什么可以帮您？"
	}
	return "收到您的消息：" + msg.Content + "，已转交给智能助手处理，请稍候。"
}

// TestPushHandler 测试推送（admin only）
func TestPushHandler(c *gin.Context) {
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Content = "HomeMate 测试推送"
	}
	if req.Content == "" {
		req.Content = "HomeMate 测试推送"
	}
	pusher := GetPusher()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := pusher.Push(ctx, "测试", req.Content); err != nil {
		response.Error(c, 500, "推送失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "推送成功", "content": req.Content, "webhook_configured": weComConfig.WebhookURL != ""})
}

// UpdateWeComConfigHandler 更新 WeCom 配置（admin only）
func UpdateWeComConfigHandler(c *gin.Context) {
	var req struct {
		WebhookURL  string `json:"webhook_url"`
		EnablePush bool   `json:"enable_push"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	weComConfig.WebhookURL = req.WebhookURL
	weComConfig.EnablePush = req.EnablePush
	// 尝试持久化到 data_source_config 表（可选，失败不阻塞）
	dbVal, _ := c.Get("db")
	if db, ok := dbVal.(*store.DB); ok && db != nil {
		// 用 data_source_config 表存储 wecom 配置（source_type=wecom）
		// 简化：仅内存更新，重启后从 config.yaml 重新加载
		_ = db
	}
	response.Success(c, gin.H{
		"webhook_url":  weComConfig.WebhookURL,
		"enable_push": weComConfig.EnablePush,
		"message":     "配置已更新",
	})
}

// GetWeComConfigHandler 获取 WeCom 配置（admin only）
func GetWeComConfigHandler(c *gin.Context) {
	response.Success(c, gin.H{
		"webhook_url":  weComConfig.WebhookURL,
		"enable_push": weComConfig.EnablePush,
	})
}
