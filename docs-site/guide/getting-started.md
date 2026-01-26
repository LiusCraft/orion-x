# 快速开始

## 环境要求

- Go 1.24.4+
- PortAudio 库 (音频 I/O)

## 安装 PortAudio

### macOS

```bash
brew install portaudio
```

### Ubuntu/Debian

```bash
sudo apt-get install libportaudio2
```

### Windows

下载并安装 [PortAudio](http://www.portaudio.com/download.html)

## 克隆项目

```bash
git clone https://github.com/liuscraft/orion-x.git
cd orion-x
```

## 配置

复制示例配置文件并填入你的 API 密钥：

```bash
cp config/voicebot.example.json config/voicebot.json
```

编辑 `config/voicebot.json`，填入：
- 阿里云 Dashscope API Key (ASR/TTS)
- 智谱 AI API Key (LLM)

## 运行

```bash
go run cmd/voicebot/main.go
```

## 测试

```bash
# 运行所有测试
go test ./...

# 运行特定模块测试
go test ./internal/voicebot/

# 查看测试覆盖率
go test -cover ./...
```
