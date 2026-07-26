package redeem

import (
	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

func getDB(c *gin.Context) *store.DB {
	dbVal, _ := c.Get("db")
	if dbVal == nil {
		return nil
	}
	return dbVal.(*store.DB)
}

// currentUser 从 context 取当前登录用户名
// JWT 仅含 username（登录账号），成员姓名（"爸爸"/"妈妈"）由前端传入，
// 后端信任已登录用户提交的 member_name（与原 localStorage 逻辑一致）
func currentUser(c *gin.Context) string {
	if u, ok := c.Get("username"); ok {
		if s, ok2 := u.(string); ok2 {
			return s
		}
	}
	return "未知"
}

// ListRedeemRecordsHandler 列出兑换记录（登录用户可见）
func ListRedeemRecordsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []interface{}{})
		return
	}
	records, err := db.ListRedeemRecords(c.Request.Context(), 200)
	if err != nil {
		response.Success(c, []interface{}{})
		return
	}
	if records == nil {
		records = []map[string]interface{}{}
	}
	response.Success(c, records)
}

// CreateRedeemRecordHandler 创建兑换记录（登录用户可提交）
func CreateRedeemRecordHandler(c *gin.Context) {
	var req struct {
		MemberName string `json:"member_name" binding:"required"`
		ItemName   string `json:"item_name" binding:"required"`
		ItemIcon   string `json:"item_icon"`
		Cost       int    `json:"cost"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	id, err := db.CreateRedeemRecord(c.Request.Context(), req.MemberName, req.ItemName, req.ItemIcon, req.Cost)
	if err != nil {
		response.InternalServerError(c, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id})
}

// UpdateRedeemRecordStatusHandler 确认/驳回兑换（admin）
func UpdateRedeemRecordStatusHandler(c *gin.Context) {
	var req struct {
		ID     int64  `json:"id" binding:"required"`
		Status string `json:"status" binding:"required"` // confirmed/rejected
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.Status != "confirmed" && req.Status != "rejected" {
		response.BadRequest(c, "status 仅支持 confirmed/rejected")
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	confirmedBy := currentUser(c)
	if err := db.UpdateRedeemRecordStatus(c.Request.Context(), req.ID, req.Status, confirmedBy); err != nil {
		response.InternalServerError(c, "更新失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": req.ID, "status": req.Status})
}

// ClearRedeemRecordsHandler 清空兑换记录（admin）
func ClearRedeemRecordsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	if err := db.DeleteAllRedeemRecords(c.Request.Context()); err != nil {
		response.InternalServerError(c, "清空失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"cleared": true})
}
