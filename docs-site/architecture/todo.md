# 当前状态与路线图

本文记录当前代码状态和后续方向，不再保留已删除架构的历史 TODO。

## 当前已具备

- [x] CLI VoiceBot 入口：`cmd/voicebot`
- [x] 线性 Pipeline：`ASRStage -> AgentStage -> TTSStage`
- [x] PortAudio 麦克风输入与播放 sink
- [x] 阿里云 DashScope ASR/TTS provider
- [x] OpenAI-compatible LLM provider
- [x] Agent 流式输出和 tool calling loop
- [x] 本地 `getTime` 工具
- [x] MCP 工具加载：stdio、SSE、streamable
- [x] Session message 记录：user、assistant、tool
- [x] Memory Service：none、session、long_term
- [x] Silero VAD 与音频切段
- [x] 异步 TTS Pipeline 与中断
- [x] JSON 配置加载、环境变量覆盖和校验
- [x] zap logging 封装

## 当前边界

- `cmd/voicebot` 是本地测试 harness，还不是多用户服务
- 用户和会话标识目前固定为 `local`
- 工具结果当前以文本形式回写给 LLM，不直接接入资源音频播放链路
- ASR/TTS provider 当前只支持 DashScope
- LLM provider 当前只支持 OpenAI-compatible 接口
- `audio.mixer` 配置仍保留，但当前主链路没有独立全局 AudioMixer 模块
- Go 没有 CI，需本地运行 `make test` 和 `go vet ./...`

## 近期优先级

1. 补齐 Agent、tools、pipeline stage 的集成测试
2. 梳理 `voicebot.example.json` 与 `internal/config` 的字段一致性
3. 完善 MCP 工具错误恢复和超时日志
4. 为 WebSocket 或 GUI 入口抽象真实 user/session context
5. 将 memory `RecordTurn` 接入完整对话生命周期

## 可扩展方向

### 入口扩展

- WebSocket 音频输入输出
- GUI 客户端
- HTTP API 或服务端多用户会话

### Provider 扩展

- 新增 ASR provider
- 新增 TTS provider
- 新增 LLM provider
- 为 provider 增加可观测指标

### 工具扩展

- 增加本地工具包
- 支持更多 MCP transport 参数，例如 headers、env、cwd
- 增强工具并发、安全策略和权限控制

### 记忆扩展

- 按真实用户隔离长期记忆
- 完善记忆抽取 prompt 和重要性评分
- 增加记忆管理接口

### 音频体验

- 更稳定的设备选择
- 蓝牙设备延迟优化
- 播放状态可观测性
- 资源音频播放链路再设计
