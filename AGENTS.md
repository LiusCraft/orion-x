# AGENTS.md

## Build & run

```bash
# Build (macOS needs ONNX Runtime via Homebrew)
make build

# Run
make run-voicebot

# Build manually
GOTOOLCHAIN=$(go env GOTOOLCHAIN) CGO_CFLAGS="-I$(brew --prefix)/include/onnxruntime" CGO_LDFLAGS="-L$(brew --prefix)/lib" go build -o bin/voicebot ./cmd/voicebot
```

Config file: `data/voicebot.json` (template: `voicebot.example.json`). ASR/TTS/LLM keys are required on startup.

## Test

```bash
make test              # go test ./...
make test-audio        # go test ./internal/audio (needs audio device + ASR keys)
```

No CI for Go -- only docs deploy in `.github/workflows/deploy-docs.yml`.

**After writing code, you MUST run `golangci-lint run ./...` and fix all issues before completing.**

## Architecture

Pipeline: `ASR → Agent → TTS` built via `pipeline.NewBuilder()` in `cmd/voicebot/main.go`.

`cmd/voicebot/` is a quick test harness for the voice agent pipeline. The real product will expand beyond CLI to WebSocket, GUI, or other interaction channels.

| Package | Role |
|---|---|
| `internal/config/` | JSON config loader + validation |
| `internal/logging/` | Zap wrapper; use `logging.Infof/Errorf/...` (not std lib `log`) |
| `internal/agent/` | LLM agent with tool calling loop |
| `internal/pipeline/` | DAG pipeline: `Stage` interface, `Builder`, `Message` bus |
| `internal/audio/` | AudioInPipe (mic→ASR), AudioOutPipe (TTS→sink), VAD, resampler |
| `internal/provider/` | ASR/TTS factory + Aliyun Dashscope impls |
| `internal/llm/` | LLM types + OpenAI-compatible provider |
| `internal/tools/` | Tool registry + MCP client |
| `internal/memory/` | Session buffer + SQLite long-term memory |
| `internal/session/` | Chat session / message tracking |
| `internal/text/` | Text segmenter, markdown filter, emotion tags |

Design docs in `docs/` -- read before modifying major modules.

## Quirks

- **Mock convention**: inline mock structs in `*_test.go` files, no mock generator.
- **Provider pattern**: ASR/TTS use factory registration (`provider/asr/factory.go`, `provider/tts/factory.go`). LLM uses `llm/provider/` with blank import in `main.go`.
