# 快速开始

## 环境要求

- Go 1.24.4+
- PortAudio
- ONNX Runtime（macOS 下 `make build` 需要）

## 安装依赖

### macOS

```bash
brew install portaudio onnxruntime
```

### Ubuntu / Debian

```bash
sudo apt-get install libportaudio2
```

## 准备配置

复制示例配置：

```bash
cp voicebot.example.json data/voicebot.json
```

填入以下密钥：

- `provider.asr.aliyun.api_key`
- `provider.tts.aliyun.api_key`
- `provider.llm.openai.api_key`

也可以使用环境变量覆盖：

```bash
export DASHSCOPE_API_KEY=...
export ZHIPU_API_KEY=...
export LOG_LEVEL=debug
```

## 运行

```bash
make run-voicebot
```

或手动构建：

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) CGO_CFLAGS="-I$(brew --prefix)/include/onnxruntime" CGO_LDFLAGS="-L$(brew --prefix)/lib" go build -o bin/voicebot ./cmd/voicebot
```

然后执行：

```bash
./bin/voicebot -config data/voicebot.json
```

## 测试

```bash
make test
go vet ./...
```

## 入口说明

- 当前产品入口是 `cmd/voicebot`
- 运行时流程是 `ASR -> Agent -> TTS`
- 工具调用走 `internal/tools.Manager`
- 记忆能力走 `internal/memory.Service`
