BIN := bin/model-notifier
LOCAL_CFG := bin/config.local.yaml

.PHONY: help build test lint run once docker up fixtures

help: ## list targets
	@grep -E '^[a-z]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

build: ## build binary into bin/
	go build -trimpath -o $(BIN) ./cmd/model-notifier

test: ## run all tests (offline)
	go test ./...

lint: ## gofmt check + go vet
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...

$(LOCAL_CFG): config.yaml
	@mkdir -p bin
	sed 's#/data/state.json#./state.json#' config.yaml > $@

run: $(LOCAL_CFG) ## run daemon locally (state in ./state.json)
	go run ./cmd/model-notifier -config $(LOCAL_CFG)

once: $(LOCAL_CFG) ## run all checks once and exit; ARGS=-notify-on-seed to send on seeding
	go run ./cmd/model-notifier -config $(LOCAL_CFG) -once $(ARGS)

docker: ## build docker image
	docker build -t model-notifier .

up: ## docker compose up
	docker compose up -d --build

fixtures: ## refresh test snapshots from live sources
	curl -fsSL -o internal/source/testdata/anthropic-models-overview.html https://platform.claude.com/docs/en/models/overview
	curl -fsSL -o internal/source/testdata/anthropic-releases.atom https://github.com/anthropics/anthropic-sdk-python/releases.atom
	curl -fsSL -o internal/source/testdata/openai-changelog.html https://developers.openai.com/api/docs/changelog
	curl -fsSL -o internal/source/testdata/openai-releases.atom https://github.com/openai/openai-python/releases.atom
