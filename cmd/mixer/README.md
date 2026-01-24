# AudioMixer 验证工具

## 概述

`cmd/mixer` 是一个用于验证 AudioMixer 功能的命令行工具，可以测试双通道音频混音和动态音量控制。

## 功能

- 双通道音频混音（TTS + Resource）
- 动态音量控制（TTS 播放时 Resource 音量自动降为 50%）
- 支持使用 WAV 音频文件或自动生成正弦波测试音频
- 音频循环播放

## 使用方法

### 基本用法（使用默认测试音频）

```bash
go run ./cmd/mixer
```

### 使用自定义音频文件

```bash
go run ./cmd/mixer -tts=voice.wav -resource=music.wav
```

### 调整测试音频时长

```bash
go run ./cmd/mixer -duration=5.0
```

### 组合使用

```bash
go run ./cmd/mixer -tts=voice.wav -duration=3.0
```

## 命令行选项

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `-tts` | TTS 音频文件路径（WAV 格式） | 440Hz 正弦波 |
| `-resource` | Resource 音频文件路径（WAV 格式） | 880Hz 正弦波 |
| `-duration` | 生成音频的持续时间（秒） | 2.0 |
| `-h` | 显示帮助信息 | - |

## 测试流程

工具会按以下阶段演示功能：

1. **阶段 1**：正常播放两个通道的音频（无音量调节）
2. **阶段 2**：调用 `OnTTSStarted()`，Resource 音量降为 50%
3. **阶段 3**：调用 `OnTTSFinished()`，Resource 音量恢复 100%

每个阶段的时长由 `-duration` 参数控制。

## 预期听觉效果

- **阶段 1**：两个音频源同时播放，音量正常
- **阶段 2**：Resource 音频音量明显降低（约 -6dB）
- **阶段 3**：Resource 音频音量恢复原大小

## 生成测试音频

如果需要生成自定义的 WAV 测试文件，可以使用 `make_test_audio.go`：

```bash
# 生成 440Hz 正弦波，时长 2 秒
go run ./cmd/make_test_audio.go test.wav 440 2

# 生成 880Hz 正弦波，时长 5 秒
go run ./cmd/make_test_audio.go test.wav 880 5
```

## 注意事项

- WAV 文件必须是 PCM 16-bit 格式，采样率 24000Hz
- 工具会跳过 WAV 文件头（前 44 字节），直接读取 PCM 数据
- 确保 Mac 系统的音频输出设备正常工作

## 技术细节

### 音频格式

- **采样率**：16000 Hz
- **声道数**：2（立体声，自动转换单声道）
- **位深**：16-bit
- **格式**：PCM Little-Endian

### 音频文件支持

工具支持读取 WAV 文件，会自动解析文件头并处理：
- **采样率**：使用系统采样率（16000 Hz）
- **声道转换**：自动将单声道转换为立体声
- **字节序**：自动处理 Little-Endian 格式

### 混音算法

使用简单的线性混音：

```
mixed = sample1 * volume1 + sample2 * volume2
```

### 音量控制

- TTS 播放时：Resource 音量 = 配置值 × 0.5
- TTS 停止时：Resource 音量 = 配置值 × 1.0
