# Agent 重构：工具调用内部化

## 重构目标

将工具调用从 Pipeline 的外部流程变为 Agent 的内部能力，简化架构。

**同时移除音频处理逻辑**：播放音频不是 Voicebot 的核心功能，工具提示音等场景暂不支持，未来可通过专门的播放器工具实现。

## 重构内容

### 1. Agent 接口变更

#### 之前
```go
type Agent struct {
    chatModel   llmfactory.ChatModel
    toolManager tools.ToolManager
    memorySvc   memory.Service
}

// 两个公共方法
func (a *Agent) Process(ctx, text) (<-chan AgentEvent, error)
func (a *Agent) SummarizeToolResult(ctx, tool, args, result) (<-chan AgentEvent, error)

// AgentEvent 包含 3 种类型
- TextChunkEvent
- ToolCallRequestedEvent  // 暴露工具调用
- FinishedEvent
```

#### 之后
```go
type Agent struct {
    chatModel    llmfactory.ChatModel
    toolManager  tools.ToolManager
    toolExecutor tools.ToolExecutor  // 新增
    memorySvc    memory.Service
}

// 只有一个公共方法
func (a *Agent) Process(ctx, text) (<-chan AgentEvent, error)

// 私有方法（内部使用）
func (a *Agent) summarizeToolResult(ctx, eventChan, tool, args, result) error

// AgentEvent 只有 2 种类型
- TextChunkEvent
- FinishedEvent

// ToolExecutor 接口简化
type ToolExecutor interface {
    Execute(tool, args) (result, error)  // 移除 audio 返回值
}
```

### 2. Process 方法行为变更

#### 之前
```go
Agent.Process() {
    for llmMsg := range llmStream {
        if llmMsg.Content != "" {
            emit TextChunkEvent
        }
        for toolCall := range llmMsg.ToolCalls {
            emit ToolCallRequestedEvent  // 让外部处理
        }
    }
}
```

#### 之后
```go
Agent.Process() {
    for llmMsg := range llmStream {
        if llmMsg.Content != "" {
            emit TextChunkEvent
        }
        for toolCall := range llmMsg.ToolCalls {
            // 内部执行工具
            result, err := toolExecutor.Execute(toolCall)
            if err != nil {
                emit FinishedEvent{Error: err}
                return
            }
            
            // 内部总结结果
            summarizeToolResult(tool, args, result)
            // 发出总结的 TextChunkEvent
        }
    }
}
```

### 3. Pipeline 简化

#### 之前（3 个 Stage）
```go
pipeline.NewBuilder().
    AddStage(stages.NewAgentStage(agent)).          // 识别工具
    AddStage(stages.NewToolExecutorStage(executor)). // 执行工具
    AddStage(stages.NewAgentStage(agent)).          // 总结结果
    Build()
```

#### 之后（1 个 Stage）
```go
pipeline.NewBuilder().
    AddStage(stages.NewAgentStage(agent)).  // Agent 内部处理所有工具逻辑
    Build()
```

### 4. 删除的代码

- ❌ `ToolCallRequestedEvent` 事件类型
- ❌ `AudioResourceEvent` 事件类型
- ❌ `SummarizeToolResult` 公共方法
- ❌ `ToolExecutorStage` 整个 Stage
- ❌ `AgentRunner.SummarizeToolResult` 接口方法
- ❌ `ToolExecutor.Execute` 的 `audio` 返回值

### 5. 新增的代码

- ✅ `Agent.toolExecutor` 字段
- ✅ `Agent.summarizeToolResult` 私有方法
- ✅ Agent 构造函数接受 `ToolExecutor` 参数

### 6. 简化的接口

- ✅ `ToolExecutor.Execute(tool, args) (result, error)` - 移除 audio 返回值
- ✅ `ToolExecutorFunc func(args) (interface{}, error)` - 移除 audio 返回值
- ✅ AgentEvent 只保留 2 种类型：TextChunk, Finished

## 架构对比

### 之前：工具调用是 Pipeline 的一部分

```
用户输入
  ↓
AgentStage: 识别需要调用工具
  ↓
ToolCallRequestedEvent (暴露到 Pipeline)
  ↓
ToolExecutorStage: 执行工具
  ↓
ToolResult (暴露到 Pipeline)
  ↓
AgentStage: 总结工具结果
  ↓
输出
```

**问题**：
- ❌ 暴露了 Agent 的内部实现细节
- ❌ Pipeline 需要了解工具调用流程
- ❌ 新增工具类型需要修改 Pipeline
- ❌ Agent 不完整（不能独立工作）

### 之后：工具调用是 Agent 的内部能力

```
用户输入
  ↓
AgentStage (Agent 内部):
  - LLM 识别工具调用
  - 执行工具
  - 总结结果
  ↓
输出 (文本 + 音频)
```

**优点**：
- ✅ Agent 是完整的智能体
- ✅ Pipeline 只关心数据流，不关心内部实现
- ✅ 符合 "Agent 会使用工具" 的直觉
- ✅ 扩展工具只需改 Agent/ToolManager

## 影响范围

### 需要更新的代码

1. **Agent 构造调用** - 所有创建 Agent 的地方需要传入 `ToolExecutor`
   ```go
   // 之前
   agent, err := NewAgent(ctx, cfg, toolManager, memorySvc)
   
   // 之后
   toolExecutor := tools.NewExecutorAdapter(ctx, toolManager)
   agent, err := NewAgent(ctx, cfg, toolManager, toolExecutor, memorySvc)
   ```

2. **Orchestrator** - 不再需要 ToolExecutorStage
   ```go
   // 移除 handleToolCallRequested
   // 移除对 ToolExecutor 的依赖
   ```

3. **Pipeline 组装** - 简化为单个 AgentStage
   ```go
   // 移除 ToolExecutorStage
   ```

4. **测试代码** - 更新 mock 和测试用例
   - 移除 ToolCallRequestedEvent 测试
   - 更新 Agent 构造调用

## 迁移指南

### 步骤 1：更新 Agent 创建
```go
toolExecutor := tools.NewExecutorAdapter(ctx, toolManager)
agent, err := agent.NewAgent(ctx, cfg, toolManager, toolExecutor, memorySvc)
```

### 步骤 2：更新 Pipeline
```go
// 删除 ToolExecutorStage
pipeline.NewBuilder().
    AddStage(stages.NewAgentStage(agent)).
    Build()
```

### 步骤 3：更新事件处理
```go
// 之前
case *agent.ToolCallRequestedEvent:
    // 处理工具调用

// 之后（删除，Agent 内部处理）
case *agent.AudioResourceEvent:
    // 处理工具返回的音频资源
```

## 测试验证

```bash
# 运行 Agent 测试
go test ./internal/agent/

# 运行 Pipeline 测试
go test ./internal/pipeline/...

```

## 收益

- **代码量减少**：删除 ~130 行（ToolExecutorStage）
- **Pipeline 简化**：从 3 个 Stage 减少到 1 个
- **架构更清晰**：Agent 是完整的智能体，不暴露内部实现
- **更易扩展**：新增工具类型无需改 Pipeline
- **符合直觉**："Agent 会使用工具" 而不是 "Pipeline 编排工具调用"

## 总结

这次重构将工具调用从 Pipeline 的显式流程变为 Agent 的隐式能力，使架构更加清晰和符合直觉。Agent 现在是一个完整的智能体，可以独立处理用户输入、调用工具并返回结果。
