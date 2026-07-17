package travel

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/homemate/server/internal/mcpmanager"
	"github.com/homemate/server/internal/store"
)

// ============ 数据类型定义 ============

// TrainResult 火车查询结果
type TrainResult struct {
	TrainCode    string             `json:"train_code"`
	FromStation  string             `json:"from_station"`
	ToStation    string             `json:"to_station"`
	StartTime    string             `json:"start_time"`
	EndTime      string             `json:"end_time"`
	Duration     string             `json:"duration"`
	Price        map[string]float64 `json:"price"`
	LeftTickets  map[string]int     `json:"left_tickets"`
}

// FlightResult 航班查询结果
type FlightResult struct {
	FlightNo    string `json:"flight_no"`
	Airline     string `json:"airline"`
	From        string `json:"from"`
	To          string `json:"to"`
	DepartTime  string `json:"depart_time"`
	ArriveTime  string `json:"arrive_time"`
	Price       float64 `json:"price"`
	CabinClass  string `json:"cabin_class"`
	LeftSeats   int    `json:"left_seats"`
}

// WeatherInfo 天气信息
type WeatherInfo struct {
	Date         string `json:"date"`
	Weather      string `json:"weather"`
	TempHigh     int    `json:"temp_high"`
	TempLow      int    `json:"temp_low"`
	WindDirection string `json:"wind_direction"`
	WindPower    string `json:"wind_power"`
}

// TripPlanRequest 旅行规划请求
type TripPlanRequest struct {
	From               string   `json:"from"`
	To                 string   `json:"to"`
	StartDate          string   `json:"start_date"`
	EndDate            string   `json:"end_date"`
	Travelers          int      `json:"travelers"`
	Preferences        []string `json:"preferences"`
	TransportPreference string  `json:"transport_preference"` // "train" / "flight" / "auto" / "any"
}

// TripPlanResult 旅行规划结果
type TripPlanResult struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	StartDate string         `json:"start_date"`
	EndDate   string         `json:"end_date"`
	Trains    []TrainResult  `json:"trains,omitempty"`
	Flights   []FlightResult `json:"flights,omitempty"`
	Weather   []WeatherInfo  `json:"weather,omitempty"`
	Notes     []string       `json:"notes,omitempty"`
}

// CompareResult 空铁比价结果
type CompareResult struct {
	Trains        []TrainResult  `json:"trains"`
	Flights       []FlightResult `json:"flights"`
	Recommendation string        `json:"recommendation"`
}

// ============ MCP Server 名称常量 ============

const (
	mcpServer12306     = "12306-mcp"
	mcpServerAmap      = "amap"
	mcpServerVariflight = "variflight"
)

// ============ Planner 旅行规划服务 ============

// Planner 旅行规划服务
type Planner struct {
	registry *mcpmanager.Registry
	manager  *mcpmanager.MCPClientManager
	db       *store.DB
}

// NewPlanner 创建规划服务
func NewPlanner(registry *mcpmanager.Registry, manager *mcpmanager.MCPClientManager, db *store.DB) *Planner {
	return &Planner{registry: registry, manager: manager, db: db}
}

// callTool 安全调用 MCP 工具，manager 为 nil 或 server 未连接时返回错误
func (p *Planner) callTool(ctx context.Context, serverName, toolName string, args map[string]interface{}) (json.RawMessage, error) {
	if p.manager == nil {
		return nil, fmt.Errorf("MCP 未初始化，请配置 MCP 服务后重试")
	}
	client, ok := p.manager.GetClient(serverName)
	if !ok || !client.Connected {
		return nil, fmt.Errorf("%s MCP 未连接，请配置后重试", serverName)
	}
	result, err := p.manager.CallTool(ctx, serverName, toolName, args)
	if err != nil {
		log.Printf("[TRAVEL] MCP 调用失败 server=%s tool=%s err=%v", serverName, toolName, err)
		return nil, err
	}
	// 将 mcp.CallToolResult 序列化为 json.RawMessage
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("序列化 MCP 结果失败: %w", err)
	}
	return json.RawMessage(raw), nil
}

// ============ SearchTrains 查询火车票 ============

// SearchTrains 查询火车票
// 1. 通过 MCP get-station-code-of-citys 获取站码
// 2. 通过 MCP get-tickets 查询车票
func (p *Planner) SearchTrains(ctx context.Context, fromCity, toCity, date string, filterFlags string) ([]TrainResult, error) {
	fromCity = NormalizeCityName(fromCity)
	toCity = NormalizeCityName(toCity)

	// 先尝试通过 MCP 获取站码
	fromStation := ""
	toStation := ""

	stationArgs := map[string]interface{}{
		"citys": fmt.Sprintf("%s,%s", fromCity, toCity),
	}
	stationResult, err := p.callTool(ctx, mcpServer12306, "get-station-code-of-citys", stationArgs)
	if err != nil {
		log.Printf("[TRAVEL] 获取站码失败，尝试本地映射: %v", err)
		// 降级：使用本地映射
		fromStation = GetTrainStationCode(fromCity)
		toStation = GetTrainStationCode(toCity)
	} else {
		// 解析 MCP 返回的站码
		fromStation, toStation, err = parseStationCodes(stationResult, fromCity, toCity)
		if err != nil {
			log.Printf("[TRAVEL] 解析站码失败，尝试本地映射: %v", err)
			fromStation = GetTrainStationCode(fromCity)
			toStation = GetTrainStationCode(toCity)
		}
	}

	if fromStation == "" || toStation == "" {
		return nil, fmt.Errorf("无法获取城市站码: %s(%s) → %s(%s)", fromCity, fromStation, toCity, toStation)
	}

	log.Printf("[TRAVEL] 查询火车: %s(%s) → %s(%s) 日期=%s 筛选=%s", fromCity, fromStation, toCity, toStation, date, filterFlags)

	// 查询车票
	ticketArgs := map[string]interface{}{
		"date":                date,
		"fromStation":         fromStation,
		"toStation":           toStation,
		"trainFilterFlags":    filterFlags,
	}
	ticketResult, err := p.callTool(ctx, mcpServer12306, "get-tickets", ticketArgs)
	if err != nil {
		return nil, err
	}

	trains, err := parseTrainResults(ticketResult)
	if err != nil {
		return nil, fmt.Errorf("解析火车票结果失败: %w", err)
	}

	log.Printf("[TRAVEL] 查到 %d 趟列车", len(trains))
	return trains, nil
}

// ============ SearchFlights 查询航班 ============

// SearchFlights 查询航班信息
func (p *Planner) SearchFlights(ctx context.Context, fromCity, toCity, date string) ([]FlightResult, error) {
	fromCity = NormalizeCityName(fromCity)
	toCity = NormalizeCityName(toCity)

	fromCode := GetIATACode(fromCity)
	toCode := GetIATACode(toCity)

	if fromCode == "" {
		return nil, fmt.Errorf("不支持的城市机场代码: %s，请确认城市名", fromCity)
	}
	if toCode == "" {
		return nil, fmt.Errorf("不支持的城市机场代码: %s，请确认城市名", toCity)
	}

	log.Printf("[TRAVEL] 查询航班: %s(%s) → %s(%s) 日期=%s", fromCity, fromCode, toCity, toCode, date)

	// 优先尝试 getFlightPriceBycities
	args := map[string]interface{}{
		"depCity":  fromCode,
		"arrCity":  toCode,
		"date":     date,
	}
	result, err := p.callTool(ctx, mcpServerVariflight, "getFlightPriceBycities", args)
	if err != nil {
		// 降级尝试 searchFlightsByDepArr
		log.Printf("[TRAVEL] getFlightPriceBycities 失败，尝试 searchFlightsByDepArr: %v", err)
		args2 := map[string]interface{}{
			"depAirport": fromCode,
			"arrAirport": toCode,
			"date":       date,
		}
		result, err = p.callTool(ctx, mcpServerVariflight, "searchFlightsByDepArr", args2)
		if err != nil {
			return nil, fmt.Errorf("飞常准 MCP 查询失败，请检查连接或稍后重试")
		}
	}

	flights, err := parseFlightResults(result, fromCode, toCode)
	if err != nil {
		return nil, fmt.Errorf("解析航班结果失败: %w", err)
	}

	log.Printf("[TRAVEL] 查到 %d 个航班", len(flights))
	return flights, nil
}

// ============ GetRouteDistance 查询驾车路线距离 ============

// GetRouteDistance 查询两点间驾车距离和时长
func (p *Planner) GetRouteDistance(ctx context.Context, from, to string) (distanceKm float64, durationMin int, err error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)

	log.Printf("[TRAVEL] 查询路线距离: %s → %s", from, to)

	// 先通过 maps_geo 获取坐标
	locations, err := p.geocodeLocations(ctx, from, to)
	if err != nil {
		log.Printf("[TRAVEL] 地理编码失败: %v，尝试直接使用地址", err)
		// 降级：直接用地名查询
		locations = map[string]string{
			"from": from,
			"to":   to,
		}
	}

	// 调用驾车路线规划
	dirArgs := map[string]interface{}{
		"origin":      locations["from"],
		"destination": locations["to"],
	}
	result, err := p.callTool(ctx, mcpServerAmap, "maps_direction_driving", dirArgs)
	if err != nil {
		return 0, 0, fmt.Errorf("高德 MCP 路线查询失败，请检查连接或稍后重试")
	}

	return parseRouteDistance(result)
}

// geocodeLocations 获取两个地点的地理编码坐标
func (p *Planner) geocodeLocations(ctx context.Context, from, to string) (map[string]string, error) {
	locations := make(map[string]string)

	for _, addr := range []struct {
		key, value string
	}{
		{"from", from},
		{"to", to},
	} {
		args := map[string]interface{}{
			"address": addr.value,
		}
		result, err := p.callTool(ctx, mcpServerAmap, "maps_geo", args)
		if err != nil {
			log.Printf("[TRAVEL] 地理编码失败 addr=%s: %v", addr.value, err)
			continue
		}
		loc, err := parseGeocodeResult(result)
		if err != nil {
			log.Printf("[TRAVEL] 解理地理编码失败 addr=%s: %v", addr.value, err)
			continue
		}
		locations[addr.key] = loc
	}

	if len(locations) < 2 {
		return nil, fmt.Errorf("地理编码不完整，仅获取到 %d/2 个坐标", len(locations))
	}
	return locations, nil
}

// ============ GetWeather 查询天气 ============

// GetWeather 查询城市天气预报
func (p *Planner) GetWeather(ctx context.Context, city string, days int) ([]WeatherInfo, error) {
	city = NormalizeCityName(city)

	if days <= 0 {
		days = 1
	}
	if days > 7 {
		days = 7
	}

	log.Printf("[TRAVEL] 查询天气: 城市=%s 天数=%d", city, days)

	// 高德天气：extensions=all 返回全部或预报
	extension := "base"
	if days > 1 {
		extension = "all"
	}

	args := map[string]interface{}{
		"city":      city,
		"extensions": extension,
	}
	result, err := p.callTool(ctx, mcpServerAmap, "maps_weather", args)
	if err != nil {
		return nil, fmt.Errorf("高德 MCP 天气查询失败，请检查连接或稍后重试")
	}

	weather, err := parseWeatherResults(result, days)
	if err != nil {
		return nil, fmt.Errorf("解析天气结果失败: %w", err)
	}

	log.Printf("[TRAVEL] 获取到 %d 天天气数据", len(weather))
	return weather, nil
}

// ============ PlanTrip 综合旅行规划 ============

// PlanTrip 综合旅行规划入口
func (p *Planner) PlanTrip(ctx context.Context, req *TripPlanRequest) (*TripPlanResult, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}

	req.From = NormalizeCityName(req.From)
	req.To = NormalizeCityName(req.To)

	if req.Travelers <= 0 {
		req.Travelers = 1
	}
	if req.TransportPreference == "" {
		req.TransportPreference = "any"
	}

	log.Printf("[TRAVEL] 开始综合规划: %s → %s (%s ~ %s) 交通偏好=%s 人数=%d",
		req.From, req.To, req.StartDate, req.EndDate, req.TransportPreference, req.Travelers)

	result := &TripPlanResult{
		From:      req.From,
		To:        req.To,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}

	var notes []string
	var errs []string

	// a. 火车查询
	if req.TransportPreference == "train" || req.TransportPreference == "any" {
		trains, err := p.SearchTrains(ctx, req.From, req.To, req.StartDate, "")
		if err != nil {
			errs = append(errs, fmt.Sprintf("火车票查询: %v", err))
			log.Printf("[TRAVEL] 火车票查询失败: %v", err)
		} else {
			result.Trains = trains
		}
	}

	// b. 航班查询
	if req.TransportPreference == "flight" || req.TransportPreference == "any" {
		flights, err := p.SearchFlights(ctx, req.From, req.To, req.StartDate)
		if err != nil {
			errs = append(errs, fmt.Sprintf("航班查询: %v", err))
			log.Printf("[TRAVEL] 航班查询失败: %v", err)
		} else {
			result.Flights = flights
		}
	}

	// c. 天气查询
	weather, err := p.GetWeather(ctx, req.To, 3)
	if err != nil {
		log.Printf("[TRAVEL] 天气查询失败: %v", err)
		notes = append(notes, fmt.Sprintf("目的地天气查询失败: %v", err))
	} else {
		result.Weather = weather
	}

	// d. 驾车距离（如果偏好 auto 或 any）
	if req.TransportPreference == "auto" || req.TransportPreference == "any" {
		dist, dur, err := p.GetRouteDistance(ctx, req.From, req.To)
		if err != nil {
			log.Printf("[TRAVEL] 路线距离查询失败: %v", err)
			notes = append(notes, fmt.Sprintf("驾车距离查询失败: %v", err))
		} else {
			notes = append(notes, fmt.Sprintf("驾车距离约 %.1f 公里，预计耗时 %d 分钟", dist, dur))
		}
	}

	// 添加出行建议
	if len(result.Trains) > 0 || len(result.Flights) > 0 {
		notes = append(notes, generateTravelNotes(result, req))
	}

	if len(errs) > 0 {
		result.Notes = append(result.Notes, fmt.Sprintf("部分查询失败: %s", strings.Join(errs, "; ")))
	}
	result.Notes = append(result.Notes, notes...)

	log.Printf("[TRAVEL] 综合规划完成: 火车=%d 航班=%d 天气=%d天",
		len(result.Trains), len(result.Flights), len(result.Weather))

	return result, nil
}

// ============ CompareTransport 空铁比价 ============

// CompareTransport 空铁比价：同时查火车和航班
func (p *Planner) CompareTransport(ctx context.Context, from, to, date string) (*CompareResult, error) {
	from = NormalizeCityName(from)
	to = NormalizeCityName(to)

	log.Printf("[TRAVEL] 空铁比价: %s → %s 日期=%s", from, to, date)

	result := &CompareResult{}
	var trainErr, flightErr error

	// 并行查询火车和航班
	trainCh := make(chan []TrainResult, 1)
	flightCh := make(chan []FlightResult, 1)
	errCh := make(chan error, 2)

	go func() {
		trains, err := p.SearchTrains(ctx, from, to, date, "")
		if err != nil {
			errCh <- err
			return
		}
		trainCh <- trains
	}()

	go func() {
		flights, err := p.SearchFlights(ctx, from, to, date)
		if err != nil {
			errCh <- err
			return
		}
		flightCh <- flights
	}()

	// 收集结果
	for i := 0; i < 2; i++ {
		select {
		case trains := <-trainCh:
			result.Trains = trains
		case flights := <-flightCh:
			result.Flights = flights
		case err := <-errCh:
			if trainErr == nil && flightErr == nil {
				trainErr = err
			} else {
				flightErr = err
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// 生成推荐
	result.Recommendation = generateCompareRecommendation(result.Trains, result.Flights, from, to)

	log.Printf("[TRAVEL] 比价完成: 火车=%d 航班=%d", len(result.Trains), len(result.Flights))
	return result, nil
}

// ============ MCP 结果解析函数 ============

// parseStationCodes 从 MCP 返回结果解析站码
func parseStationCodes(raw json.RawMessage, fromCity, toCity string) (string, string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", "", err
	}

	// 尝试从 content 字段获取（MCP tool 返回标准格式）
	content := extractContent(data)

	var stations map[string]string
	if c, ok := content[0].(map[string]interface{}); ok {
		if text, ok := c["text"].(string); ok {
			if err := json.Unmarshal([]byte(text), &stations); ok {
				return stations[fromCity], stations[toCity], nil
			}
		}
	}

	// 直接尝试作为 map 解析
	if err := json.Unmarshal(raw, &stations); err == nil {
		return stations[fromCity], stations[toCity], nil
	}

	return "", "", fmt.Errorf("无法解析站码数据")
}

// parseTrainResults 解析火车票查询结果
func parseTrainResults(raw json.RawMessage) ([]TrainResult, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}

	content := extractContent(data)

	// 尝试从 content[0].text 解析
	if len(content) > 0 {
		if c, ok := content[0].(map[string]interface{}); ok {
			if text, ok := c["text"].(string); ok {
				var trains []TrainResult
				if err := json.Unmarshal([]byte(text), &trains); err == nil && len(trains) > 0 {
					return trains, nil
				}
			}
		}
	}

	// 尝试直接从 data 解析
	var tickets []map[string]interface{}
	if arr, ok := data["tickets"].([]interface{}); ok {
		tickets = make([]map[string]interface{}, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				tickets = append(tickets, m)
			}
		}
	}

	if len(tickets) == 0 {
		// 尝试解析为数组
		if err := json.Unmarshal(raw, &tickets); err != nil {
			return nil, err
		}
	}

	var trains []TrainResult
	for _, t := range tickets {
		train := TrainResult{
			TrainCode:   getStringField(t, "train_code", "trainCode", "station_train_code"),
			FromStation: getStringField(t, "from_station", "from_station", "start_station_name"),
			ToStation:   getStringField(t, "to_station", "to_station", "end_station_name"),
			StartTime:   getStringField(t, "start_time", "start_time"),
			EndTime:     getStringField(t, "end_time", "end_time", "arrive_time"),
			Duration:    getStringField(t, "duration", "duration", "lishi"),
			Price:       parsePriceMap(t, "price"),
			LeftTickets: parseTicketCountMap(t, "leftTickets", "left_tickets"),
		}
		if train.TrainCode != "" {
			trains = append(trains, train)
		}
	}

	return trains, nil
}

// parseFlightResults 解析航班查询结果
func parseFlightResults(raw json.RawMessage, fromCode, toCode string) ([]FlightResult, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}

	content := extractContent(data)

	var flights []FlightResult

	if len(content) > 0 {
		if c, ok := content[0].(map[string]interface{}); ok {
			if text, ok := c["text"].(string); ok {
				if err := json.Unmarshal([]byte(text), &flights); err == nil && len(flights) > 0 {
					return flights, nil
				}
			}
		}
	}

	// 尝试直接解析
	var items []map[string]interface{}
	if arr, ok := data["flights"].([]interface{}); ok {
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				items = append(items, m)
			}
		}
	}
	if len(items) == 0 {
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
	}

	for _, f := range items {
		flight := FlightResult{
			FlightNo:   getStringField(f, "flight_no", "flightNo", "flightNumber"),
			Airline:    getStringField(f, "airline", "airlineName"),
			From:       getStringField(f, "from", "depAirport", "dport"),
			To:         getStringField(f, "to", "arrAirport", "aport"),
			DepartTime: getStringField(f, "depart_time", "depTime", "dtime"),
			ArriveTime: getStringField(f, "arrive_time", "arrTime", "atime"),
			Price:      getFloatField(f, "price", "parPrice"),
			CabinClass: getStringField(f, "cabin_class", "cabinClass", "defaultCabinClass"),
			LeftSeats:  getIntField(f, "left_seats", "seats"),
		}
		if flight.From == "" {
			flight.From = fromCode
		}
		if flight.To == "" {
			flight.To = toCode
		}
		if flight.FlightNo != "" {
			flights = append(flights, flight)
		}
	}

	return flights, nil
}

// parseRouteDistance 解析驾车路线距离
func parseRouteDistance(raw json.RawMessage) (distanceKm float64, durationMin int, err error) {
	var data map[string]interface{}
	if err = json.Unmarshal(raw, &data); err != nil {
		return
	}

	content := extractContent(data)
	if len(content) > 0 {
		if c, ok := content[0].(map[string]interface{}); ok {
			if text, ok := c["text"].(string); ok {
				var routeData map[string]interface{}
				if json.Unmarshal([]byte(text), &routeData) == nil {
					if r, ok := routeData["route"].(map[string]interface{}); ok {
						if paths, ok := r["paths"].([]interface{}); ok && len(paths) > 0 {
							if path, ok := paths[0].(map[string]interface{}); ok {
								distanceKm = getFloatField(path, "distance") / 1000.0
								durationMin = getIntField(path, "duration") / 60
								if distanceKm > 0 {
									return
								}
							}
						}
					}
				}
			}
		}
	}

	// 直接尝试从顶层字段获取
	if r, ok := data["route"].(map[string]interface{}); ok {
		if paths, ok := r["paths"].([]interface{}); ok && len(paths) > 0 {
			if path, ok := paths[0].(map[string]interface{}); ok {
				distanceKm = getFloatField(path, "distance") / 1000.0
				durationMin = getIntField(path, "duration") / 60
				return
			}
		}
	}

	distanceKm = getFloatField(data, "distance")
	durationMin = getIntField(data, "duration")
	return
}

// parseGeocodeResult 解析地理编码结果，返回经纬度字符串 "lng,lat"
func parseGeocodeResult(raw json.RawMessage) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}

	content := extractContent(data)
	if len(content) > 0 {
		if c, ok := content[0].(map[string]interface{}); ok {
			if text, ok := c["text"].(string); ok {
				var geoData []map[string]interface{}
				if json.Unmarshal([]byte(text), &geoData) == nil && len(geoData) > 0 {
					loc := geoData[0]
					if locStr, ok := loc["location"].(string); ok && locStr != "" {
						return locStr, nil
					}
					lng := getFloatField(loc, "lng", "longitude")
					lat := getFloatField(loc, "lat", "latitude")
					if lng != 0 && lat != 0 {
						return fmt.Sprintf("%f,%f", lng, lat), nil
					}
				}
			}
		}
	}

	// 直接解析
	if geocodes, ok := data["geocodes"].([]interface{}); ok && len(geocodes) > 0 {
		if geo, ok := geocodes[0].(map[string]interface{}); ok {
			if locStr, ok := geo["location"].(string); ok {
				return locStr, nil
			}
		}
	}

	return "", fmt.Errorf("未找到有效的地理编码结果")
}

// parseWeatherResults 解析天气预报结果
func parseWeatherResults(raw json.RawMessage, maxDays int) ([]WeatherInfo, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}

	content := extractContent(data)

	var forecasts []WeatherInfo

	if len(content) > 0 {
		if c, ok := content[0].(map[string]interface{}); ok {
			if text, ok := c["text"].(string); ok {
				var weatherData map[string]interface{}
				if json.Unmarshal([]byte(text), &weatherData) == nil {
					forecasts = parseForecastsFromMap(weatherData, maxDays)
					if len(forecasts) > 0 {
						return forecasts, nil
					}
				}
			}
		}
	}

	// 直接解析
	forecasts = parseForecastsFromMap(data, maxDays)
	return forecasts, nil
}

// parseForecastsFromMap 从 map 中解析天气预报
func parseForecastsFromMap(data map[string]interface{}, maxDays int) []WeatherInfo {
	var forecasts []WeatherInfo

	// 高德天气返回格式：forecasts -> [casts]
	if forecastsData, ok := data["forecasts"].([]interface{}); ok && len(forecastsData) > 0 {
		if fc, ok := forecastsData[0].(map[string]interface{}); ok {
			if casts, ok := fc["casts"].([]interface{}); ok {
				for i, c := range casts {
					if i >= maxDays {
						break
					}
					if cast, ok := c.(map[string]interface{}); ok {
						info := WeatherInfo{
							Date:          getStringField(cast, "date"),
							Weather:       getStringField(cast, "weather"),
							TempHigh:      getIntField(cast, "temp_high", "daytemp"),
							TempLow:       getIntField(cast, "temp_low", "nighttemp"),
							WindDirection: getStringField(cast, "wind_direction", "daywind"),
							WindPower:     getStringField(cast, "wind_power", "daypower"),
						}
						forecasts = append(forecasts, info)
					}
				}
			}
		}
	}

	return forecasts
}

// ============ 辅助函数 ============

// extractContent 从 MCP 标准响应中提取 content 字段
func extractContent(data map[string]interface{}) []interface{} {
	if content, ok := data["content"].([]interface{}); ok {
		return content
	}
	return nil
}

// getStringField 从 map 中获取字符串字段（支持多个备选 key）
func getStringField(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// getFloatField 从 map 中获取浮点数字段
func getFloatField(m map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch n := v.(type) {
			case float64:
				return n
			case int:
				return float64(n)
			case int64:
				return float64(n)
			case string:
				var f float64
				if _, err := fmt.Sscanf(n, "%f", &f); err == nil {
					return f
				}
			}
		}
	}
	return 0
}

// getIntField 从 map 中获取整数字段
func getIntField(m map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			case int:
				return n
			case int64:
				return int(n)
			case string:
				var i int
				if _, err := fmt.Sscanf(n, "%d", &i); err == nil {
					return i
				}
			}
		}
	}
	return 0
}

// parsePriceMap 解析价格 map
func parsePriceMap(m map[string]interface{}, keys ...string) map[string]float64 {
	var priceData interface{}
	for _, key := range keys {
		if v, ok := m[key]; ok {
			priceData = v
			break
		}
	}
	if priceData == nil {
		return nil
	}

	prices := make(map[string]float64)
	switch pd := priceData.(type) {
	case map[string]interface{}:
		for k, v := range pd {
			prices[k] = getFloatField(map[string]interface{}{"_v": v}, "_v")
		}
	case map[string]float64:
		return pd
	}
	return prices
}

// parseTicketCountMap 解析余票数 map
func parseTicketCountMap(m map[string]interface{}, keys ...string) map[string]int {
	var ticketData interface{}
	for _, key := range keys {
		if v, ok := m[key]; ok {
			ticketData = v
			break
		}
	}
	if ticketData == nil {
		return nil
	}

	tickets := make(map[string]int)
	switch td := ticketData.(type) {
	case map[string]interface{}:
		for k, v := range td {
			tickets[k] = getIntField(map[string]interface{}{"_v": v}, "_v")
		}
	case map[string]int:
		return td
	}
	return tickets
}

// generateTravelNotes 生成出行建议
func generateTravelNotes(result *TripPlanResult, req *TripPlanRequest) string {
	var notes []string

	// 天气建议
	if len(result.Weather) > 0 {
		firstDay := result.Weather[0]
		if strings.Contains(firstDay.Weather, "雨") {
			notes = append(notes, "目的地首日有雨，建议携带雨具")
		}
		if strings.Contains(firstDay.Weather, "雪") {
			notes = append(notes, "目的地首日有雪，注意保暖和路面湿滑")
		}
		if firstDay.TempLow <= 0 || firstDay.TempHigh <= 5 {
			notes = append(notes, "气温较低，请注意保暖")
		}
		if firstDay.TempHigh >= 35 {
			notes = append(notes, "气温较高，注意防暑降温")
		}
	}

	// 价格建议
	if len(result.Trains) > 0 && len(result.Flights) > 0 {
		cheapestTrain := math.MaxFloat64
		for _, t := range result.Trains {
			for _, p := range t.Price {
				if p > 0 && p < cheapestTrain {
					cheapestTrain = p
				}
			}
		}
		cheapestFlight := math.MaxFloat64
		for _, f := range result.Flights {
			if f.Price > 0 && f.Price < cheapestFlight {
				cheapestFlight = f.Price
			}
		}
		if cheapestTrain < math.MaxFloat64 && cheapestFlight < math.MaxFloat64 {
			if cheapestTrain < cheapestFlight {
				notes = append(notes, fmt.Sprintf("火车票价更低（最低 %.0f 元 vs 飞机 %.0f 元）", cheapestTrain, cheapestFlight))
			} else {
				notes = append(notes, fmt.Sprintf("飞机票价更低（最低 %.0f 元 vs 火车 %.0f 元）", cheapestFlight, cheapestTrain))
			}
		}
	}

	if len(notes) == 0 {
		return ""
	}
	return strings.Join(notes, "；")
}

// generateCompareRecommendation 生成比价推荐文案
func generateCompareRecommendation(trains []TrainResult, flights []FlightResult, from, to string) string {
	var parts []string

	if len(trains) == 0 && len(flights) == 0 {
		return fmt.Sprintf("未查询到 %s → %s 的火车和航班信息，请检查日期或城市是否正确", from, to)
	}

	// 找最低价
	cheapestTrain := math.MaxFloat64
	fastestTrainDur := ""
	for _, t := range trains {
		for _, p := range t.Price {
			if p > 0 && p < cheapestTrain {
				cheapestTrain = p
			}
		}
		if fastestTrainDur == "" || t.Duration < fastestTrainDur {
			fastestTrainDur = t.Duration
		}
	}

	cheapestFlight := math.MaxFloat64
	fastestFlight := ""
	for _, f := range flights {
		if f.Price > 0 && f.Price < cheapestFlight {
			cheapestFlight = f.Price
		}
		dt := f.DepartTime
		if at := f.ArriveTime; dt != "" && at != "" {
			if fastestFlight == "" {
				fastestFlight = fmt.Sprintf("%s-%s", dt, at)
			}
		}
	}

	if len(trains) > 0 {
		parts = append(parts, fmt.Sprintf("火车：%d 趟可选，最低 %.0f 元", len(trains), cheapestTrain))
	}
	if len(flights) > 0 {
		parts = append(parts, fmt.Sprintf("航班：%d 个可选，最低 %.0f 元", len(flights), cheapestFlight))
	}

	// 综合推荐
	if cheapestTrain < math.MaxFloat64 && cheapestFlight < math.MaxFloat64 {
		if cheapestTrain < cheapestFlight {
			parts = append(parts, fmt.Sprintf("推荐：选择火车出行更经济（节省 %.0f 元）", cheapestFlight-cheapestTrain))
		} else if cheapestFlight < cheapestTrain {
			parts = append(parts, fmt.Sprintf("推荐：选择飞机出行更经济（节省 %.0f 元）", cheapestTrain-cheapestFlight))
		} else {
			parts = append(parts, "推荐：火车和飞机价格相当，可根据时间偏好选择")
		}
	}

	return strings.Join(parts, "；")
}
