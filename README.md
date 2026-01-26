# Orion-X

> 智能语音机器人系统 - 基于 Go 的实时语音交互平台

Orion-X 是一个基于 Go 语言开发的智能语音机器人系统，采用管道式架构，集成了自动语音识别 (ASR)、文本转语音 (TTS)、大语言模型 (LLM) 和工具执行能力，实现低延迟的实时语音交互体验。

## 特性

- **实时语音交互** - 低延迟的语音输入输出，支持语音活动检测 (VAD)
- **声学回声消除 (AEC)** - 提升语音通话质量
- **智能打断** - 支持用户在 AI 播放时随时打断
- **情感语音合成** - 根据对话内容自动切换情感音色
- **工具调用** - 支持查询类和动作类工具扩展
- **音频混音** - 双通道音频混音，支持背景音乐播放
- **事件驱动架构** - 基于 EventBus 的松耦合设计

## 架构概览

```
User Speech → ASR → LLM Processing → TTS → Audio Output
              ↓        ↓              ↓
         Tool Call → Tools → Resource Audio → Audio Mixer
```

### 核心模块

| 模块 | 职责 |
|------|------|
| `Orchestrator` | 状态机管理、事件路由、组件协调 |
| `VoiceAgent` | LLM 调用、工具识别、情绪标注 |
| `AudioMixer` | 音频混合、音量控制 |
| `AudioOutPipe` | TTS/资源音频播放、队列管理 |
| `AudioInPipe` | 麦克风输入、ASR 集成 |
| `ASR` | 阿里云 Dashscope 语音识别 |
| `TTS` | 阿里云 CosyVoice 语音合成 |
| `ToolExecutor` | 工具执行、结果返回 |

### 状态机

```
Idle → Listening → Processing → Speaking → Idle
  ↑                      ↑
  └──────────────────────┘
```

## 快速开始

### 环境要求

- Go 1.24.4+
- PortAudio 库 (音频 I/O)

### 安装 PortAudio

**macOS:**
```bash
brew install portaudio
```

**Ubuntu/Debian:**
```bash
sudo apt-get install libportaudio2
```

**Windows:**
下载并安装 [PortAudio](http://www.portaudio.com/download.html)

### 克隆项目

```bash
git clone https://github.com/liuscraft/orion-x.git
cd orion-x
```

### 配置

复制示例配置文件并填入你的 API 密钥：

```bash
cp config/voicebot.example.json config/voicebot.json
```

编辑 `config/voicebot.json`，填入：
- 阿里云 Dashscope API Key (ASR/TTS)
- 智谱 AI API Key (LLM)

### 运行

```bash
go run cmd/voicebot/main.go
```

## 配置说明

```json
{
  "logging": {
    "level": "info",        // 日志级别: debug, info, warn, error
    "format": "console"     // 日志格式: console, json
  },
  "asr": {
    "api_key": "...",       // 阿里云 Dashscope API Key
    "model": "fun-asr-realtime",
    "endpoint": "wss://dashscope.aliyuncs.com/api-ws/v1/inference"
  },
  "tts": {
    "api_key": "...",       // 阿里云 Dashscope API Key
    "model": "cosyvoice-v3-flash",
    "voice": "longanyang",  // 默认音色
    "voice_map": {          // 情感音色映射
      "happy": "longanyang",
      "sad": "zhichu",
      "angry": "zhimeng",
      "calm": "longxiaochun"
    }
  },
  "llm": {
    "api_key": "...",       // 智谱 AI API Key
    "base_url": "https://open.bigmodel.cn/api/coding/paas/v4",
    "model": "glm-4-flash"
  },
  "tools": {
    "types": {              // 工具类型定义
      "getTime": "query",   // query: 需要返回给 LLM
      "playMusic": "action" // action: 直接执行并返回预设响应
    }
  }
}
```

## 工具开发

### 创建新工具

1. 在 `internal/tools/` 中实现工具逻辑
2. 在配置文件中注册工具类型和响应

```go
func NewMyTool() Tool {
    return &myTool{}
}
```

### 工具类型

- **query**: 工具执行结果返回给 LLM 处理
- **action**: 直接返回预设响应，不经过 LLM

## 测试

```bash
# 运行所有测试
go test ./...

# 运行特定模块测试
go test ./internal/voicebot/

# 查看测试覆盖率
go test -cover ./...
```

## 开发规范

本项目遵循 `AGENTS.md` 中定义的开发规范：

1. **自上而下的架构设计** - 优先定义接口，后实现细节
2. **单元测试强制覆盖** - 公共 API 必须有测试
3. **模块职责分明** - 单一职责原则
4. **设计文档优先** - 实现前先更新文档
5. **主动沟通确认** - 不确定时主动询问

## 项目结构

```
orion-x/
├── cmd/                    # 命令行入口
│   ├── voicebot/          # 主程序
│   ├── asr/               # ASR 独立测试
│   ├── tts/               # TTS 独立测试
│   └── mixer/             # 音频混音测试
├── internal/              # 内部包
│   ├── voicebot/          # Orchestrator 核心
│   ├── agent/             # VoiceAgent 实现
│   ├── audio/             # 音频处理管道
│   ├── asr/               # ASR 实现
│   ├── tts/               # TTS 实现
│   ├── tools/             # 工具执行
│   ├── text/              # 文本处理
│   └── logging/           # 日志工具
├── pkg/                   # 公共包
├── config/                # 配置文件
└── docs/                  # 设计文档
```

## 技术栈

| 组件 | 技术 |
|------|------|
| 编程语言 | Go 1.24.4 |
| AI 框架 | CloudWeGo Eino |
| 音频 I/O | PortAudio |
| WebSocket | Gorilla WebSocket |
| 日志 | Zap |
| ASR/TTS | 阿里云 Dashscope |
| LLM | 智谱 AI GLM-4 |

## 文档

- [AGENTS.md](./AGENTS.md) - AI 开发规范
- [docs/voicebot-architecture.md](./docs/voicebot-architecture.md) - 系统架构设计
- [docs/voicebot-modules.md](./docs/voicebot-modules.md) - 模块详细设计
- [docs/voicebot-todo.md](./docs/voicebot-todo.md) - 开发任务列表

## 许可证

MIT License
