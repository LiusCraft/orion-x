# VoiceBot

语音机器人测试程序，用于快速验证语音 Agent Pipeline 能力。

> 这是开发调试用途的 CLI 入口，实际产品可能通过 WebSocket、GUI 等渠道交互。

## 配置

复制 `voicebot.example.json` 到 `data/voicebot.json`，填入 API 密钥后直接运行。

```bash
make run-voicebot
```

## 构建

```bash
make build
# 或手动：
# GOTOOLCHAIN=$(go env GOTOOLCHAIN) CGO_CFLAGS="-I$(brew --prefix)/include/onnxruntime" CGO_LDFLAGS="-L$(brew --prefix)/lib" go build -o bin/voicebot ./cmd/voicebot
```

## 管线

```
AudioSource (麦克风) → AudioInPipe (VAD→ASR) → Pipeline → Agent (LLM+Tool) → AudioOutPipe (TTS) → AudioSink (扬声器)
```

Pipeline 阶段：`ASR → Agent → TTS`

## 本地工具

- `getTime`: 获取当前时间

（更多工具通过 MCP 协议扩展）

## macOS 权限

首次运行时需在"系统设置 → 隐私与安全性 → 麦克风"中授权终端。
