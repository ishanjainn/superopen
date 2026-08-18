.PHONY: build test test-web install smoke lint

VERSION ?= $(shell tr -d '[:space:]' < VERSION 2>/dev/null || echo 0.1.0)
PKG_VERSION := github.com/ishanjainn/superopen/internal/version

build:
	go build -ldflags "-X $(PKG_VERSION).Version=$(VERSION)" -o bin/so ./cmd/so

test:
	go test -race -count=1 ./...

test-web:
	cd web && npm ci --ignore-scripts && npm run typecheck && npm test

lint:
	go vet ./...
	cd web && npm ci --ignore-scripts && npm run lint

install:
	go install -ldflags "-X $(PKG_VERSION).Version=$(VERSION)" ./cmd/so

smoke: build
	./bin/so --help
	./bin/so graph status --json >/dev/null
