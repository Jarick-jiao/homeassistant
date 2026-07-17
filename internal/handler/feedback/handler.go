package feedback

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
)

// ListFeedbackHandler 列出反馈摘要
func ListFeedbackHandler(c *gin.Context) {
	feedbacks := []model.Feedback{
		{ID: 1, MemberID: 1, Type: "suggestion", Content: "希望增加语音控制功能", Rating: 5, CreatedAt: "2024-01-10 10:00"},
		{ID: 2, MemberID: 4, Type: "issue", Content: "字体还是有点小，希望能更大一些", Rating: 3, CreatedAt: "2024-01-12 15:30"},
		{ID: 3, MemberID: 3, Type: "praise", Content: "界面很好看，我喜欢！", Rating: 5, CreatedAt: "2024-01-13 09:00"},
	}
	response.Success(c, feedbacks)
}

// SubmitFeedbackHandler 提交反馈
func SubmitFeedbackHandler(c *gin.Context) {
	var req model.Feedback
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	req.ID = 4
	req.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	response.Success(c, req)
}
