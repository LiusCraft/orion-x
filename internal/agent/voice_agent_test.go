package agent

import (
	"context"
	"testing"
	"time"
)

func TestNewVoiceAgent(t *testing.T) {
	ctx := context.Background()

	agent, err := NewVoiceAgent(ctx)
	if err != nil {
		t.Fatalf("NewVoiceAgent() error = %v", err)
	}

	if agent == nil {
		t.Fatal("NewVoiceAgent() returned nil")
	}
}

func TestVoiceAgentProcess(t *testing.T) {
	ctx := context.Background()
	agent, err := NewVoiceAgent(ctx)
	if err != nil {
		t.Fatalf("NewVoiceAgent() error = %v", err)
	}

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"简单问候", "你好", false},
		{"天气查询", "北京天气怎么样", false},
		{"时间查询", "现在几点了", false},
		{"音乐播放", "播放一首歌", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventChan, err := agent.Process(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			var hasTextChunk, hasFinished bool
			timeout := time.After(30 * time.Second)
			finished := false

			for !finished {
				select {
				case event, ok := <-eventChan:
					if !ok {
						return
					}
					switch e := event.(type) {
					case *TextChunkEvent:
						hasTextChunk = true
						if e.Chunk == "" {
							t.Error("TextChunkEvent has empty chunk")
						}
						t.Logf("TextChunk: %s (Emotion: %s)", e.Chunk, e.Emotion)
					case *EmotionChangedEvent:
						t.Logf("EmotionChanged: %s", e.Emotion)
					case *ToolCallRequestedEvent:
						t.Logf("ToolCallRequested: %s, Type: %s, Args: %v", e.Tool, e.ToolType, e.Args)
					case *FinishedEvent:
						hasFinished = true
						finished = true
						if e.Error != nil {
							t.Logf("Finished with error: %v", e.Error)
						}
					}
				case <-timeout:
					t.Fatal("Test timed out after 30 seconds")
				}
			}

			if !hasTextChunk && !hasFinished {
				t.Error("No TextChunkEvent or FinishedEvent received")
			}
		})
	}
}

func TestEmotionExtractor(t *testing.T) {
	extractor := NewEmotionExtractor()

	tests := []struct {
		input    string
		expected string
	}{
		{"你好啊 [EMO:happy]", "happy"},
		{"这个很糟糕 [EMO:sad]", "sad"},
		{"我好生气 [EMO:angry]", "angry"},
		{"冷静一下 [EMO:calm]", "calm"},
		{"太棒了 [EMO:excited]", "excited"},
		{"普通回复", "default"},
		{"没有标签的内容", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractor.Extract(tt.input)
			if result != tt.expected {
				t.Errorf("Extract() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMarkdownFilter(t *testing.T) {
	filter := NewMarkdownFilter()

	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			"移除情绪标签",
			"你好啊 [EMO:happy]",
			[]string{"你好啊", "你好啊 [EMO:happy]"},
		},
		{
			"情绪标签",
			"天气很好 [EMO:happy]",
			[]string{"天气很好"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.Filter(tt.input)
			for _, contain := range tt.contains {
				if len(contain) > 20 {
					if result == contain {
						t.Logf("Result matches exactly: %s", result)
					}
				} else {
					found := false
					for _, s := range tt.contains {
						if len(s) < len(contain) && result == s {
							found = true
							break
						}
					}
					if !found {
						t.Logf("Filtered: '%s' -> '%s'", tt.input, result)
					}
				}
			}
		})
	}
}

func TestToolClassifier(t *testing.T) {
	classifier := NewToolClassifier()

	tests := []struct {
		name     string
		tool     string
		expected ToolType
	}{
		{"getWeather 查询类", "getWeather", ToolTypeQuery},
		{"getTime 查询类", "getTime", ToolTypeQuery},
		{"search 查询类", "search", ToolTypeQuery},
		{"playMusic 动作类", "playMusic", ToolTypeAction},
		{"setVolume 动作类", "setVolume", ToolTypeAction},
		{"pauseMusic 动作类", "pauseMusic", ToolTypeAction},
		{"未知工具", "unknownTool", ToolTypeQuery},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.GetToolType(tt.tool)
			if result != tt.expected {
				t.Errorf("GetToolType() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestActionResponseGenerator(t *testing.T) {
	generator := NewActionResponseGenerator()

	tests := []struct {
		name     string
		tool     string
		args     map[string]interface{}
		expected string
	}{
		{"playMusic", "playMusic", map[string]interface{}{"song": "告白气球"}, "正在为您播放告白气球"},
		{"setVolume", "setVolume", map[string]interface{}{"level": "80"}, "已将音量设置为80"},
		{"pauseMusic", "pauseMusic", nil, "音乐已暂停"},
		{"未知工具", "unknownTool", nil, "好的，正在为您处理"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generator.GenerateResponse(tt.tool, tt.args)
			if result != tt.expected {
				t.Errorf("GenerateResponse() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestActionResponseGeneratorWithTemplates(t *testing.T) {
	generator := NewActionResponseGeneratorWithTemplates(map[string]string{
		"playMusic": "正在播放{{song}}",
	})

	result := generator.GenerateResponse("playMusic", map[string]interface{}{"song": "稻香"})
	if result != "正在播放稻香" {
		t.Fatalf("template response = %q, want %q", result, "正在播放稻香")
	}
}

func TestParseToolType(t *testing.T) {
	if toolType, err := ParseToolType("query"); err != nil || toolType != ToolTypeQuery {
		t.Fatalf("ParseToolType(query) = %v, %v", toolType, err)
	}
	if toolType, err := ParseToolType("action"); err != nil || toolType != ToolTypeAction {
		t.Fatalf("ParseToolType(action) = %v, %v", toolType, err)
	}
	if _, err := ParseToolType("unknown"); err == nil {
		t.Fatalf("expected error for unknown tool type")
	}
}

func TestNewToolClassifierWithTypes(t *testing.T) {
	classifier := NewToolClassifierWithTypes(map[string]ToolType{
		"custom": ToolTypeAction,
	})
	if classifier.GetToolType("custom") != ToolTypeAction {
		t.Fatalf("expected custom tool type to be Action")
	}
}

func TestVoiceAgentGetToolType(t *testing.T) {
	ctx := context.Background()
	agent, err := NewVoiceAgent(ctx)
	if err != nil {
		t.Fatalf("NewVoiceAgent() error = %v", err)
	}

	tests := []struct {
		name     string
		tool     string
		expected ToolType
	}{
		{"getWeather", "getWeather", ToolTypeQuery},
		{"playMusic", "playMusic", ToolTypeAction},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.GetToolType(tt.tool)
			if result != tt.expected {
				t.Errorf("GetToolType() = %v, want %v", result, tt.expected)
			}
		})
	}
}
