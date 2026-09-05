package redeem

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/pkg/memberctx"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

func getDB(c *gin.Context) *store.DB {
	return memberctx.GetDB(c)
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

// CreateRedeemRecordHandler 创建兑换申请
// v4.0（范式 §2.2）：兑换人身份取自 JWT → family_members，请求体只传物品信息；
// 兑换不立即扣分，管理员确认时在事务内校验余额并写负向积分流水。
func CreateRedeemRecordHandler(c *gin.Context) {
	var req struct {
		ItemName string `json:"item_name" binding:"required"`
		ItemIcon string `json:"item_icon"`
		Cost     int    `json:"cost"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.Cost <= 0 {
		response.BadRequest(c, "兑换积分必须大于 0")
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	member, err := memberctx.CurrentMember(c)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	id, err := db.CreateRedeemRecord(c.Request.Context(), member.ID, member.Name, req.ItemName, req.ItemIcon, req.Cost)
	if err != nil {
		response.InternalServerError(c, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id})
}

// UpdateRedeemRecordStatusHandler 确认/驳回兑换（管理员）
// 状态机（范式 §1.3）：仅 pending → confirmed（扣分）/rejected（不扣分）
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
	operator := memberctx.Username(c)

	var err error
	if req.Status == "confirmed" {
		err = db.ConfirmRedeemRecord(c.Request.Context(), req.ID, operator)
	} else {
		err = db.RejectRedeemRecord(c.Request.Context(), req.ID, operator)
	}
	switch {
	case err == nil:
		response.Success(c, gin.H{"id": req.ID, "status": req.Status})
	case errors.Is(err, store.ErrInsufficientPoints):
		response.BadRequest(c, "积分余额不足，无法确认兑换")
	case errors.Is(err, store.ErrRedeemNotPending):
		response.Conflict(c, "该兑换记录已处理，不能重复操作")
	case errors.Is(err, store.ErrRedeemRecordNotFound):
		response.NotFound(c, "兑换记录不存在")
	default:
		response.InternalServerError(c, "更新失败: "+err.Error())
	}
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
