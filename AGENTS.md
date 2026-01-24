# AGENTS.md - AI 开发规范

## 开发原则

### 1. 自上而下的架构设计
- 优先从用户视角和业务需求出发，先设计高层接口和流程
- 使用 Mock 或接口定义验证架构可行性
- 确定接口后，再逐步实现底层细节
- 避免过早陷入实现细节，先保证整体流程可运行

**示例**：
```go
// 先定义接口
type Orchestrator interface {
    Start(ctx context.Context) error
    OnASRFinal(text string)
}

// 再实现具体逻辑
type orchestratorImpl struct { ... }
```

### 2. 单元测试强制覆盖
- 所有公共 API 必须有单元测试
- 边界条件、错误处理必须有测试覆盖
- 复杂逻辑需要集成测试
- 测试失败时，优先修复代码而非忽略测试

**测试要求**：
```go
func TestOrchestrator(t *testing.T) {
    tests := []struct {
        name     string
        input    Input
        expected Expected
    }{ ... }

    for _, tt := range tests { ... }
}
```

### 3. 模块职责分明
- 每个模块只负责单一职责
- 避免功能混乱，如：Orchestrator 不应包含音频编解码逻辑
- 接口设计遵循最小暴露原则
- 依赖关系清晰，避免循环依赖

**模块职责示例**：
- `Orchestrator`: 状态管理、事件路由、组件协调
- `VoiceAgent`: LLM 调用、工具识别、情绪标注
- `AudioMixer`: 音频混合、音量控制
- `ToolExecutor`: 工具执行、结果返回

### 4. 设计文档优先
- 实现前必须先更新或创建对应的设计文档
- 模块内新增功能需先找到该模块设计文档，调整后再编码
- 文档应包含：接口定义、流程图、状态机、依赖关系
- 代码实现后需同步更新文档

**文档检查清单**：
- [ ] 接口定义是否完整
- [ ] 状态转换是否清晰
- [ ] 数据流是否正确
- [ ] 错误处理是否考虑

### 5. TODO 同步更新
- 完成 TODO 任务后，必须同步更新 `docs/voicebot-todo.md` 文档
- 将已完成的任务标记为 `[x]`
- 更新进度状态，确保文档与实际开发进度一致
- 遇到 TODO 中未记录的新任务时，先补充到文档再实现

### 6. 主动沟通确认
- 遇到不确定的需求或设计时，必须主动询问
- 不要自行假设或猜测
- 对设计有不同意见时，可以提出讨论
- 明确任务范围，避免过度设计

**提问示例**：
- "关于工具调用的流程，我建议先实现查询类工具，您认为？"
- "AudioMixer 需要支持淡入淡出效果吗？当前文档未提及。"

### 7. 技术决策与沟通
- 基于项目实际情况做出技术决策
- 认为方案更优时，可以提出建议并说明理由
- 保持开放心态，接受合理的反馈
- 重要技术决策需记录到文档

## 代码规范

### 文件结构
```go
package voicebot

// 1. 常量定义
const (
    StateIdle State = iota
    StateListening
)

// 2. 类型定义
type Orchestrator interface { ... }

// 3. 结构体
type orchestratorImpl struct { ... }

// 4. 构造函数
func NewOrchestrator(...) Orchestrator { ... }

// 5. 公共方法
func (o *orchestratorImpl) Start(...) error { ... }

// 6. 私有方法
func (o *orchestratorImpl) handleEvent(...) { ... }
```

### 错误处理
- 公共 API 必须返回 error
- 使用 `errors.New()` 或 `fmt.Errorf()` 创建错误
- 不要忽略错误，必须处理
- 上下文取消错误检查：`if errors.Is(err, context.Canceled)`

### 并发安全
- 共享状态使用 `sync.Mutex` 保护
- 读写操作区分 `RLock()` / `Lock()`
- 避免 defer + 锁的性能问题
- Context 传递贯穿调用链

### 日志规范
```go
import "log"

log.Printf("State changed: %s -> %s", oldState, newState)
log.Printf("Tool execution error: %v", err)
```

## 开发流程

### 新功能开发
1. **设计阶段**
   - 阅读相关模块设计文档
   - 确认需求和边界条件
   - 设计接口和数据流
   - 更新设计文档

2. **实现阶段**
   - 定义接口（自顶向下）
   - 编写单元测试（TDD 推荐）
   - 实现核心逻辑
   - 运行测试确保通过

3. **验证阶段**
   - 运行所有测试：`go test ./...`
   - 代码审查：`go vet ./...`
   - 文档更新

### Bug 修复
1. 复现问题，定位根因
2. 编写测试用例验证问题
3. 修复代码
4. 确保测试通过
5. 检查是否影响其他模块

## 项目架构概览

### 模块依赖关系
```
voicebot (Orchestrator)
    ├── agent (VoiceAgent)
    ├── audio (AudioOutPipe/AudioInPipe/AudioMixer)
    ├── tools (ToolExecutor)
    ├── asr (ASR - 已有)
    └── tts (TTS - 已有)
```

### 状态机
```
Idle → Listening → Processing → Speaking → Idle
  ↑                      ↑
  └──────────────────────┘
```

### 事件流
```
ASRFinal → Orchestrator → VoiceAgent → LLM/Tool → AudioOutPipe
UserSpeakingDetected → Orchestrator → AudioOutPipe.Interrupt()
```

## 工具使用

### 测试
```bash
# 运行所有测试
go test ./...

# 运行特定包
go test ./internal/voicebot/

# 测试覆盖率
go test -cover ./...

# 详细输出
go test -v ./internal/voicebot/
```

### 构建
```bash
# 构建所有包
go build ./...

# 格式化代码
go fmt ./...

# 检查代码
go vet ./...
```

### 依赖管理
```bash
# 整理依赖
go mod tidy

# 更新依赖
go get -u ./...
```

## 常见问题

### Q: 如何确定某个功能应该放在哪个模块？
A: 查看现有模块的职责定义，选择最匹配的。如果不确定，询问或查看设计文档。

### Q: 测试覆盖率目标是多少？
A: 核心逻辑必须 100% 覆盖，公共 API 建议 >80%。

### Q: 如何处理依赖缺失的情况？
A: 优先使用接口定义，然后提供 Mock 实现。避免阻塞其他模块开发。

### Q: 设计文档在哪里？
A: 在 `docs/` 目录下，模块通常有对应的设计文档。

## 质量标准

- ✅ 所有公共 API 有单元测试
- ✅ 代码通过 `go vet` 检查
- ✅ 无明显的性能问题（避免 O(n^2)、内存泄漏）
- ✅ 错误处理完善
- ✅ 文档更新及时
- ✅ 遵循 Go 最佳实践

## 提醒

- 遵循自上而下的设计原则
- 主动沟通不确定的内容
- 保持代码简洁清晰
- 先写文档再写代码
- 测试驱动开发
