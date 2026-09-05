package calendar

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/homemate/server/internal/model"
)

// NextOccurrence 计算下次事件发生时间
// baseTime: 事件原始开始时间
// rule: 周期规则
// after: 从此时间之后查找下一次（通常为 now）
// 返回下次发生时间，无更多则返回零值
func NextOccurrence(baseTime time.Time, rule model.RecurrenceRule, after time.Time) (time.Time, error) {
	if err := IsValidRule(rule); err != nil {
		return time.Time{}, err
	}

	interval := rule.Interval
	if interval <= 0 {
		interval = 1
	}

	// 从 baseTime 开始按步长推进，直到超过 after
	current := baseTime
	maxIter := 1000 // 防死循环
	for i := 0; i < maxIter; i++ {
		if current.After(after) {
			// 检查 until 边界
			if rule.Until != "" {
				until, err := time.ParseInLocation("2006-01-02", rule.Until, time.Local)
				if err == nil && current.After(until.Add(24*time.Hour)) {
					return time.Time{}, nil
				}
			}
			// 检查 count 边界（已生成实例数）
			// 简化：count 仅在 ExpandOccurrences 中校验，这里按 until 优先
			return current, nil
		}
		current = advanceByFreq(current, rule.Freq, interval, rule.ByWeekday)
	}
	return time.Time{}, fmt.Errorf("超过最大迭代次数")
}

// ExpandOccurrences 展开区间内所有实例
func ExpandOccurrences(baseTime time.Time, rule model.RecurrenceRule, from, to time.Time) ([]time.Time, error) {
	if err := IsValidRule(rule); err != nil {
		return nil, err
	}
	interval := rule.Interval
	if interval <= 0 {
		interval = 1
	}

	var instances []time.Time
	current := baseTime
	maxIter := 10000
	var untilTime time.Time
	hasUntil := false
	if rule.Until != "" {
		var err error
		untilTime, err = time.ParseInLocation("2006-01-02", rule.Until, time.Local)
		if err == nil {
			untilTime = untilTime.Add(24 * time.Hour)
			hasUntil = true
		}
	}

	for i := 0; i < maxIter; i++ {
		if hasUntil && current.After(untilTime) {
			break
		}
		if rule.Count > 0 && i >= rule.Count {
			break
		}
		if !current.Before(from) && !current.After(to) {
			instances = append(instances, current)
		}
		if current.After(to) {
			break
		}
		current = advanceByFreq(current, rule.Freq, interval, rule.ByWeekday)
		if current.IsZero() {
			break
		}
	}
	return instances, nil
}

// advanceByFreq 按频率推进时间
func advanceByFreq(t time.Time, freq string, interval int, byweekday []int) time.Time {
	switch freq {
	case "daily":
		return t.AddDate(0, 0, interval)
	case "weekly":
		if len(byweekday) > 0 {
			// 找下一个匹配的 weekday
			for d := 1; d <= 7; d++ {
				next := t.AddDate(0, 0, d)
				for _, wd := range byweekday {
					if int(next.Weekday()) == wd {
						return next
					}
				}
			}
		}
		return t.AddDate(0, 0, 7*interval)
	case "monthly":
		return t.AddDate(0, interval, 0)
	case "yearly":
		return t.AddDate(interval, 0, 0)
	}
	return t.AddDate(0, 0, interval)
}

// IsValidRule 校验规则合法性
func IsValidRule(rule model.RecurrenceRule) error {
	switch rule.Freq {
	case "daily", "weekly", "monthly", "yearly":
		// valid
	default:
		return fmt.Errorf("非法频率: %s", rule.Freq)
	}
	if rule.Interval < 0 {
		return fmt.Errorf("间隔不能为负")
	}
	if rule.Count < 0 {
		return fmt.Errorf("次数不能为负")
	}
	return nil
}

// ParseRule 从 JSON 字符串解析规则
func ParseRule(ruleJSON string) (*model.RecurrenceRule, error) {
	if ruleJSON == "" {
		return nil, nil
	}
	var rule model.RecurrenceRule
	if err := json.Unmarshal([]byte(ruleJSON), &rule); err != nil {
		return nil, fmt.Errorf("解析周期规则失败: %w", err)
	}
	if err := IsValidRule(rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

// DescribeRule 生成人类可读描述
func DescribeRule(ruleJSON string) string {
	rule, err := ParseRule(ruleJSON)
	if err != nil || rule == nil {
		return ""
	}
	interval := rule.Interval
	if interval <= 0 {
		interval = 1
	}
	var freqName string
	switch rule.Freq {
	case "daily":
		if interval == 1 {
			freqName = "每天"
		} else {
			freqName = fmt.Sprintf("每%d天", interval)
		}
	case "weekly":
		if interval == 1 {
			freqName = "每周"
		} else {
			freqName = fmt.Sprintf("每%d周", interval)
		}
		if len(rule.ByWeekday) > 0 {
			names := []string{}
			wdNames := []string{"日", "一", "二", "三", "四", "五", "六"}
			for _, wd := range rule.ByWeekday {
				if wd >= 0 && wd < 7 {
					names = append(names, wdNames[wd])
				}
			}
			if len(names) > 0 {
				freqName += "周" + joinStrings(names, "、")
			}
		}
	case "monthly":
		if interval == 1 {
			freqName = "每月"
		} else {
			freqName = fmt.Sprintf("每%d月", interval)
		}
	case "yearly":
		if interval == 1 {
			freqName = "每年"
		} else {
			freqName = fmt.Sprintf("每%d年", interval)
		}
	}
	return freqName
}

func joinStrings(ss []string, sep string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}
