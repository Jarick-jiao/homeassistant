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
		"weather-mcp",
		"1.0.0",
		server.WithLogging(),
	)

	weatherTool := mcp.NewTool("get_weather",
		mcp.WithDescription("获取指定城市的天气信息"),
		mcp.WithString("city",
			mcp.Required(),
			mcp.Description("城市名称，例如：北京、上海、广州"),
		),
	)

	s.AddTool(weatherTool, handleGetWeather)

	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

func handleGetWeather(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	city, ok := request.Params.Arguments["city"].(string)
	if !ok || city == "" {
		return nil, fmt.Errorf("city parameter is required")
	}

	rand.Seed(time.Now().UnixNano())

	conditions := []string{"晴", "多云", "阴", "小雨", "雷阵雨", "雪"}
	condition := conditions[rand.Intn(len(conditions))]

	weatherData := map[string]interface{}{
		"city":        city,
		"temperature": rand.Intn(25) + 10,
		"condition":   condition,
		"humidity":    rand.Intn(40) + 40,
		"wind_speed":  rand.Intn(20) + 5,
		"updated_at":  time.Now().Format("2006-01-02 15:04:05"),
	}

	jsonData, err := json.MarshalIndent(weatherData, "", "  ")
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}
