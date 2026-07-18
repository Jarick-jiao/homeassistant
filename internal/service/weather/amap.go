package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client 天气查询客户端抽象
type Client interface {
	GetWeather(ctx context.Context, adcode string) (*WeatherInfo, error)
}

// WeatherInfo 天气信息
type WeatherInfo struct {
	Province     string    // 省份
	City         string    // 城市
	Condition    string    // 天气现象（晴/多云/雨...）
	Temperature  string    // 实况温度
	Humidity     string    // 湿度
	WindDirection string   // 风向
	WindPower    string    // 风力
	ReportTime   string    // 发布时间
	Forecasts    []Forecast // 预报（extensions=all 时返回）
}

// Forecast 单日预报
type Forecast struct {
	Date         string
	DayWeather   string
	NightWeather string
	DayTemp      string
	NightTemp    string
}

// amapClient 高德天气客户端
type amapClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewClient 创建高德天气客户端
// apiKey 为空时返回 nil（调用方需 nil 检查）
func NewClient(apiKey, baseURL string) Client {
	if apiKey == "" {
		return nil
	}
	if baseURL == "" {
		baseURL = "https://restapi.amap.com/v3"
	}
	return &amapClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// amapResponse 高德天气 API 响应结构
type amapResponse struct {
	Status   string `json:"status"`        // 1=成功 0=失败
	Info     string `json:"info"`          // 状态说明
	Count    string `json:"count"`         // 返回结果数
	Lives    []struct {
		Province      string `json:"province"`
		City          string `json:"city"`
		Weather       string `json:"weather"`
		Temperature   string `json:"temperature"`
		Humidity      string `json:"humidity"`
		WindDirection string `json:"winddirection"`
		WindPower     string `json:"windpower"`
		ReportTime    string `json:"reporttime"`
	} `json:"lives"`
	Forecasts []struct {
		City       string `json:"city"`
		Adcode     string `json:"adcode"`
		Province   string `json:"province"`
		ReportTime string `json:"reporttime"`
		Casts      []struct {
			Date         string `json:"date"`
			Week         string `json:"week"`
			DayWeather   string `json:"dayweather"`
			NightWeather string `json:"nightweather"`
			DayTemp      string `json:"daytemp"`
			NightTemp    string `json:"nighttemp"`
			DayWind      string `json:"daywind"`
			NightWind    string `json:"nightwind"`
			DayPower     string `json:"daypower"`
			NightPower   string `json:"nightpower"`
		} `json:"casts"`
	} `json:"forecasts"`
}

// GetWeather 查询天气（默认返回预报 extensions=all）
func (c *amapClient) GetWeather(ctx context.Context, adcode string) (*WeatherInfo, error) {
	if adcode == "" {
		adcode = "110100" // 默认北京
	}

	u := fmt.Sprintf("%s/weather/weatherInfo?key=%s&city=%s&extensions=all",
		c.baseURL, url.QueryEscape(c.apiKey), url.QueryEscape(adcode))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用高德天气 API 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB 上限
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("高德 API 返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result amapResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析天气响应失败: %w", err)
	}

	if result.Status != "1" {
		return nil, fmt.Errorf("高德 API 失败: status=%s info=%s", result.Status, result.Info)
	}

	info := &WeatherInfo{}
	// 预报数据
	if len(result.Forecasts) > 0 {
		f := result.Forecasts[0]
		info.Province = f.Province
		info.City = f.City
		info.ReportTime = f.ReportTime
		for _, cast := range f.Casts {
			info.Forecasts = append(info.Forecasts, Forecast{
				Date:         cast.Date,
				DayWeather:   cast.DayWeather,
				NightWeather: cast.NightWeather,
				DayTemp:      cast.DayTemp,
				NightTemp:    cast.NightTemp,
			})
		}
		// 从预报中取当日白天天气作为实况近似
		if len(f.Casts) > 0 {
			info.Condition = f.Casts[0].DayWeather
			info.Temperature = f.Casts[0].DayTemp + "°C"
		}
	}

	// 实况数据（若同时有 lives，优先取实况）
	if len(result.Lives) > 0 {
		l := result.Lives[0]
		info.Province = l.Province
		info.City = l.City
		info.Condition = l.Weather
		info.Temperature = l.Temperature + "°C"
		info.Humidity = l.Humidity + "%"
		info.WindDirection = l.WindDirection
		info.WindPower = l.WindPower
		info.ReportTime = l.ReportTime
	}

	log.Printf("[AMAP] 天气查询成功: %s %s %s", info.City, info.Condition, info.Temperature)
	return info, nil
}

// FormatWeekendWeather 格式化为周末天气展示（周六/周日/温度/建议）
func FormatWeekendWeather(w *WeatherInfo) (saturday, sunday, temperature, advice string) {
	if w == nil {
		return "暂无数据", "暂无数据", "--", "未配置天气数据"
	}

	// 预报数据：取前两天的日期匹配周六周日
	forecasts := w.Forecasts
	if len(forecasts) == 0 {
		return w.Condition, w.Condition, w.Temperature, "暂无预报数据"
	}

	// 找出即将到来的周六周日
	now := time.Now()
	for offset := 0; offset < 14; offset++ {
		d := now.AddDate(0, 0, offset)
		weekday := d.Weekday()
		if weekday == time.Saturday {
			dateStr := d.Format("2006-01-02")
			// 匹配预报中的同日期
			for _, f := range forecasts {
				if f.Date == dateStr {
					saturday = "周六: " + f.DayWeather + " " + f.DayTemp + "°C"
				}
			}
		}
		if weekday == time.Sunday {
			dateStr := d.Format("2006-01-02")
			for _, f := range forecasts {
				if f.Date == dateStr {
					sunday = "周日: " + f.DayWeather + " " + f.DayTemp + "°C"
				}
			}
		}
	}

	if saturday == "" && len(forecasts) > 0 {
		saturday = "周六: " + forecasts[0].DayWeather + " " + forecasts[0].DayTemp + "°C"
	}
	if sunday == "" && len(forecasts) > 1 {
		sunday = "周日: " + forecasts[1].DayWeather + " " + forecasts[1].DayTemp + "°C"
	}

	temperature = w.Temperature

	// 根据天气给出建议
	cond := w.Condition
	switch {
	case strings.Contains(cond, "雨"):
		advice = "有雨，建议室内活动或带伞出行"
	case strings.Contains(cond, "雪"):
		advice = "有雪，注意保暖，适合室内亲子活动"
	case strings.Contains(cond, "晴"):
		advice = "天气晴好，适合户外活动"
	case strings.Contains(cond, "云") || strings.Contains(cond, "阴"):
		advice = "多云适宜，户外活动可带件外套"
	default:
		advice = "天气适宜，注意根据温度调整着装"
	}
	return
}
