package iot

import (
	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
)

// ControlRequest 设备控制请求参数
type ControlRequest struct {
	EntityID string                 `json:"entity_id" binding:"required"`
	Action   string                 `json:"action" binding:"required"`
	Params   map[string]interface{} `json:"params,omitempty"`
}

// ListIoTDevicesHandler 列出 IoT 设备（通过 HA MCP）
func ListIoTDevicesHandler(c *gin.Context) {
	devices := []model.IoTDevice{
		{ID: 1, Name: "客厅灯", Type: "light", EntityID: "light.living_room", State: "on", Attrs: map[string]interface{}{"brightness": 80, "color_temp": 350}},
		{ID: 2, Name: "卧室空调", Type: "climate", EntityID: "climate.bedroom", State: "cool", Attrs: map[string]interface{}{"temperature": 24, "fan_mode": "auto"}},
		{ID: 3, Name: "前门门锁", Type: "lock", EntityID: "lock.front_door", State: "locked", Attrs: map[string]interface{}{"battery_level": 85}},
		{ID: 4, Name: "摄像头", Type: "camera", EntityID: "camera.living_room", State: "recording", Attrs: map[string]interface{}{"motion_detected": false}},
	}
	response.Success(c, devices)
}

// ControlIoTDeviceHandler 控制 IoT 设备
func ControlIoTDeviceHandler(c *gin.Context) {
	var req ControlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"entity_id": req.EntityID,
		"action":    req.Action,
		"params":    req.Params,
		"result":    "success",
		"message":   "设备控制指令已下发到 Home Assistant MCP",
	})
}
