# 工具开发

## 工具类型

Orion-X 支持两种工具类型，决定了工具执行后的处理流程：

| 类型 | 说明 | 适用场景 |
|------|------|----------|
| query | 工具执行结果返回给 LLM 处理 | 查询天气、搜索、获取时间等 |
| action | 直接返回预设响应，不经过 LLM | 播放音乐、设置音量、暂停音乐等 |

## 工具开发流程

### 1. 实现工具函数

在 `internal/tools/` 中创建工具文件：

```go
package tools

import (
    "context"
    "fmt"
)

// MyToolFunc 自定义工具函数
func MyToolFunc(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    // 获取参数
    param, ok := args["param"].(string)
    if !ok {
        return nil, fmt.Errorf("missing param")
    }

    // 执行工具逻辑
    result := fmt.Sprintf("处理结果: %s", param)

    // query 类型返回数据，action 类型返回 nil
    return result, nil
}
```

### 2. 注册工具

在 `cmd/voicebot/main.go` 或工具初始化代码中注册：

```go
executor := tools.NewExecutor()

// 注册 query 类型工具
executor.RegisterTool("myTool", tools.ToolTypeQuery, myToolFunc)

// 注册 action 类型工具
executor.RegisterTool("playMusic", tools.ToolTypeAction, playMusicFunc)
```

### 3. 配置文件注册

在 `config/voicebot.json` 中添加工具类型：

```json
{
  "tools": {
    "types": {
      "myTool": "query",
      "playMusic": "action"
    },
    "action_responses": {
      "playMusic": "正在为您播放{{song}}"
    }
  }
}
```

## 工具示例

### Query 类型：获取天气

```go
func GetWeatherTool(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    city, ok := args["city"].(string)
    if !ok {
        return nil, fmt.Errorf("missing city parameter")
    }

    // 调用天气 API
    weather := callWeatherAPI(city)

    // 返回结构化数据，LLM 会将其转换为自然语言
    return map[string]interface{}{
        "city":    city,
        "weather": weather.Condition,
        "temp":    weather.Temperature,
    }, nil
}
```

### Action 类型：播放音乐

```go
func PlayMusicTool(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    song, ok := args["song"].(string)
    if !ok {
        return nil, fmt.Errorf("missing song parameter")
    }

    // 执行播放逻辑
    audioStream := playMusic(song)

    // action 类型返回音频流（可选）
    return nil, audioStream
}
```

配置文件中的响应模板：

```json
{
  "tools": {
    "action_responses": {
      "playMusic": "正在为您播放{{song}}"
    }
  }
}
```

## 工具参数规范

### 参数获取

```go
// 字符串参数
name, _ := args["name"].(string)

// 数字参数
count, _ := args["count"].(float64)

// 布尔参数
enabled, _ := args["enabled"].(bool)
```

### 错误处理

```go
if !ok {
    return nil, fmt.Errorf("invalid parameter: %s", key)
}
```

## LLM 工具调用格式

LLM 需要按照以下格式调用工具：

```
<tool_call>
{"name": "getWeather", "arguments": {"city": "北京"}}
</tool_call>
```

系统会自动：
1. 解析工具名称和参数
2. 调用对应的工具函数
3. query 类型：将结果返回给 LLM 继续处理
4. action 类型：直接播放预设响应

## 测试工具

```go
func TestMyTool(t *testing.T) {
    ctx := context.Background()
    args := map[string]interface{}{
        "param": "test",
    }

    result, err := MyToolFunc(ctx, args)
    assert.NoError(t, err)
    assert.NotNil(t, result)
}
```

## 开发规范

本项目遵循 `AGENTS.md` 中定义的开发规范：

1. **自上而下的架构设计** - 优先定义接口，后实现细节
2. **单元测试强制覆盖** - 公共 API 必须有测试
3. **模块职责分明** - 单一职责原则
4. **设计文档优先** - 实现前先更新文档
5. **主动沟通确认** - 不确定时主动询问

## 相关文档

- [配置管理](/guide/configuration) - 工具配置说明
- [系统架构](/architecture/overview) - 架构设计
