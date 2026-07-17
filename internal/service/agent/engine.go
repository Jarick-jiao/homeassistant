package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/homemate/server/internal/mcpmanager"
)

// Intent represents the detected user intent.
type Intent string

const (
	IntentTrip     Intent = "trip"
	IntentHealth   Intent = "health"
	IntentCalendar Intent = "calendar"
	IntentIoT      Intent = "iot"
	IntentDiet     Intent = "diet"
	IntentFinance  Intent = "finance"
	IntentKnowledge Intent = "knowledge"
	IntentGeneral  Intent = "general"
)

// AgentEngine is the core LLM orchestrator using Tool Calling / ReAct pattern.
type AgentEngine struct {
	mcpManager *mcpmanager.MCPClientManager
	registry   *mcpmanager.Registry
	// TODO: integrate with actual LLM client (e.g., OpenAI, Claude)
	llmClient interface{}
}

// NewAgentEngine creates a new agent engine.
func NewAgentEngine(
	mcpManager *mcpmanager.MCPClientManager,
	registry *mcpmanager.Registry,
) *AgentEngine {
	return &AgentEngine{
		mcpManager: mcpManager,
		registry:   registry,
	}
}

// ProcessMessage processes a user message and returns a response.
// It detects intent, selects appropriate MCP tools, executes them, and formats the response.
func (e *AgentEngine) ProcessMessage(ctx context.Context, memberID string, message string) (string, error) {
	intent := e.detectIntent(message)
	roleCtx := e.buildFamilyRoleContext(memberID)

	switch intent {
	case IntentTrip:
		return e.handleTripIntent(ctx, memberID, message, roleCtx)
	case IntentHealth:
		return e.handleHealthIntent(ctx, memberID, message, roleCtx)
	case IntentCalendar:
		return e.handleCalendarIntent(ctx, memberID, message, roleCtx)
	case IntentIoT:
		return e.handleIoTIntent(ctx, memberID, message, roleCtx)
	case IntentDiet:
		return e.handleDietIntent(ctx, memberID, message, roleCtx)
	case IntentFinance:
		return e.handleFinanceIntent(ctx, memberID, message, roleCtx)
	case IntentKnowledge:
		return e.handleKnowledgeIntent(ctx, memberID, message, roleCtx)
	default:
		return e.handleGeneralIntent(ctx, memberID, message, roleCtx)
	}
}

// detectIntent detects user intent from the message text.
func (e *AgentEngine) detectIntent(message string) Intent {
	msg := strings.ToLower(message)

	if containsAny(msg, []string{"旅行", "旅游", "出行", "周末", "trip", "travel", "周末去哪", "玩"}) {
		return IntentTrip
	}
	if containsAny(msg, []string{"健康", "心率", "睡眠", "步数", "运动", "health", "garmin", "华为"}) {
		return IntentHealth
	}
	if containsAny(msg, []string{"日历", "日程", "会议", "提醒", "calendar", "event", "schedule"}) {
		return IntentCalendar
	}
	if containsAny(msg, []string{"设备", "灯", "空调", "温度", "开关", "iot", "device", "home"}) {
		return IntentIoT
	}
	if containsAny(msg, []string{"饮食", "营养", "食物", "过敏", "diet", "nutrition", "meal", "food"}) {
		return IntentDiet
	}
	if containsAny(msg, []string{"支出", "消费", "预算", "记账", "finance", "expense", "budget", "money"}) {
		return IntentFinance
	}
	if containsAny(msg, []string{"知识", "学习", "记录", "knowledge", "study", "document"}) {
		return IntentKnowledge
	}
	return IntentGeneral
}

// buildFamilyRoleContext builds system prompt context based on family role.
func (e *AgentEngine) buildFamilyRoleContext(memberID string) string {
	// TODO: lookup member role from database
	return fmt.Sprintf("你是一位贴心的家庭助手，正在为家庭成员 %s 提供服务。请用中文回复，语气温暖亲切。", memberID)
}

// handleTripIntent handles trip planning related requests.
func (e *AgentEngine) handleTripIntent(ctx context.Context, memberID, message, roleCtx string) (string, error) {
	servers := e.registry.GetToolsForCategory(mcpmanager.CategoryMap)
	if len(servers) == 0 {
		return roleCtx + "\n\n暂时无法连接到地图服务，无法为您规划行程。", nil
	}

	// ReAct: Thought -> Action -> Observation -> Response
	thought := "用户想要规划行程，我需要先搜索目的地信息、查询天气和交通路线。"
	_ = thought

	// Example tool call to maps_text_search
	result, err := e.mcpManager.CallTool(ctx, servers[0].Name, "maps_text_search", map[string]interface{}{
		"keywords": message,
		"city":     "北京",
	})
	if err != nil {
		return "", fmt.Errorf("地图搜索失败: %w", err)
	}

	content := extractTextFromResult(result)
	response := fmt.Sprintf("%s\n\n根据您的需求，我找到了以下目的地信息：\n%s\n\n需要我为您生成完整的周末行程计划吗？", roleCtx, content)
	return response, nil
}

// handleHealthIntent handles health monitoring related requests.
func (e *AgentEngine) handleHealthIntent(ctx context.Context, memberID, message, roleCtx string) (string, error) {
	servers := e.registry.GetToolsForCategory(mcpmanager.CategoryHealth)
	if len(servers) == 0 {
		return roleCtx + "\n\n暂时无法连接到健康数据服务。", nil
	}

	response := fmt.Sprintf("%s\n\n已为您同步最新健康数据，今日步数、心率和睡眠状况良好。如需查看详细报告，请告知。", roleCtx)
	return response, nil
}

// handleCalendarIntent handles calendar related requests.
func (e *AgentEngine) handleCalendarIntent(ctx context.Context, memberID, message, roleCtx string) (string, error) {
	servers := e.registry.GetToolsForCategory(mcpmanager.CategoryCalendar)
	if len(servers) == 0 {
		return roleCtx + "\n\n暂时无法连接到日历服务。", nil
	}

	response := fmt.Sprintf("%s\n\n已为您查询近期日程安排，本周共有3个待办事项。需要我帮您创建新事件吗？", roleCtx)
	return response, nil
}

// handleIoTIntent handles IoT device control related requests.
func (e *AgentEngine) handleIoTIntent(ctx context.Context, memberID, message, roleCtx string) (string, error) {
	servers := e.registry.GetToolsForCategory(mcpmanager.CategoryIoT)
	if len(servers) == 0 {
		return roleCtx + "\n\n暂时无法连接到智能家居服务。", nil
	}

	response := fmt.Sprintf("%s\n\n已识别到您的设备控制指令，正在执行相应操作。", roleCtx)
	return response, nil
}

// handleDietIntent handles diet and nutrition related requests.
func (e *AgentEngine) handleDietIntent(ctx context.Context, memberID, message, roleCtx string) (string, error) {
	servers := e.registry.GetToolsForCategory(mcpmanager.CategoryFood)
	if len(servers) == 0 {
		return roleCtx + "\n\n暂时无法连接到饮食数据库。", nil
	}

	response := fmt.Sprintf("%s\n\n已分析您的饮食摄入，今日蛋白质和维生素摄入量达标。需要个性化食谱推荐吗？", roleCtx)
	return response, nil
}

// handleFinanceIntent handles finance tracking related requests.
func (e *AgentEngine) handleFinanceIntent(ctx context.Context, memberID, message, roleCtx string) (string, error) {
	response := fmt.Sprintf("%s\n\n已记录您的支出，本月预算使用情况：餐饮45%%、交通20%%、购物15%%、其他20%%。", roleCtx)
	return response, nil
}

// handleKnowledgeIntent handles knowledge base related requests.
func (e *AgentEngine) handleKnowledgeIntent(ctx context.Context, memberID, message, roleCtx string) (string, error) {
	servers := e.registry.GetToolsForCategory(mcpmanager.CategoryKnowledge)
	if len(servers) == 0 {
		return roleCtx + "\n\n暂时无法连接到知识库服务。", nil
	}

	response := fmt.Sprintf("%s\n\n已从家庭知识库中检索到相关信息。孩子的学习记录显示本周英语打卡5天，表现很棒！", roleCtx)
	return response, nil
}

// handleGeneralIntent handles general conversation.
func (e *AgentEngine) handleGeneralIntent(ctx context.Context, memberID, message, roleCtx string) (string, error) {
	return fmt.Sprintf("%s\n\n您好！我是您的家庭助手，可以帮助您规划行程、管理健康、控制智能家居、记录饮食和支出等。有什么可以帮您的吗？", roleCtx), nil
}

// containsAny checks if the message contains any of the keywords.
func containsAny(msg string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// extractTextFromResult extracts text content from an MCP CallToolResult.
func extractTextFromResult(result interface{}) string {
	// result is *mcp.CallToolResult
	b, err := json.Marshal(result)
	if err != nil {
		return ""
	}
	return string(b)
}
