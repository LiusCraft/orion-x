# VoiceBot

语音机器人主程序，集成所有模块实现完整的语音对话功能。

## 环境变量

运行前需要设置以下环境变量：

```bash
export DASHSCOPE_API_KEY=your_api_key_here
# 或
export ZHIPU_API_KEY=your_api_key_here
```

## 构建

```bash
go build -o voicebot ./cmd/voicebot
```

## 运行

```bash
./voicebot
```

## 功能特性

- 语音识别 (ASR) - 实时将语音转换为文本
- 语音合成 (TTS) - 将文本转换为语音输出
- 对话管理 - 管理对话状态和流程
- 工具调用 - 支持获取时间、天气等工具
- 情绪标注 - 根据对话情绪调整音色
- 音频混音 - 支持 TTS 和背景音频混合
- 中断机制 - 检测用户说话时自动中断当前播放
- 多音频源支持 - 支持麦克风、WebSocket、文件等多种输入源

## 架构

```
麦克风 AudioSource
        ↓
AudioInPipe (ASR) → Orchestrator → VoiceAgent (LLM) → AudioOutPipe (TTS) → 扬声器
                                                                 ↓
                                                            ToolExecutor
```

## 状态机

- Idle: 空闲状态
- Listening: 监听中
- Processing: 处理中
- Speaking: 播放中

## 已实现的工具

- getTime: 获取当前时间
- getWeather: 获取天气信息

## 日志说明

程序运行时会输出详细的日志信息：

- 组件启动/停止
- 状态转换
- ASR 识别结果
- LLM 生成内容
- 工具调用和执行
- TTS 播放状态
- 情绪变化

## macOS 权限

首次运行时需要在"系统设置 → 隐私与安全性 → 麦克风"中授权 Terminal 或 VS Code。
