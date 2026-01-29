# 多采样率支持设计文档

## 背景

不同的 TTS/ASR 厂商只支持特定的采样率：
- **DashScope TTS**: 支持 16000, 22050, 24000, 48000 Hz
- **Edge TTS**: 24000 Hz
- **其他厂商**: 可能有不同要求

当前问题：
1. AudioMixer 硬编码 16000 Hz
2. 没有音频重采样功能
3. TTS 输出可能是不同采样率，但 Mixer 只能处理 16000 Hz

## 设计目标

1. **支持多采样率输入**：允许 TTS 厂商使用其原生采样率
2. **系统内部统一**：保持内部处理采样率一致（16000 Hz）
3. **透明转换**：自动进行采样率转换，对上层透明
4. **可配置化**：Mixer 采样率可通过配置调整
5. **高音质**：使用高质量重采样算法

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
                            │
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

### 重采样策略

**方案 A：在 OutPipe 中重采样（推荐）**
- TTS 输出后立即重采样到系统标准采样率
- Mixer 只处理统一采样率的数据
- 优点：Mixer 逻辑简单，性能可控
- 缺点：需要在多个地方插入 Resampler

**方案 B：在 Mixer 中重采样**
- Mixer 支持多采样率输入，内部自动重采样
- 优点：对上层完全透明
- 缺点：Mixer 逻辑复杂，性能开销大

**最终选择**：方案 A（OutPipe 重采样）

## 接口设计

### 1. Resampler 接口

```go
// Resampler 音频重采样器
type Resampler interface {
    // Resample 重采样音频数据
    // input: 输入 PCM 数据 (int16)
    // inputRate: 输入采样率
    // outputRate: 输出采样率
    // channels: 声道数
    // 返回重采样后的 PCM 数据
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

func NewResamplingReader(source io.Reader, inputRate, outputRate, channels int) *ResamplingReader
func (r *ResamplingReader) Read(p []byte) (int, error)
```

### 2. MixerConfig 扩展

```go
type MixerConfig struct {
    TTSVolume      float64
    ResourceVolume float64
    SampleRate     int     // 新增：系统采样率（默认 16000）
    Channels       int     // 新增：输出声道数（默认 2）
}
```

### 3. TTS Stream 元数据

```go
// Stream 接口扩展，增加采样率信息
type Stream interface {
    WriteTextChunk(ctx context.Context, text string) error
    Close(ctx context.Context) error
    AudioReader() io.ReadCloser
    SampleRate() int  // 新增：返回音频采样率
    Channels() int    // 新增：返回声道数
}
```

## 重采样算法

### 算法选择

1. **线性插值（Linear Interpolation）**
   - 优点：简单、快速、无依赖
   - 缺点：音质一般，高频失真
   - 适用场景：实时语音，对音质要求不高

2. **Sinc 插值（Sinc Resampling）**
   - 优点：高音质，频率响应好
   - 缺点：计算复杂，延迟高
   - 需要第三方库：libsamplerate (SRC)

3. **FFT 重采样**
   - 优点：音质最佳
   - 缺点：复杂度高，不适合流式处理

**初期实现**：线性插值（简单实现，后续可扩展）

### 线性插值实现

```
输出采样点 = 输入采样点[i] * (1 - frac) + 输入采样点[i+1] * frac

其中:
  ratio = inputRate / outputRate
  position = outputIndex * ratio
  i = floor(position)
  frac = position - i
```

## 配置方案

### config/voicebot.json

```json
{
  "tts": {
    "provider": "dashscope",
    "sample_rate": 24000,  // TTS 厂商原生采样率
    "format": "pcm"
  },
  "audio": {
    "mixer": {
      "sample_rate": 16000,  // 系统采样率
      "channels": 2,
      "tts_volume": 1.0,
      "resource_volume": 1.0
    }
  }
}
```

### 自动协商

1. TTS Provider 返回其支持的采样率
2. OutPipe 检测 TTS 采样率与系统采样率
3. 如果不一致，自动插入 ResamplingReader
4. Mixer 接收统一采样率的数据

## 实现步骤

### 阶段一：基础框架（优先级：高）

1. **创建 Resampler 接口和线性插值实现**
   - `internal/audio/resampler.go`
   - `internal/audio/resampler_linear.go`
   - 单元测试

2. **实现 ResamplingReader**
   - `internal/audio/resampling_reader.go`
   - 包装 io.Reader，自动重采样
   - 单元测试

3. **扩展 MixerConfig**
   - 添加 `SampleRate` 和 `Channels` 字段
   - 修改 `mixer_impl.go` 使用配置中的采样率
   - 更新配置文件和默认值

4. **扩展 TTS Stream 接口**
   - 添加 `SampleRate()` 和 `Channels()` 方法
   - DashScope Provider 实现

5. **OutPipe 集成重采样**
   - 检测 TTS 采样率
   - 自动插入 ResamplingReader
   - 日志记录采样率转换

### 阶段二：高级功能（优先级：中）

1. **支持多种重采样算法**
   - 接入 libsamplerate (可选)
   - 配置化算法选择

2. **性能优化**
   - 缓存重采样器实例
   - 批量处理优化

3. **采样率自动检测**
   - 从 PCM 数据推断采样率（困难）
   - 从 TTS 元数据获取

### 阶段三：测试和验证（优先级：中）

1. **单元测试**
   - 测试各种采样率转换（16k→24k, 24k→16k, 22k→16k）
   - 边界条件测试

2. **音质测试**
   - 生成测试音频文件
   - 对比原始音频和重采样后音频

3. **集成测试**
   - 端到端 TTS 播放测试
   - 多厂商 TTS 兼容性测试

## 性能考虑

### 延迟

- **线性插值**：~0.1ms (16k→24k, 1024 samples)
- **Sinc 插值**：~1-5ms
- **目标**：总延迟 < 10ms

### 内存

- 缓冲区大小：输入样本数 * (outputRate / inputRate) * 2 字节
- 示例：1024 samples @ 16k→24k = 1536 samples * 2 = 3KB

### CPU

- 线性插值：约 5-10% CPU（实时处理）
- Sinc 插值：约 20-30% CPU

## 兼容性

### 支持的采样率转换

| 源采样率 | 目标采样率 | 倍率 | 难度 |
|---------|-----------|------|------|
| 16000   | 16000     | 1.0  | 无需转换 |
| 22050   | 16000     | 0.73 | 简单 |
| 24000   | 16000     | 0.67 | 简单 |
| 48000   | 16000     | 0.33 | 简单 |
| 16000   | 24000     | 1.5  | 中等 |
| 16000   | 48000     | 3.0  | 中等 |

### 厂商兼容性

- ✅ DashScope (16k/22k/24k/48k)
- ✅ Edge TTS (24k)
- ✅ 其他标准采样率 (8k/32k/44.1k)

## 错误处理

1. **不支持的采样率**：返回错误，记录日志
2. **重采样失败**：降级到原始采样率，警告日志
3. **采样率检测失败**：使用默认 16kHz

## 日志记录

```go
logging.Infof("TTS output: %d Hz, system: %d Hz, resampling required", ttsRate, systemRate)
logging.Debugf("Resampling: %d samples @ %d Hz → %d samples @ %d Hz", 
    inputSamples, inputRate, outputSamples, outputRate)
```

## 文档更新

需要更新的文档：
- `docs/tts.md` - 添加多采样率说明
- `docs/audio-architecture.md` - 添加重采样模块
- `config/voicebot.example.json` - 添加采样率配置示例
- `README.md` - 更新支持的采样率列表

## 测试计划

### 单元测试

```go
func TestLinearResampler_16kTo24k(t *testing.T)
func TestLinearResampler_24kTo16k(t *testing.T)
func TestResamplingReader_StreamProcessing(t *testing.T)
func TestMixer_WithMultipleSampleRates(t *testing.T)
```

### 集成测试

```bash
# 测试不同采样率的 TTS
go run ./cmd/tts -sample-rate 16000 -text "测试16k"
go run ./cmd/tts -sample-rate 24000 -text "测试24k"

# 播放并验证音质
```

## 未来扩展

1. **自适应采样率**：根据网络带宽动态调整
2. **多声道支持**：Stereo/5.1 环绕声
3. **硬件加速**：使用 SIMD 优化重采样
4. **压缩格式支持**：MP3/Opus 解码后重采样

## 参考资料

- [libsamplerate](http://www.mega-nerd.com/SRC/) - 高质量采样率转换库
- [SoX Resampler](http://sox.sourceforge.net/) - SoX 重采样算法
- [WebRTC Resampler](https://webrtc.googlesource.com/src/+/refs/heads/main/common_audio/resampler/) - WebRTC 实时重采样