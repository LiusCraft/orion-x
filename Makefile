UNAME := $(shell uname -s)

ifeq ($(UNAME),Darwin)
BREW_PREFIX := $(shell brew --prefix)
CGO_FLAGS := CGO_CFLAGS="-I$(BREW_PREFIX)/include/onnxruntime" CGO_LDFLAGS="-L$(BREW_PREFIX)/lib"
endif

GO := GOTOOLCHAIN=$(GO_TOOLCHAIN) $(CGO_FLAGS) go

.DEFAULT_GOAL := help

.PHONY: help all build build-voicebot run-voicebot build-wsserver run-wsserver build-manager build-tools run-manager install-frontend dev-frontend build-frontend swagger test test-audio lint clean

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "  build             Build voicebot, wsserver, manager, and tools"
	@echo "  run-voicebot      Run the local voicebot CLI"
	@echo "  run-wsserver      Run the WebSocket voice server"
	@echo "  run-manager       Run the web management server"
	@echo "  frontend          Install/develop/build: install-frontend, dev-frontend, build-frontend"
	@echo "  quality           Run tests, audio tests, or lint: test, test-audio, lint"

all: build

build: build-voicebot build-wsserver build-manager build-tools

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

build-tools:
	mkdir -p bin
	go build -o bin/tools ./cmd/tools

# Regenerate Swagger/OpenAPI files after changing cmd/manager/swagger.go.
swagger:
	go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/manager/swagger.go -d . -o docs/manager

run-manager: build-manager
	./bin/manager

WEB_DIR := web/manager

install-frontend:
	cd $(WEB_DIR) && npm install

dev-frontend:
	cd $(WEB_DIR) && npm run dev

build-frontend:
	cd $(WEB_DIR) && npm run build

test:
	$(GO) test ./...

test-audio:
	$(GO) test ./internal/audio

lint:
	$(CGO_FLAGS) golangci-lint run ./...

clean:
	rm -rf bin
