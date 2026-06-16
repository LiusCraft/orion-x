---
layout: home

hero:
  name: Orion-X
  text: 实时语音 Agent 实验工程
  tagline: 当前版本以 CLI VoiceBot 为入口，验证 ASR -> Agent -> TTS 的端到端语音交互链路。
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/getting-started
    - theme: alt
      text: 查看架构
      link: /architecture/overview

features:
  - icon: ⚡
    title: 线性 Pipeline
    details: 通过 pipeline.NewBuilder() 组装 ASR、Agent、TTS Stage，消息在统一 Message 总线上流转。
  - icon: 🎙️
    title: 实时音频输入
    details: PortAudio 麦克风输入接入 AudioInPipe，支持 Silero VAD 和阿里云 DashScope 实时 ASR。
  - icon: 🧠
    title: LLM Agent
    details: 使用 OpenAI-compatible provider，支持流式输出和工具调用循环。
  - icon: 🔧
    title: 工具扩展
    details: 内置 getTime 工具，并可通过 MCP stdio、SSE、streamable transport 加载外部工具。
  - icon: 💾
    title: 记忆与会话
    details: Session 负责对话轮次，Memory Service 支持 none、session、long_term 三种模式。
  - icon: 🔊
    title: 异步 TTS 输出
    details: AudioOutPipe 管理 TTS 流和播放 sink，异步 TTS Pipeline 降低句间等待。
---
