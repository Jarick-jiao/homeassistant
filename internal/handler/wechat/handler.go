package wechat

import (
	"encoding/xml"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
)

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
