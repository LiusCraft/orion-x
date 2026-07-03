UNAME := $(shell uname -s)

ifeq ($(UNAME),Darwin)
BREW_PREFIX := $(shell brew --prefix)
CGO_FLAGS := CGO_CFLAGS="-I$(BREW_PREFIX)/include/onnxruntime" CGO_LDFLAGS="-L$(BREW_PREFIX)/lib"
endif

GO := GOTOOLCHAIN=$(GO_TOOLCHAIN) $(CGO_FLAGS) go

.PHONY: all build build-voicebot run-voicebot build-wsserver run-wsserver build-manager run-manager test test-audio lint clean

all: build

build: build-voicebot build-wsserver build-manager

build-voicebot:
	mkdir -p bin
	$(GO) build -o bin/voicebot ./cmd/voicebot

run-voicebot: build-voicebot
	./bin/voicebot

# wsserver additionally needs libopus (e.g. `brew install opus`); it's
# discovered automatically via pkg-config, no extra CGO flags required.
build-wsserver:
	mkdir -p bin
	$(GO) build -o bin/wsserver ./cmd/wsserver

run-wsserver: build-wsserver
	./bin/wsserver -config data/wsserver.yaml

build-manager:
	mkdir -p bin
	go build -o bin/manager ./cmd/manager

run-manager: build-manager
	./bin/manager

test:
	$(GO) test ./...

test-audio:
	$(GO) test ./internal/audio

lint:
	$(CGO_FLAGS) golangci-lint run ./...

clean:
	rm -rf bin
