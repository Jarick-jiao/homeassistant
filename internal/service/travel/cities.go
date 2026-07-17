package travel

import "strings"

// CityIATACode 城市到机场 IATA 代码映射
var CityIATACode = map[string]string{
	"北京": "PEK", "上海": "SHA", "广州": "CAN", "成都": "CTU",
	"深圳": "SZX", "杭州": "HGH", "南京": "NKG", "武汉": "WUH",
	"西安": "XIY", "重庆": "CKG", "郑州": "CGO", "长沙": "CSX",
	"青岛": "TAO", "大连": "DLC", "昆明": "KMG", "厦门": "XMN",
	"哈尔滨": "HRB", "沈阳": "SHE", "天津": "TSN", "济南": "TNA",
	"福州": "FOC", "合肥": "HFE", "贵阳": "KWE", "南宁": "NNG",
	"海口": "HAK", "三亚": "SYX", "乌鲁木齐": "URC", "拉萨": "LXA",
}

// TrainStationCode 12306 城市到站码映射（常用）
var TrainStationCode = map[string]string{
	"北京": "BJP", "上海": "SHH", "广州": "GZQ", "深圳": "SZQ",
	"杭州": "HZH", "南京": "NJH", "武汉": "WHN", "成都": "CDW",
	"西安": "XAY", "重庆": "CQW", "郑州": "ZZF", "长沙": "CSQ",
	"天津": "TJP", "济南": "JNK", "青岛": "QDK", "大连": "DLT",
	"沈阳": "SYT", "哈尔滨": "HBB", "福州": "FZS", "厦门": "XMS",
	"合肥": "HFH", "贵阳": "GIW", "南宁": "NNZ", "海口": "HKQ",
	"三亚": "SYQ", "昆明": "KMM", "石家庄": "SJP", "太原": "TYV",
	"兰州": "LZJ", "银川": "YQC", "西宁": "XNO", "呼和浩特": "HHC",
	"南昌": "NCG", "苏州": "SZH", "无锡": "WXH", "常州": "CZH",
	"宁波": "NBH", "温州": "WZH", "珠海": "ZHQ", "佛山": "FEQ",
	"东莞": "DAQ", "惠州": "HZQ", "中山": "ZSQ", "徐州": "XUH",
	"烟台": "YTK", "潍坊": "WFK", "洛阳": "LYF", "桂林": "GLZ",
	"绵阳": "MYW", "遵义": "ZYW", "大理": "DLC", "丽江": "LJX",
}

// GetIATACode 获取城市 IATA 代码，支持模糊匹配
// 匹配策略：精确匹配 → 包含匹配 → 返回空
func GetIATACode(city string) string {
	if code, ok := CityIATACode[city]; ok {
		return code
	}
	// 模糊匹配：检查城市名是否包含某个 key 或 key 是否包含城市名
	for k, code := range CityIATACode {
		if strings.Contains(city, k) || strings.Contains(k, city) {
			return code
		}
	}
	return ""
}

// GetTrainStationCode 获取城市 12306 站码，支持模糊匹配
func GetTrainStationCode(city string) string {
	if code, ok := TrainStationCode[city]; ok {
		return code
	}
	for k, code := range TrainStationCode {
		if strings.Contains(city, k) || strings.Contains(k, city) {
			return code
		}
	}
	return ""
}

// NormalizeCityName 统一城市名称格式
// 去除常见后缀如 "市"、"站"，统一空格
func NormalizeCityName(city string) string {
	city = strings.TrimSpace(city)
	city = strings.TrimSuffix(city, "市")
	city = strings.TrimSuffix(city, "站")
	return city
}
