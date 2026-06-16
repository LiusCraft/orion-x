# Orion-X

> 智能语音机器人系统 - 基于 Go 的实时语音交互平台

基于管道式架构，集成 ASR、TTS、LLM 和工具执行能力，实现低延迟实时语音交互。

## 快速开始

### 依赖

- Go 1.26+
- PortAudio（音频 I/O）
- ONNX Runtime（macOS：`brew install onnxruntime`）

```bash
# 安装 PortAudio
brew install portaudio        # macOS
sudo apt install libportaudio2 # Linux

# 配置
cp voicebot.example.json data/voicebot.json
# 编辑 data/voicebot.json，填入 API 密钥

# 运行
make run-voicebot
```

## 架构

```
麦克风 → AudioInPipe (ASR) → Pipeline → Agent (LLM) → AudioOutPipe (TTS) → 扬声器
                                 ↓
                            ToolManager + Memory
```

Pipeline: `ASR Stage → Agent Stage → TTS Stage`，通过 `pipeline.NewBuilder()` 构建。

## 核心模块

| 模块 | 职责 |
|------|------|
| `cmd/voicebot/` | 主程序入口，组装所有模块（测试用途，后续扩展为 WebSocket/GUI） |
| `internal/pipeline/` | 流式 Pipeline 框架（Stage 接口、Builder、Message 总线） |
| `internal/agent/` | LLM Agent，工具调用循环 |
| `internal/audio/` | AudioInPipe（麦克风→ASR）、AudioOutPipe（TTS→扬声器）、VAD、重采样 |
| `internal/provider/` | ASR/TTS Provider 工厂 + 阿里云 Dashscope 实现 |
| `internal/llm/` | LLM 类型 + OpenAI 兼容 Provider |
| `internal/tools/` | 工具管理器 + MCP 客户端 + 本地工具注册 |
| `internal/memory/` | 会话缓冲 + SQLite 长期记忆 |
| `internal/session/` | 对话会话 / 消息追踪 |
| `internal/config/` | JSON 配置加载与校验 |
| `internal/logging/` | Zap 封装日志（带 trace_id/turn_id） |
| `internal/text/` | 文本分段、Markdown 过滤、情感标签 |

## 配置

参见 `voicebot.example.json`：

```json
{
  "provider": {
    "asr": { "type": "aliyun", "aliyun": { "api_key": "...", "model": "fun-asr-realtime" } },
    "tts": { "type": "aliyun", "aliyun": { "api_key": "...", "model": "cosyvoice-v3-flash", "voice": "longanyang" } },
    "llm": { "type": "openai", "openai": { "api_key": "...", "base_url": "...", "model": "glm-4-flash" } }
  },
  "audio": {
    "in_pipe": { "sample_rate": 16000, "enable_vad": true, "vad_type": "silero" }
  },
  "memory": { "mode": "session", "session_max_turns": 10 }
}
```

## 开发

```bash
make build       # 构建
make test        # go test ./...
make test-audio  # go test ./internal/audio（需要音频设备 + API 密钥）
go vet ./...     # 代码检查
```

开发规范见 `AGENTS.md`。

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.26 |
| 音频 I/O | PortAudio + Opus 编解码 |
| VAD | Silero VAD (ONNX Runtime) |
| WebSocket | Gorilla WebSocket |
| 日志 | Zap |
| 存储 | SQLite (modernc.org/sqlite) |
| ASR/TTS | 阿里云 Dashscope |
| LLM | 智谱 AI GLM-4 (OpenAI 兼容) |

## 许可证

MIT
