# 语音机器人开发 TODO

## 阶段一：核心功能实现

### 0. 模块串联 (优先级: 高) ⭐️ 已完成
- [x] 创建主程序 `cmd/voicebot/main.go`
- [x] 集成 Orchestrator、VoiceAgent、AudioOutPipe、AudioInPipe
- [x] 集成 ToolExecutor 和工具
- [x] 验证各模块能够正确协作
- [x] 添加集成测试

### 0.1 日志库接入 (优先级: 高)
- [x] 引入 zap 日志库
- [x] 统一封装 `internal/logging`
- [x] 支持 `traceId-turnId` 日志标识
- [x] 替换全项目 `log.*` 调用

### 1. Orchestrator 实现 (优先级: 高) ⭐️ 优先
- [x] 搭建基础框架和事件总线
- [x] 实现状态机（Idle/Listening/Processing/Speaking）
- [x] 实现事件路由逻辑
- [x] 实现组件协调（启动/停止）
- [x] 实现中断处理逻辑
- [x] 集成各模块接口（先使用mock验证流程）
- [x] 集成 `text.Segmenter` 分句器和 TTS 生成

### 2. VoiceAgent 实现 (优先级: 高)
- [x] 实现 `VoiceAgent.Process()` 方法
- [x] 集成 `internal/ai/llm.go` 的工具调用逻辑
- [x] 启用流式LLM输出（当前被注释）
- [x] 实现工具识别和调用逻辑
- [x] 区分查询类工具和动作类工具的流程
- [x] 实现情绪标签注入到Prompt（优化为标签在句子开头）
- [x] 集成 `EmotionExtractor` 和 `MarkdownFilter`
- [x] 集成 `text.Segmenter` 分句器
- [x] 修复 LLM 流式增量输出，避免重复前缀

### 3. AudioMixer 实现 (优先级: 高)
- [x] 实现双通道音频混合逻辑
- [x] 实现动态音量控制（TTS播放时资源音频降为50%）
- [x] 实现音频流添加/移除
- [x] 集成音频播放器（PortAudio或其他）

### 4. AudioOutPipe 实现 (优先级: 高)
- [x] 实现 `PlayTTS()` 方法
- [x] 实现 `PlayResource()` 方法
- [x] 实现 `Interrupt()` 方法（立即停止播放）
- [x] 集成 `AudioMixer`
- [x] 集成 `internal/tts/dashscope.go` 的TTS服务
- [x] 实现音色切换逻辑（基于情绪）
- [x] 修复 TTS 播放阻塞/不出声问题

### 4.1 TTSPipeline 异步管道 (优先级: 高) ⭐
- [x] 设计并实现 TTSPipeline 接口
- [x] 实现文本队列（可配置大小）
- [x] 实现 TTS Worker Pool（可配置并发数）
- [x] 实现 TTS 音频缓冲区（可配置大小）
- [x] 实现 Audio Player 播放协程
- [x] 实现 Interrupt() 快速中断机制
- [x] 集成到 AudioOutPipe
- [x] 实现 Agent context 取消联动

### 4.2 多采样率支持 (优先级: 中)
- [x] 设计 Resampler 接口
- [x] 实现线性插值重采样算法
- [x] 实现 ResamplingReader 包装器
- [x] 扩展 MixerConfig 支持采样率配置
- [x] TTS Stream 元数据添加采样率信息
- [x] OutPipe 集成自动重采样

### 5. AudioInPipe 实现 (优先级: 高)
- [x] 实现完整的状态机逻辑
- [x] 实现事件路由
- [x] 集成 `EventBus`
- [x] 协调各组件启动/停止
- [x] 处理中断逻辑
- [x] 实现音频采集（从麦克风读取数据）
- [x] 设计 AudioSource 接口支持多种输入源
- [x] 修复 Ctrl+C 退出卡住问题
- [x] 实现 MicrophoneSource 麦克风音频源
- [x] 修复 AudioInPipe.Stop() 等待卡住问题
- [x] 修复 MicrophoneSource 关闭时阻塞问题
- [x] MicrophoneSource.Read 支持 context 取消并主动 Abort
- [x] ASR SendAudio 支持 context 取消，避免 Stop 卡住
- [x] 集成 VAD 检测（可选）
- [x] 修复 TTS DNS 查询被取消问题
- [x] 修复 Mixer.Start() 可能阻塞问题

## 阶段二：工具集成

### 6. 工具实现 (优先级: 中)
- [ ] 实现真实的天气查询工具（调用天气API）
- [ ] 实现真实的音乐播放工具（集成音乐服务）
- [x] 实现时间获取工具
- [ ] 实现搜索工具
- [ ] 添加更多实用工具

### 7. 工具注册 (优先级: 中)
- [ ] 实现工具自动发现机制
- [ ] 实现工具配置管理
- [ ] 工具类型配置化（查询类/动作类）

## 阶段三：功能完善

### 8. 音色管理 (优先级: 中)
- [ ] 实现情绪到音色的映射表
- [ ] 支持自定义音色配置
- [ ] 实现音色动态切换

### 9. MarkdownFilter 实现 (优先级: 中)
- [ ] 实现完整的正则过滤逻辑
- [ ] 支持可配置的过滤规则
- [ ] 测试各种Markdown格式

### 10. 配置管理 (优先级: 低)
- [x] 实现配置文件支持（JSON）
- [x] API密钥管理
- [x] 音频参数配置
- [x] 工具配置

## 阶段四：测试和优化

### 11. 单元测试 (优先级: 中)
- [ ] 测试 `Orchestrator` 状态转换
- [ ] 测试 `AudioMixer` 混音逻辑
- [ ] 测试 `VoiceAgent` 流程
- [ ] 测试 `MarkdownFilter` 过滤逻辑
- [ ] 测试工具调用流程

### 12. 集成测试 (优先级: 中)
- [ ] 端到端语音对话测试
- [ ] 工具调用测试
- [ ] 中断机制测试
- [ ] 音频混音测试

### 13. 性能优化 (优先级: 低)
- [ ] 优化流式处理延迟
- [ ] 优化音频混音性能
- [ ] 连接池管理（TTS/ASR）
- [ ] 内存优化

## 阶段五：扩展功能

### 14. 多轮对话 (优先级: 低)
- [ ] 实现对话历史管理
- [ ] 实现上下文记忆
- [ ] 实现会话管理

### 15. 音效处理 (优先级: 低)
- [ ] 实现音频淡入淡出
- [ ] 实现混响效果
- [ ] 实现降噪处理

### 16. 远程访问 (优先级: 低)
- [ ] 实现WebSocket服务
- [ ] 实现HTTP API
- [ ] 客户端SDK

## 优先级说明

- **高**: 核心功能，必须先实现
- **中**: 重要功能，需要实现
- **低**: 扩展功能，后续实现

## 建议实现顺序（自顶向下）

1. **Orchestrator** ⭐️ - 外壳框架，先搭建整体流程，使用mock验证
2. **VoiceAgent** - 逻辑层，实现LLM调用和工具识别
3. **AudioMixer** - 工具层，实现音频混音功能
4. **AudioOutPipe** - 硬件层，实现音频输出（依赖AudioMixer）
5. **AudioInPipe** - 硬件层，实现音频输入（基于现有ASR）

**为什么自顶向下？**
- 快速验证架构设计是否合理
- 每层都能独立运行和测试
- 上层接口明确后，下层按需实现
- 从用户视角出发，需求更清晰

## 技术要点

### 流式处理
- LLM流式输出 → 分句 → TTS流式生成 → 播放
- 保持低延迟，边生成边播放

### 工具调用流程
- 查询类: ASR → 识别工具 → 调用 → 获取数据 → LLM总结 → TTS
- 动作类: ASR → 识别工具 → 生成回复 → TTS → 调用工具 → 播放音频

### 混音逻辑
```
TTS播放中: 资源音频 = 50%音量
TTS停止时: 资源音频 = 100%音量
```

### 中断机制
```
用户说话 → VAD检测 → 发送Interrupt事件 → 停止播放 → 清空队列 → 重新识别
```

### 情绪标注
```
LLM Prompt要求格式: "回答内容 [EMO:emotion]"
提取情绪 → 查表映射音色 → 切换TTS音色
```

## 依赖模块

- `internal/ai/llm.go` - LLM工具调用框架
- `internal/asr/*` - ASR模块
- `internal/tts/*` - TTS模块
- `internal/text/segmenter.go` - 分句器
- `github.com/gordonklaus/portaudio` - 音频播放
