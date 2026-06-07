UNAME := $(shell uname -s)

ifeq ($(UNAME),Darwin)
BREW_PREFIX := $(shell brew --prefix)
CGO_FLAGS := CGO_CFLAGS="-I$(BREW_PREFIX)/include/onnxruntime" CGO_LDFLAGS="-L$(BREW_PREFIX)/lib"
endif

GO := GOTOOLCHAIN=$(GO_TOOLCHAIN) $(CGO_FLAGS) go

.PHONY: all build build-voicebot run-voicebot test test-audio clean

all: build

build: build-voicebot

build-voicebot:
	mkdir -p bin
	$(GO) build -o bin/voicebot ./cmd/voicebot

run-voicebot: build-voicebot
	./bin/voicebot

test:
	$(GO) test ./...

test-audio:
	$(GO) test ./internal/audio

clean:
	rm -rf bin
