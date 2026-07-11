# AGENTS.md

## Build & run

```bash
# Build (macOS needs ONNX Runtime + libopus via Homebrew: `brew install opus`)
make build

# Run the CLI harness (local mic/speaker)
make run-voicebot

# Run the WebSocket server (cmd/wsserver, one session per connection)
make run-wsserver          # listens on :8080/ws by default; see -addr/-path flags

# Build manually
GOTOOLCHAIN=$(go env GOTOOLCHAIN) CGO_CFLAGS="-I$(brew --prefix)/include/onnxruntime" CGO_LDFLAGS="-L$(brew --prefix)/lib" go build -o bin/voicebot ./cmd/voicebot
```

Config file: `data/voicebot.json` (template: `voicebot.example.json`). ASR/TTS/LLM keys are required on startup. `cmd/wsserver` shares the same config file.

## Test

```bash
make test              # go test ./...
make test-audio        # go test ./internal/audio (needs audio device + ASR keys)
```

No CI for Go -- only docs deploy in `.github/workflows/deploy-docs.yml`.

**After writing code, you MUST run `golangci-lint run ./...` and fix all issues before completing.**

## Architecture

Pipeline: `ASR → Agent → TTS → <output>` built via `pipeline.NewBuilder()` (linear) or `pipeline.NewDAGBuilder()` (fan-out/fan-in, e.g. `asr` broadcasting to both `agent` and a `stt`-echoing output stage). TTS audio flows through the pipeline `Message` bus (`TTSStage` registers `TTSProcessor.OnChunk` internally and emits `audio.TTSChunk` messages) rather than a side-channel callback, so `cmd/voicebot` and `cmd/wsserver` each plug in their own output stage (`PortAudioOutputStage`, `stages.WSOutputStage`).

Two entry points:

- `cmd/voicebot/` -- CLI harness: local mic/speaker, one process-wide session.
- `cmd/wsserver/` -- WebSocket server: one independent session + DAG pipeline per connection (`asr`→`agent`→`tts`→`ws_output`, plus `asr`→`ws_output` for STT echo). Protocol (hello/listen/abort/stt/tts JSON control frames + binary audio frames) is defined in `internal/wsproto/`; it's inspired by xiaozhi-esp32-server's protocol but deliberately omits `iot`/`mcp`/`server` (device-control/remote-tool-call/ops concerns unrelated to plain voice chat). Supports `auto` (server VAD) and `manual` (client-driven `listen start/stop`, via `ASRProcessor.BeginTurn/EndTurn`) modes, negotiated at hello time and fixed for the connection's lifetime. Audio wire format defaults to Opus/16kHz when a client's hello omits `audio_params` (negotiable per-connection, PCM passthrough or Opus, `internal/audio/codec/`); TTS output is always synthesized at 16kHz server-side (== `audio.InternalSampleRate`, independent of `cmd/voicebot`'s 22050Hz) since that's a valid Opus rate.

| Package | Role |
| --- | --- |
| `internal/config/` | JSON config loader + validation |
| `internal/logging/` | Zap wrapper; use `logging.Infof/Errorf/...` (not std lib `log`) |
| `internal/agent/` | LLM agent with tool calling loop |
| `internal/pipeline/` | Linear + DAG pipeline: `Stage` interface, `Builder`/`DAGBuilder`, `Message` bus |
| `internal/audio/` | ASRProcessor (VAD+ASR), TTSProcessor (splitter+synthesis), resampler |
| `internal/audio/codec/` | PCM passthrough / Opus codecs for WebSocket wire audio |
| `internal/wsproto/` | WebSocket voice session protocol message types |
| `internal/provider/` | ASR/TTS factory + Aliyun Dashscope impls |
| `internal/llm/` | LLM types + OpenAI-compatible provider |
| `internal/tools/` | Tool registry + MCP client |
| `internal/memory/` | Session buffer + SQLite long-term memory |
| `internal/session/` | Chat session / message tracking |
| `internal/text/` | Text segmenter, markdown filter, emotion tags |

Design docs in `docs/` -- read before modifying major modules.

## Diagnostics

- Default: `lens_diagnostics mode=delta` only. Do NOT run mode=all or mode=full unless explicitly told "跑".
- The user is called **piagent**. Always address them as piagent.

## Quirks

- **Mock convention**: inline mock structs in `*_test.go` files, no mock generator.
- **Provider pattern**: ASR/TTS use factory registration (`provider/asr/factory.go`, `provider/tts/factory.go`). LLM uses `llm/provider/` with blank import in `main.go`.
