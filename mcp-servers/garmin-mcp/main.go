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
		"garmin-mcp",
		"1.0.0",
		server.WithLogging(),
	)

	stepsTool := mcp.NewTool("get_steps",
		mcp.WithDescription("获取 Garmin 步数数据"),
		mcp.WithString("member_id",
			mcp.Required(),
			mcp.Description("家庭成员 ID"),
		),
		mcp.WithString("date",
			mcp.Required(),
			mcp.Description("日期，格式 YYYY-MM-DD"),
		),
	)

	sleepTool := mcp.NewTool("get_sleep",
		mcp.WithDescription("获取 Garmin 睡眠数据"),
		mcp.WithString("member_id",
			mcp.Required(),
			mcp.Description("家庭成员 ID"),
		),
		mcp.WithString("date",
			mcp.Required(),
			mcp.Description("日期，格式 YYYY-MM-DD"),
		),
	)

	hrTool := mcp.NewTool("get_heart_rate",
		mcp.WithDescription("获取 Garmin 心率数据"),
		mcp.WithString("member_id",
			mcp.Required(),
			mcp.Description("家庭成员 ID"),
		),
		mcp.WithString("date",
			mcp.Required(),
			mcp.Description("日期，格式 YYYY-MM-DD"),
		),
	)

	stressTool := mcp.NewTool("get_stress",
		mcp.WithDescription("获取 Garmin 压力等级数据"),
		mcp.WithString("member_id",
			mcp.Required(),
			mcp.Description("家庭成员 ID"),
		),
		mcp.WithString("date",
			mcp.Required(),
			mcp.Description("日期，格式 YYYY-MM-DD"),
		),
	)

	s.AddTool(stepsTool, handleGetSteps)
	s.AddTool(sleepTool, handleGetSleep)
	s.AddTool(hrTool, handleGetHeartRate)
	s.AddTool(stressTool, handleGetStress)

	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

func handleGetSteps(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	memberID, _ := request.Params.Arguments["member_id"].(string)
	date, _ := request.Params.Arguments["date"].(string)

	rand.Seed(time.Now().UnixNano())
	data := map[string]interface{}{
		"member_id": memberID,
		"date":      date,
		"steps":     rand.Intn(8000) + 4000,
		"goal":      10000,
		"source":    "garmin",
	}

	jsonData, _ := json.MarshalIndent(data, "", "  ")
	return mcp.NewToolResultText(string(jsonData)), nil
}

func handleGetSleep(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	memberID, _ := request.Params.Arguments["member_id"].(string)
	date, _ := request.Params.Arguments["date"].(string)

	rand.Seed(time.Now().UnixNano())
	data := map[string]interface{}{
		"member_id":   memberID,
		"date":        date,
		"total_hours": rand.Float64()*3 + 5,
		"deep_hours":  rand.Float64()*2 + 1,
		"light_hours": rand.Float64()*2 + 2,
		"rem_hours":   rand.Float64()*1.5 + 0.5,
		"score":       rand.Intn(30) + 70,
		"source":      "garmin",
	}

	jsonData, _ := json.MarshalIndent(data, "", "  ")
	return mcp.NewToolResultText(string(jsonData)), nil
}

func handleGetHeartRate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	memberID, _ := request.Params.Arguments["member_id"].(string)
	date, _ := request.Params.Arguments["date"].(string)

	rand.Seed(time.Now().UnixNano())
	data := map[string]interface{}{
		"member_id": memberID,
		"date":      date,
		"resting":   rand.Intn(20) + 55,
		"max":       rand.Intn(40) + 140,
		"avg":       rand.Intn(25) + 65,
		"source":    "garmin",
	}

	jsonData, _ := json.MarshalIndent(data, "", "  ")
	return mcp.NewToolResultText(string(jsonData)), nil
}

func handleGetStress(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	memberID, _ := request.Params.Arguments["member_id"].(string)
	date, _ := request.Params.Arguments["date"].(string)

	rand.Seed(time.Now().UnixNano())
	data := map[string]interface{}{
		"member_id":   memberID,
		"date":        date,
		"avg_stress":  rand.Intn(40) + 20,
		"max_stress":  rand.Intn(30) + 60,
		"rest_stress": rand.Intn(15) + 10,
		"source":      "garmin",
	}

	jsonData, _ := json.MarshalIndent(data, "", "  ")
	return mcp.NewToolResultText(string(jsonData)), nil
}
