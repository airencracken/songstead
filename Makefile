.DEFAULT_GOAL := help
GOCACHE ?= /tmp/songstead-go-cache
export GOCACHE

.PHONY: help build test test-race test-js test-mutations check demo
help:
	@echo 'Songstead: make build, test, test-race, test-js, test-mutations, check, demo'
build:
	CGO_ENABLED=0 go build -trimpath -o bin/songstead ./cmd/songstead
test:
	go test ./...
test-race:
	go test -race ./...
test-js:
	node --test internal/web/static/theme.test.js
test-mutations:
	python3 scripts/mutate.py
check: test-race test-js test-mutations
	go vet ./...
	@test -z "$$(gofmt -l cmd internal)" || { echo 'Run gofmt on cmd and internal'; exit 1; }
demo: build
	python3 scripts/demo.py
