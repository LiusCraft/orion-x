# 多采样率支持

## 概述

多采样率支持允许系统处理不同 TTS/ASR 厂商的原生采样率，自动进行采样率转换，对上层透明。

### 背景

不同 TTS 厂商支持不同的采样率：

| 厂商 | 支持的采样率 |
|------|-------------|
| DashScope TTS | 16000, 22050, 24000, 48000 Hz |
| Edge TTS | 24000 Hz |
| 其他厂商 | 各有不同 |

### 设计目标

1. **支持多采样率输入**：允许 TTS 使用其原生采样率
2. **系统内部统一**：内部处理采样率一致（16000 Hz）
3. **透明转换**：自动进行采样率转换
4. **可配置化**：Mixer 采样率可通过配置调整

## 架构设计

### 系统采样率标准

```
┌─────────────────────────────────────────────────────────┐
│                    系统标准采样率                          │
│                   System Sample Rate                     │
│                      16000 Hz                            │
└─────────────────────────────────────────────────────────┘

输入端 (ASR/Microphone)     │     输出端 (TTS/Mixer)
      16000 Hz              │         16000 Hz (默认)
```

### 数据流转

```
TTS Provider (任意采样率)
    ↓
AudioReader (原始采样率: 16k/22k/24k/48k)
    ↓
Resampler (重采样到系统采样率)
    ↓
AudioMixer (系统采样率: 16k)
    ↓
PortAudio (系统采样率: 16k, Stereo)
    ↓
扬声器
```

## 接口设计

### Resampler 接口

```go
// Resampler 音频重采样器
type Resampler interface {
    // Resample 重采样音频数据
    Resample(input []int16, inputRate, outputRate, channels int) ([]int16, error)
}

// ResamplingReader 包装 io.Reader，自动进行重采样
type ResamplingReader struct {
    source      io.Reader
    resampler   Resampler
    inputRate   int
    outputRate  int
    channels    int
}
```

### MixerConfig 扩展

```go
type MixerConfig struct {
    TTSVolume      float64
    ResourceVolume float64
    SampleRate     int     // 系统采样率（默认 16000）
    Channels       int     // 输出声道数（默认 2）
}
```

### TTS Stream 元数据

```go
type Stream interface {
    WriteTextChunk(ctx context.Context, text string) error
    Close(ctx context.Context) error
    AudioReader() io.ReadCloser
    SampleRate() int  // 返回音频采样率
    Channels() int    // 返回声道数
}
```

## 重采样算法

### 线性插值（初期实现）

```
输出采样点 = 输入采样点[i] * (1 - frac) + 输入采样点[i+1] * frac

其中:
  ratio = inputRate / outputRate
  position = outputIndex * ratio
  i = floor(position)
  frac = position - i
```

| 算法 | 优点 | 缺点 | 适用场景 |
|------|------|------|----------|
| 线性插值 | 简单、快速、无依赖 | 音质一般 | 实时语音 |
| Sinc 插值 | 高音质 | 计算复杂 | 高质量要求 |
| FFT 重采样 | 音质最佳 | 不适合流式 | 离线处理 |

## 配置方案

### config/voicebot.json

```json
{
  "tts": {
    "provider": "dashscope",
    "sample_rate": 24000,
    "format": "pcm"
  },
  "audio": {
    "mixer": {
      "sample_rate": 16000,
      "channels": 2,
      "tts_volume": 1.0,
      "resource_volume": 1.0
    }
  }
}
```

### 自动协商流程

1. TTS Provider 返回其支持的采样率
2. OutPipe 检测 TTS 采样率与系统采样率
3. 如果不一致，自动插入 ResamplingReader
4. Mixer 接收统一采样率的数据

## 支持的采样率转换

| 源采样率 | 目标采样率 | 倍率 | 难度 |
|---------|-----------|------|------|
| 16000   | 16000     | 1.0  | 无需转换 |
| 22050   | 16000     | 0.73 | 简单 |
| 24000   | 16000     | 0.67 | 简单 |
| 48000   | 16000     | 0.33 | 简单 |
| 16000   | 24000     | 1.5  | 中等 |
| 16000   | 48000     | 3.0  | 中等 |

## 性能考虑

| 指标 | 线性插值 | Sinc 插值 |
|------|----------|-----------|
| 延迟 | ~0.1ms | ~1-5ms |
| CPU | 5-10% | 20-30% |
| 内存 | 3KB/1024 samples | 略高 |

## 错误处理

1. **不支持的采样率**：返回错误，记录日志
2. **重采样失败**：降级到原始采样率，警告日志
3. **采样率检测失败**：使用默认 16kHz
