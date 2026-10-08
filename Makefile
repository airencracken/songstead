.DEFAULT_GOAL := help
VERSION ?= 0.1.1
GOCACHE ?= /tmp/songstead-go-cache
export GOCACHE

.PHONY: help build test test-race test-js test-mutations test-sandbox check demo
help:
	@echo 'Songstead: make build, test, test-race, test-js, test-mutations, check, demo'
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$(VERSION)" -o bin/songstead ./cmd/songstead
test:
	go test ./...
test-race:
	go test -race ./...
test-packaging:
	python3 scripts/test_packaging.py
test-js:
	node --test internal/web/static/theme.test.js
test-mutations:
	python3 scripts/mutate.py
check: test-race test-js test-mutations test-packaging
	go vet ./...
	@test -z "$$(gofmt -l cmd internal)" || { echo 'Run gofmt on cmd and internal'; exit 1; }
demo: build
	python3 scripts/demo.py

test-sandbox:
	COMFYWARE_SANDBOX_TEST=1 go test ./cmd/songstead -run 'TestRealSandbox' -count=1
