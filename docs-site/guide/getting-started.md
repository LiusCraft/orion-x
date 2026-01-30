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

### 方式一：配置文件（推荐）

复制示例配置文件并填入你的 API 密钥：

```bash
cp config/voicebot.example.json config/voicebot.json
```

编辑 `config/voicebot.json`，填入：
- 阿里云 Dashscope API Key (ASR/TTS)
- 智谱 AI API Key (LLM)

### 方式二：环境变量

```bash
export DASHSCOPE_API_KEY=your_dashscope_api_key
export ZHIPU_API_KEY=your_zhipu_api_key
```

环境变量会覆盖配置文件中的值。

## 运行

```bash
go run cmd/voicebot/main.go
```

### 运行参数

```bash
# 指定配置文件
go run cmd/voicebot/main.go -config /path/to/config.json

# 设置日志级别
LOG_LEVEL=debug go run cmd/voicebot/main.go
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

## 相关文档

- [配置管理](/guide/configuration) - 详细配置说明
- [工具开发](/guide/development) - 工具开发指南
