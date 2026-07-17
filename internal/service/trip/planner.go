package trip

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/homemate/server/internal/mcpmanager"
)

// Preferences holds user preferences for trip planning.
type Preferences struct {
	Destination   string   `json:"destination"`
	DepartureCity string   `json:"departure_city"`
	StartDate     string   `json:"start_date"`
	EndDate       string   `json:"end_date"`
	Budget        int      `json:"budget"`
	Interests     []string `json:"interests"`
	TravelMode    string   `json:"travel_mode"` // driving, public_transport, flight
	Adults        int      `json:"adults"`
	Children      int      `json:"children"`
}

// TripPlan represents a structured weekend trip plan.
type TripPlan struct {
	MemberID      string        `json:"member_id"`
	Destination   string        `json:"destination"`
	StartDate     string        `json:"start_date"`
	EndDate       string        `json:"end_date"`
	Schedule      []ScheduleItem `json:"schedule"`
	Weather       *WeatherInfo  `json:"weather,omitempty"`
	Transport     *TransportInfo `json:"transport,omitempty"`
	EstimatedCost int           `json:"estimated_cost"`
	CreatedAt     time.Time     `json:"created_at"`
}

// ScheduleItem represents a single item in the trip schedule.
type ScheduleItem struct {
	Time        string `json:"time"`
	Activity    string `json:"activity"`
	Location    string `json:"location"`
	Description string `json:"description"`
	Type        string `json:"type"` // sightseeing, dining, transport, rest
}

// WeatherInfo holds weather forecast for the trip.
type WeatherInfo struct {
	City        string `json:"city"`
	Date        string `json:"date"`
	Condition   string `json:"condition"`
	Temperature string `json:"temperature"`
	Wind        string `json:"wind"`
}

// TransportInfo holds transportation details.
type TransportInfo struct {
	Mode          string `json:"mode"`
	Duration      string `json:"duration"`
	Distance      string `json:"distance"`
	RouteOverview string `json:"route_overview"`
}

// TripPlanner provides trip planning functionality.
type TripPlanner struct {
	mcpManager *mcpmanager.MCPClientManager
	registry   *mcpmanager.Registry
}

// NewTripPlanner creates a new trip planner.
func NewTripPlanner(mcpManager *mcpmanager.MCPClientManager, registry *mcpmanager.Registry) *TripPlanner {
	return &TripPlanner{
		mcpManager: mcpManager,
		registry:   registry,
	}
}

// PlanWeekendTrip plans a weekend trip based on user preferences.
func (p *TripPlanner) PlanWeekendTrip(ctx context.Context, memberID string, preferences Preferences) (*TripPlan, error) {
	plan := &TripPlan{
		MemberID:    memberID,
		Destination: preferences.Destination,
		StartDate:   preferences.StartDate,
		EndDate:     preferences.EndDate,
		CreatedAt:   time.Now(),
	}

	// Step 1: Search destination via 高德 MCP (maps_text_search)
	if err := p.searchDestination(ctx, plan); err != nil {
		return nil, fmt.Errorf("目的地搜索失败: %w", err)
	}

	// Step 2: Query weather via 高德 MCP (maps_weather)
	if err := p.queryWeather(ctx, plan); err != nil {
		// Non-fatal: continue without weather
		plan.Weather = nil
	}

	// Step 3: Query driving directions via 高德 MCP (maps_direction_driving)
	if preferences.TravelMode == "driving" {
		if err := p.queryDrivingRoute(ctx, plan, preferences.DepartureCity); err != nil {
			plan.Transport = nil
		}
	}

	// Step 4: Query flights via 飞常准 MCP if applicable
	if preferences.TravelMode == "flight" {
		if err := p.queryFlights(ctx, plan, preferences); err != nil {
			plan.Transport = nil
		}
	}

	// Step 5: Generate structured schedule
	plan.Schedule = p.generateSchedule(preferences)

	// Step 6: Estimate cost
	plan.EstimatedCost = p.estimateCost(preferences)

	return plan, nil
}

// searchDestination searches for destination information.
func (p *TripPlanner) searchDestination(ctx context.Context, plan *TripPlan) error {
	servers := p.registry.GetToolsForCategory(mcpmanager.CategoryMap)
	if len(servers) == 0 {
		return fmt.Errorf("no map server available")
	}

	result, err := p.mcpManager.CallTool(ctx, servers[0].Name, "maps_text_search", map[string]interface{}{
		"keywords": plan.Destination,
		"city":     plan.Destination,
		"offset":   5,
	})
	if err != nil {
		return err
	}

	_ = result
	return nil
}

// queryWeather queries weather information for the destination.
func (p *TripPlanner) queryWeather(ctx context.Context, plan *TripPlan) error {
	servers := p.registry.GetToolsForCategory(mcpmanager.CategoryMap)
	if len(servers) == 0 {
		return fmt.Errorf("no map server available")
	}

	result, err := p.mcpManager.CallTool(ctx, servers[0].Name, "maps_weather", map[string]interface{}{
		"city": plan.Destination,
	})
	if err != nil {
		return err
	}

	// Parse weather result
	if result != nil && len(result.Content) > 0 {
		contentBytes, _ := json.Marshal(result.Content)
		plan.Weather = &WeatherInfo{
			City:        plan.Destination,
			Date:        plan.StartDate,
			Condition:   string(contentBytes),
			Temperature: "待解析",
			Wind:        "待解析",
		}
	}

	return nil
}

// queryDrivingRoute queries driving directions.
func (p *TripPlanner) queryDrivingRoute(ctx context.Context, plan *TripPlan, origin string) error {
	servers := p.registry.GetToolsForCategory(mcpmanager.CategoryMap)
	if len(servers) == 0 {
		return fmt.Errorf("no map server available")
	}

	result, err := p.mcpManager.CallTool(ctx, servers[0].Name, "maps_direction_driving", map[string]interface{}{
		"origin":      origin,
		"destination": plan.Destination,
	})
	if err != nil {
		return err
	}

	if result != nil {
		contentBytes, _ := json.Marshal(result.Content)
		plan.Transport = &TransportInfo{
			Mode:          "driving",
			Duration:      "待解析",
			Distance:      "待解析",
			RouteOverview: string(contentBytes),
		}
	}

	return nil
}

// queryFlights queries flight information via 飞常准 MCP.
func (p *TripPlanner) queryFlights(ctx context.Context, plan *TripPlan, preferences Preferences) error {
	servers := p.registry.GetToolsForCategory(mcpmanager.CategoryFlight)
	if len(servers) == 0 {
		return fmt.Errorf("no flight server available")
	}

	// Search flights by departure and arrival
	result, err := p.mcpManager.CallTool(ctx, servers[0].Name, "searchFlightsByDepArr", map[string]interface{}{
		"dep": preferences.DepartureCity,
		"arr": preferences.Destination,
		"date": preferences.StartDate,
	})
	if err != nil {
		return err
	}

	if result != nil {
		contentBytes, _ := json.Marshal(result.Content)
		plan.Transport = &TransportInfo{
			Mode:          "flight",
			Duration:      "待解析",
			Distance:      "待解析",
			RouteOverview: string(contentBytes),
		}
	}

	return nil
}

// generateSchedule generates a structured schedule based on preferences.
func (p *TripPlanner) generateSchedule(preferences Preferences) []ScheduleItem {
	schedule := []ScheduleItem{
		{
			Time:        "08:00",
			Activity:    "出发",
			Location:    preferences.DepartureCity,
			Description: "从家出发，前往目的地",
			Type:        "transport",
		},
		{
			Time:        "10:30",
			Activity:    "抵达游览",
			Location:    preferences.Destination,
			Description: "抵达目的地，开始游览",
			Type:        "sightseeing",
		},
		{
			Time:        "12:30",
			Activity:    "午餐",
			Location:    preferences.Destination,
			Description: "品尝当地特色美食",
			Type:        "dining",
		},
		{
			Time:        "14:00",
			Activity:    "继续游览",
			Location:    preferences.Destination,
			Description: "参观主要景点",
			Type:        "sightseeing",
		},
		{
			Time:        "17:00",
			Activity:    "返程",
			Location:    preferences.Destination,
			Description: "结束一天的行程，返回家中",
			Type:        "transport",
		},
	}

	return schedule
}

// estimateCost estimates the total trip cost.
func (p *TripPlanner) estimateCost(preferences Preferences) int {
	baseCost := 500
	if preferences.TravelMode == "flight" {
		baseCost += 2000
	} else if preferences.TravelMode == "driving" {
		baseCost += 300
	}

	// Per person costs
	perPerson := 200
	totalPeople := preferences.Adults + preferences.Children
	return baseCost + perPerson*totalPeople
}
