VERSION    := $(shell ./scripts/version.sh)
COMMIT     := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -X main.version=$(VERSION) -X main.gitCommit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)
BINARY     := ~/.local/bin/ductile
UNAME_S    := $(shell uname -s)

.PHONY: build install test test-docker test-premerge test-main

build:
	go build -ldflags "$(LDFLAGS)" -o ductile ./cmd/ductile
ifeq ($(UNAME_S),Darwin)
	codesign --sign com.mattjoyce.ductile --identifier com.mattjoyce.ductile --force --timestamp=none ./ductile
endif

install: build
ifeq ($(UNAME_S),Linux)
	systemctl --user stop ductile-local
	cp ductile $(BINARY)
	systemctl --user start ductile-local
else
	cp ductile $(BINARY)
endif
	rm ductile

test:
	./scripts/test-fast

test-docker:
	./scripts/test-docker

test-premerge:
	./scripts/test-premerge

test-main:
	./scripts/test-main
