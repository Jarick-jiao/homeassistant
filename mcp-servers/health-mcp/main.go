package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"health-mcp",
		"1.0.0",
		server.WithLogging(),
	)

	syncGarminTool := mcp.NewTool("sync_garmin_data",
		mcp.WithDescription("同步 Garmin 健康数据"),
		mcp.WithString("member_id",
			mcp.Required(),
			mcp.Description("家庭成员 ID"),
		),
	)

	syncHuaweiTool := mcp.NewTool("sync_huawei_data",
		mcp.WithDescription("同步华为健康数据"),
		mcp.WithString("member_id",
			mcp.Required(),
			mcp.Description("家庭成员 ID"),
		),
	)

	healthSummaryTool := mcp.NewTool("get_health_summary",
		mcp.WithDescription("获取聚合健康摘要"),
		mcp.WithString("member_id",
			mcp.Required(),
			mcp.Description("家庭成员 ID"),
		),
	)

	s.AddTool(syncGarminTool, handleSyncGarmin)
	s.AddTool(syncHuaweiTool, handleSyncHuawei)
	s.AddTool(healthSummaryTool, handleHealthSummary)

	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

func handleSyncGarmin(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	memberID, ok := request.Params.Arguments["member_id"].(string)
	if !ok || memberID == "" {
		return nil, fmt.Errorf("member_id parameter is required")
	}

	rand.Seed(time.Now().UnixNano())
	data := map[string]interface{}{
		"source":      "garmin",
		"member_id":   memberID,
		"steps":       rand.Intn(8000) + 4000,
		"sleep_hours": rand.Float64()*3 + 5,
		"heart_rate":  rand.Intn(30) + 60,
		"sync_time":   time.Now().Format("2006-01-02 15:04:05"),
	}

	jsonData, _ := json.MarshalIndent(data, "", "  ")
	return mcp.NewToolResultText(string(jsonData)), nil
}

func handleSyncHuawei(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	memberID, ok := request.Params.Arguments["member_id"].(string)
	if !ok || memberID == "" {
		return nil, fmt.Errorf("member_id parameter is required")
	}

	rand.Seed(time.Now().UnixNano())
	data := map[string]interface{}{
		"source":      "huawei",
		"member_id":   memberID,
		"steps":       rand.Intn(7000) + 5000,
		"sleep_hours": rand.Float64()*2 + 6,
		"heart_rate":  rand.Intn(25) + 65,
		"spo2":        rand.Intn(5) + 95,
		"sync_time":   time.Now().Format("2006-01-02 15:04:05"),
	}

	jsonData, _ := json.MarshalIndent(data, "", "  ")
	return mcp.NewToolResultText(string(jsonData)), nil
}

func handleHealthSummary(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	memberID, ok := request.Params.Arguments["member_id"].(string)
	if !ok || memberID == "" {
		return nil, fmt.Errorf("member_id parameter is required")
	}

	rand.Seed(time.Now().UnixNano())
	data := map[string]interface{}{
		"member_id":      memberID,
		"date":           time.Now().Format("2006-01-02"),
		"total_steps":    rand.Intn(5000) + 6000,
		"avg_sleep":      rand.Float64()*2 + 6,
		"avg_heart_rate": rand.Intn(20) + 65,
		"data_sources":   []string{"garmin", "huawei"},
		"summary":        "今日健康数据正常，步数和睡眠均在建议范围内。",
	}

	jsonData, _ := json.MarshalIndent(data, "", "  ")
	return mcp.NewToolResultText(string(jsonData)), nil
}
