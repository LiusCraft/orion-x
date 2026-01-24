package tools

import (
	"fmt"
	"io"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
)

// GetWeatherTool 获取天气工具
func GetWeatherTool(args map[string]interface{}) (interface{}, io.Reader, error) {
	city := args["city"].(string)

	logging.Infof("GetWeatherTool: querying weather for city: %s", city)

	// TODO: 实际调用天气API
	// 这里模拟天气数据
	weather := map[string]interface{}{
		"city":        city,
		"temperature": 25,
		"condition":   "晴天",
		"humidity":    60,
		"wind":        "东风3级",
	}

	logging.Infof("GetWeatherTool: weather result: %v", weather)
	return weather, nil, nil
}

// GetTimeTool 获取时间工具
func GetTimeTool(args map[string]interface{}) (interface{}, io.Reader, error) {
	logging.Infof("GetTimeTool: getting current time")

	now := map[string]interface{}{
		"current":   getCurrentTimeFormatted(),
		"year":      getCurrentYear(),
		"month":     getCurrentMonth(),
		"day":       getCurrentDay(),
		"hour":      getCurrentHour(),
		"minute":    getCurrentMinute(),
		"second":    getCurrentSecond(),
		"weekday":   getCurrentWeekday(),
		"timezone":  getTimezone(),
		"timestamp": getCurrentTimestamp(),
	}

	logging.Infof("GetTimeTool: time result: %v", now)
	return now, nil, nil
}

// SearchTool 搜索工具
func SearchTool(args map[string]interface{}) (interface{}, io.Reader, error) {
	query := args["query"].(string)

	// TODO: 实际调用搜索API
	// 这里模拟搜索结果
	results := map[string]interface{}{
		"query": query,
		"results": []string{
			"搜索结果1",
			"搜索结果2",
			"搜索结果3",
		},
	}

	return results, nil, nil
}

func getCurrentTimeFormatted() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func getCurrentYear() int {
	return time.Now().Year()
}

func getCurrentMonth() int {
	return int(time.Now().Month())
}

func getCurrentDay() int {
	return time.Now().Day()
}

func getCurrentHour() int {
	return time.Now().Hour()
}

func getCurrentMinute() int {
	return time.Now().Minute()
}

func getCurrentSecond() int {
	return time.Now().Second()
}

func getCurrentWeekday() string {
	weekdays := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	return weekdays[time.Now().Weekday()]
}

func getTimezone() string {
	_, offset := time.Now().Zone()
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	sign := "+"
	if offset < 0 {
		sign = "-"
		hours = -hours
		minutes = -minutes
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)
}

func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}
