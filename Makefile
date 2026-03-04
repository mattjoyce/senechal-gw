VERSION    := $(shell ./scripts/version.sh)
COMMIT     := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -X main.version=$(VERSION) -X main.gitCommit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)
BINARY     := ~/.local/bin/ductile

.PHONY: build install

build:
	go build -ldflags "$(LDFLAGS)" -o ductile ./cmd/ductile

install: build
	systemctl --user stop ductile-local
	cp ductile $(BINARY)
	systemctl --user start ductile-local
	rm ductile
