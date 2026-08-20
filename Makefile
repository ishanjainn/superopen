.PHONY: build test test-web install uninstall smoke lint

VERSION ?= $(shell tr -d '[:space:]' < VERSION 2>/dev/null || echo 0.1.0)
PKG_VERSION := github.com/ishanjainn/superopen/internal/version

build:
	go build -ldflags "-X $(PKG_VERSION).Version=$(VERSION)" -o bin/so ./cmd/so

test:
	go test -race -timeout 30m -count=1 ./...

test-web:
	cd web && npm ci --ignore-scripts && npm run typecheck && npm test

lint:
	go vet ./...
	cd web && npm ci --ignore-scripts && npm run lint

# Same layout as production curl / install.ps1 (~/.superopen/bin + so install).
install:
	sh scripts/install.sh

uninstall:
	sh scripts/uninstall.sh

smoke: build
	./bin/so --help
	./bin/so graph status --json >/dev/null
