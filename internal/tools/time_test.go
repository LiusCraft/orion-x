package tools

import (
	"testing"
	"time"
)

func TestGetTimeTool(t *testing.T) {
	result, audio, err := GetTimeTool(nil)

	if err != nil {
		t.Fatalf("GetTimeTool returned error: %v", err)
	}

	if audio != nil {
		t.Error("GetTimeTool should return nil audio")
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("GetTimeTool result is not a map")
	}

	requiredKeys := []string{
		"current", "year", "month", "day", "hour", "minute", "second",
		"weekday", "timezone", "timestamp",
	}

	for _, key := range requiredKeys {
		if _, exists := data[key]; !exists {
			t.Errorf("GetTimeTool result missing key: %s", key)
		}
	}

	current, ok := data["current"].(string)
	if !ok {
		t.Error("current is not a string")
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	if current != now {
		t.Errorf("current time mismatch: got %s, want %s", current, now)
	}

	year, ok := data["year"].(int)
	if !ok {
		t.Error("year is not an int")
	}
	if year != time.Now().Year() {
		t.Errorf("year mismatch: got %d, want %d", year, time.Now().Year())
	}
}

func TestGetCurrentTimeFormatted(t *testing.T) {
	formatted := getCurrentTimeFormatted()

	now := time.Now().Format("2006-01-02 15:04:05")
	if formatted != now {
		t.Errorf("getCurrentTimeFormatted returned %s, want %s", formatted, now)
	}
}

func TestGetCurrentYear(t *testing.T) {
	year := getCurrentYear()
	if year != time.Now().Year() {
		t.Errorf("getCurrentYear returned %d, want %d", year, time.Now().Year())
	}
}

func TestGetCurrentMonth(t *testing.T) {
	month := getCurrentMonth()
	if month < 1 || month > 12 {
		t.Errorf("getCurrentMonth returned invalid value: %d", month)
	}
	if month != int(time.Now().Month()) {
		t.Errorf("getCurrentMonth returned %d, want %d", month, time.Now().Month())
	}
}

func TestGetCurrentDay(t *testing.T) {
	day := getCurrentDay()
	if day < 1 || day > 31 {
		t.Errorf("getCurrentDay returned invalid value: %d", day)
	}
	if day != time.Now().Day() {
		t.Errorf("getCurrentDay returned %d, want %d", day, time.Now().Day())
	}
}

func TestGetCurrentHour(t *testing.T) {
	hour := getCurrentHour()
	if hour < 0 || hour > 23 {
		t.Errorf("getCurrentHour returned invalid value: %d", hour)
	}
}

func TestGetCurrentMinute(t *testing.T) {
	minute := getCurrentMinute()
	if minute < 0 || minute > 59 {
		t.Errorf("getCurrentMinute returned invalid value: %d", minute)
	}
}

func TestGetCurrentSecond(t *testing.T) {
	second := getCurrentSecond()
	if second < 0 || second > 59 {
		t.Errorf("getCurrentSecond returned invalid value: %d", second)
	}
}

func TestGetCurrentWeekday(t *testing.T) {
	weekday := getCurrentWeekday()
	expected := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}

	valid := false
	for _, e := range expected {
		if weekday == e {
			valid = true
			break
		}
	}

	if !valid {
		t.Errorf("getCurrentWeekday returned invalid value: %s", weekday)
	}

	if weekday != expected[time.Now().Weekday()] {
		t.Errorf("getCurrentWeekday returned %s, want %s", weekday, expected[time.Now().Weekday()])
	}
}

func TestGetTimezone(t *testing.T) {
	timezone := getTimezone()
	if len(timezone) == 0 {
		t.Error("GetTimezone returned empty string")
	}
	if timezone[0] != 'U' || timezone[1] != 'T' || timezone[2] != 'C' {
		t.Errorf("GetTimezone returned invalid format: %s", timezone)
	}
}

func TestGetCurrentTimestamp(t *testing.T) {
	timestamp := getCurrentTimestamp()

	now := time.Now().Unix()

	if timestamp > now+2 || timestamp < now-2 {
		t.Errorf("GetCurrentTimestamp returned %d, want around %d", timestamp, now)
	}
}
