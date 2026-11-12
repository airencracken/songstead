.DEFAULT_GOAL := help
VERSION ?= $(shell cat VERSION)
GOCACHE ?= /tmp/songstead-go-cache
export GOCACHE

.PHONY: help build test test-race test-js test-mutations test-sandbox test-cli test-browser check demo
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
	node --test internal/web/static/*.test.js
test-mutations:
	python3 scripts/comfy.py mutate scripts/mutations.json scripts/mutations/search_index.json
test-cli: build
	python3 scripts/test_cli.py
	python3 "$$(go list -m -f '{{.Dir}}' github.com/airencracken/comfylib)/tools/check_password_cli.py" --binary bin/songstead --command create-owner --data-env SONGSTEAD_DATA_DIR --prompt "Password: " --confirmation "Confirm password: "
	python3 "$$(go list -m -f '{{.Dir}}' github.com/airencracken/comfylib)/tools/check_setup.py" --binary bin/songstead --app songstead
check: test-suite-assets test-config test-design-mutations test-containers test-race test-js test-mutations test-packaging test-cli
	go vet ./...
	@test -z "$$(gofmt -l cmd internal scripts)" || { echo 'Run gofmt on cmd, internal and scripts'; exit 1; }
demo: build
	python3 scripts/demo.py

test-sandbox:
	COMFYWARE_SANDBOX_TEST=1 go test ./cmd/songstead -run 'TestRealSandbox' -count=1

test-browser: build
	cd scripts/browser && npm ci && npm test

.PHONY: test-containers test-container-lifecycle
test-containers: ## Check container release, schema, mutation and publication contracts
	python3 scripts/comfy.py test-containers

test-container-lifecycle: ## Exercise a local image (build it first; needs Docker)
	python3 scripts/comfy.py container-lifecycle --image songstead:local --version "$(VERSION)" --upgrade

.PHONY: test-container-registry
test-container-registry: ## Exercise real multi-platform publication in an isolated local registry
	python3 scripts/comfy.py container-registry

.PHONY: test-design-mutations
test-design-mutations: ## Verify invitation design regressions reject deliberate defects
	GOWORK=off go mod download
	GOWORK=off python3 scripts/comfy.py mutate scripts/mutations/design.json

.PHONY: test-config
test-config:
	python3 scripts/comfy.py service-config

.PHONY: test-suite-assets
test-suite-assets: ## Verify shared module integration and deployment contracts
	python3 scripts/comfy.py check
	python3 scripts/comfy.py suite-deployment
	python3 scripts/comfy.py release-dispatch
