package iot

import (
	"context"
	"fmt"

	"github.com/homemate/server/internal/mcpmanager"
)

// Device represents a smart home device.
type Device struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Type     string                 `json:"type"` // light, thermostat, switch, sensor, camera
	Room     string                 `json:"room"`
	State    map[string]interface{} `json:"state"`
	Online   bool                   `json:"online"`
	Metadata map[string]string      `json:"metadata,omitempty"`
}

// DeviceAction represents an action to perform on a device.
type DeviceAction struct {
	Action string                 `json:"action"` // on, off, set_temperature, set_brightness, set_color
	Params map[string]interface{} `json:"params,omitempty"`
}

// IoTController provides IoT device control functionality.
type IoTController struct {
	mcpManager *mcpmanager.MCPClientManager
	registry   *mcpmanager.Registry
}

// NewIoTController creates a new IoT controller.
func NewIoTController(mcpManager *mcpmanager.MCPClientManager, registry *mcpmanager.Registry) *IoTController {
	return &IoTController{
		mcpManager: mcpManager,
		registry:   registry,
	}
}

// ListDevices lists all available smart home devices.
func (c *IoTController) ListDevices(ctx context.Context) ([]Device, error) {
	servers := c.registry.GetToolsForCategory(mcpmanager.CategoryIoT)
	if len(servers) == 0 {
		return nil, fmt.Errorf("no iot server available")
	}

	result, err := c.mcpManager.CallTool(ctx, servers[0].Name, "listDevices", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("list devices failed: %w", err)
	}

	_ = result
	// TODO: parse result content into []Device
	return []Device{}, nil
}

// GetDeviceState retrieves the current state of a specific device.
func (c *IoTController) GetDeviceState(ctx context.Context, deviceID string) (*Device, error) {
	servers := c.registry.GetToolsForCategory(mcpmanager.CategoryIoT)
	if len(servers) == 0 {
		return nil, fmt.Errorf("no iot server available")
	}

	result, err := c.mcpManager.CallTool(ctx, servers[0].Name, "getDeviceState", map[string]interface{}{
		"device_id": deviceID,
	})
	if err != nil {
		return nil, fmt.Errorf("get device state failed: %w", err)
	}

	_ = result
	// TODO: parse result into Device
	return &Device{ID: deviceID, State: make(map[string]interface{})}, nil
}

// ControlDevice sends a control command to a specific device.
func (c *IoTController) ControlDevice(ctx context.Context, deviceID string, action DeviceAction) error {
	servers := c.registry.GetToolsForCategory(mcpmanager.CategoryIoT)
	if len(servers) == 0 {
		return fmt.Errorf("no iot server available")
	}

	_, err := c.mcpManager.CallTool(ctx, servers[0].Name, "controlDevice", map[string]interface{}{
		"device_id": deviceID,
		"action":    action.Action,
		"params":    action.Params,
	})
	if err != nil {
		return fmt.Errorf("control device failed: %w", err)
	}

	return nil
}
