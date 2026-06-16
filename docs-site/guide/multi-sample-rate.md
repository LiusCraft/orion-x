# 多采样率支持

当前多采样率能力集中在 TTS 播放链路：当 TTS provider 返回的音频采样率不是播放链路期望的采样率时，`TTSPipeline` 会插入 `ResamplingReader`。

## 当前实现

位置：

```text
internal/audio/resampler/
internal/audio/tts_pipeline_impl.go
```

`TTSPipeline` 播放 TTS stream 时会读取：

```go
ttsSampleRate := stream.SampleRate()
ttsChannels := stream.Channels()
systemSampleRate := 16000
```

如果 `ttsSampleRate != 16000`，会使用线性插值重采样到 16 kHz。

## Resampler 接口

```go
type Resampler interface {
    Resample(input []int16, inputRate, outputRate, channels int) ([]int16, error)
}
```

`ResamplingReader` 包装 `io.Reader`，输入和输出都是 little-endian int16 PCM。

```go
reader := resampler.NewResamplingReader(
    audioReader,
    ttsSampleRate,
    16000,
    ttsChannels,
    resampler.NewLinearResampler(),
)
```

## 算法

当前实现是线性插值：

```text
ratio = inputRate / outputRate
position = outputIndex * ratio
i = floor(position)
frac = position - i
output = input[i] * (1 - frac) + input[i+1] * frac
```

特点：

- 无第三方依赖
- 足够快，适合实时语音
- 音质不如 sinc 等高质量重采样算法

## 配置关系

TTS 采样率配置在：

```json
{
  "provider": {
    "tts": {
      "aliyun": {
        "sample_rate": 16000
      }
    }
  }
}
```

`audio.mixer.sample_rate` 当前保留在配置结构中，但主 TTS 播放链路的重采样目标仍是 16 kHz。需要把系统播放采样率做成真正可配置时，应同步改 `TTSPipeline`、`AudioSink` 初始化和 PortAudio sink format。

## 支持场景

| TTS 输出采样率 | 播放目标 | 行为 |
|---:|---:|---|
| 16000 | 16000 | 直接透传 |
| 22050 | 16000 | 线性插值降采样 |
| 24000 | 16000 | 线性插值降采样 |
| 48000 | 16000 | 线性插值降采样 |

## 限制

- 当前只处理 int16 PCM
- 当前输出目标采样率固定为 16 kHz
- 当前没有声道数转换逻辑
- `ResamplingReader` 是简单流式包装，不做高质量滤波
