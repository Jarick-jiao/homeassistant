package archive

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// ListArchivedHandler 分页查询归档表数据（管理员）
// GET /api/archive/:table?limit=&offset=
func ListArchivedHandler(c *gin.Context) {
	table := c.Param("table")
	if !store.IsArchivableTable(table) {
		response.BadRequest(c, "不支持的归档表: "+table)
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	dbVal, exists := c.Get("db")
	if !exists || dbVal == nil {
		response.BadRequest(c, "数据库不可用")
		return
	}
	db, ok := dbVal.(*store.DB)
	if !ok {
		response.BadRequest(c, "数据库类型错误")
		return
	}

	list, total, err := db.ListArchived(c.Request.Context(), table, limit, offset)
	if err != nil {
		response.BadRequest(c, "查询归档失败: "+err.Error())
		return
	}

	// 同时返回活跃行数，便于前端展示容量信息
	active, _ := db.CountActive(c.Request.Context(), table)

	response.Success(c, gin.H{
		"table":         table,
		"list":          list,
		"total":         total,
		"active_count":  active,
		"limit":         limit,
		"offset":        offset,
	})
}

// ListArchiveTablesHandler 返回支持的归档表清单及容量上限
// GET /api/archive
func ListArchiveTablesHandler(c *gin.Context) {
	specs := store.ArchiveTableSpecs()
	result := make([]gin.H, 0, len(specs))
	dbVal, _ := c.Get("db")
	db, _ := dbVal.(*store.DB)
	for _, s := range specs {
		item := gin.H{
			"table":      s.Table,
			"archive":    s.Archive,
			"cap":         s.Cap,
			"time_column": s.TimeColumn,
		}
		if db != nil {
			if active, err := db.CountActive(c.Request.Context(), s.Table); err == nil {
				item["active_count"] = active
			}
			if archived, _, err := db.ListArchived(c.Request.Context(), s.Table, 1, 0); err == nil {
				_ = archived
				// 取归档总数
				var archivedTotal int64
				// 简化：通过 ListArchived 已返回 total，但这里再查一次总数
				_, archivedTotal, _ = db.ListArchived(c.Request.Context(), s.Table, 1, 0)
				item["archived_count"] = archivedTotal
			}
		}
		result = append(result, item)
	}
	response.Success(c, gin.H{"tables": result})
}
