SHELL := /bin/bash

DOCKER ?= docker
DOCKER_IMAGE ?= x11-input-daemon:local
DOCKER_VERSION ?= devel
DOCKER_REVISION ?= unknown
DOCKER_DIRTY ?= unknown

.PHONY: check check-docs check-secrets fmt-check tidy-check build test lint fuzz docker-build docker-check docker-smoke

check: check-docs check-secrets fmt-check tidy-check build test lint

check-docs:
	@./scripts/check-docs.sh

check-secrets:
	@./scripts/check-secrets.sh

fmt-check:
	@if [[ ! -f go.mod ]]; then \
		printf '%s\n' 'fmt-check: SKIP (go.mod is pending the active planning phase)'; \
	elif ! command -v gofmt >/dev/null 2>&1; then \
		printf '%s\n' 'fmt-check: FAIL (gofmt is required once go.mod exists)' >&2; exit 1; \
	else \
		mapfile -d '' -t files < <(git ls-files --cached --others --exclude-standard -z -- '*.go'); \
		if (($${#files[@]} == 0)); then \
			printf '%s\n' 'fmt-check: PASS (no Go source files)'; \
		else \
			unformatted="$$(gofmt -l "$${files[@]}")"; \
			if [[ -n "$$unformatted" ]]; then printf '%s\n' "$$unformatted" >&2; exit 1; fi; \
			printf 'fmt-check: PASS (%d Go files)\n' "$${#files[@]}"; \
		fi; \
	fi

tidy-check:
	@if [[ ! -f go.mod ]]; then \
		printf '%s\n' 'tidy-check: SKIP (go.mod is pending the active planning phase)'; \
	elif ! command -v go >/dev/null 2>&1; then \
		printf '%s\n' 'tidy-check: FAIL (Go is required once go.mod exists)' >&2; exit 1; \
	else \
		set -e; \
		go mod tidy -diff; \
		printf '%s\n' 'tidy-check: PASS'; \
	fi

build:
	@if [[ ! -f go.mod ]]; then \
		printf '%s\n' 'build: SKIP (go.mod is pending the active planning phase)'; \
	elif ! command -v go >/dev/null 2>&1; then \
		printf '%s\n' 'build: FAIL (Go is required once go.mod exists)' >&2; exit 1; \
	else \
		set -e; \
		go build -mod=readonly ./...; \
		printf '%s\n' 'build: PASS'; \
	fi

test:
	@if [[ ! -f go.mod ]]; then \
		printf '%s\n' 'test: SKIP (go.mod is pending the active planning phase)'; \
	elif ! command -v go >/dev/null 2>&1; then \
		printf '%s\n' 'test: FAIL (Go is required once go.mod exists)' >&2; exit 1; \
	else \
		set -e; \
		go vet -mod=readonly ./...; \
		go test -mod=readonly -race ./...; \
		printf '%s\n' 'test: PASS (vet and race tests)'; \
	fi

lint:
	@if [[ ! -f go.mod ]]; then \
		printf '%s\n' 'lint: SKIP (go.mod is pending the active planning phase)'; \
	elif ! command -v golangci-lint >/dev/null 2>&1; then \
		printf '%s\n' 'lint: SKIP LOCAL (golangci-lint is mandatory in CI)'; \
	else \
		set -e; \
		GOFLAGS=-mod=readonly golangci-lint run --timeout=5m; \
		printf '%s\n' 'lint: PASS'; \
	fi

fuzz:
	@if [[ ! -f go.mod ]]; then \
		printf '%s\n' 'fuzz: SKIP (go.mod is pending the active planning phase)'; \
	elif ! command -v go >/dev/null 2>&1; then \
		printf '%s\n' 'fuzz: FAIL (Go is required once go.mod exists)' >&2; exit 1; \
	else \
		./scripts/run-fuzz.sh; \
	fi

docker-build:
	$(DOCKER) build \
		--build-arg BUILD_VERSION="$(DOCKER_VERSION)" \
		--build-arg BUILD_REVISION="$(DOCKER_REVISION)" \
		--build-arg BUILD_DIRTY="$(DOCKER_DIRTY)" \
		--tag "$(DOCKER_IMAGE)" \
		.

docker-check: docker-build
	@set -euo pipefail; \
	user="$$( $(DOCKER) image inspect --format '{{.Config.User}}' "$(DOCKER_IMAGE)" )"; \
	case "$$user" in \
		''|0|0:*|root|root:*) printf 'docker-check: image user is not non-root: %s\n' "$$user" >&2; exit 1 ;; \
	esac; \
	entrypoint="$$( $(DOCKER) image inspect --format '{{json .Config.Entrypoint}}' "$(DOCKER_IMAGE)" )"; \
	want_entrypoint='["/usr/local/bin/xtest-server","-vnext","unix:/run/x11-input/input.sock","-vnext-allow","euid","-lock-file","/run/x11-input/authority.lock"]'; \
	if [[ "$$entrypoint" != "$$want_entrypoint" ]]; then \
		printf 'docker-check: unexpected entrypoint: %s\n' "$$entrypoint" >&2; exit 1; \
	fi; \
	exposed="$$( $(DOCKER) image inspect --format '{{json .Config.ExposedPorts}}' "$(DOCKER_IMAGE)" )"; \
	if [[ "$$exposed" != null ]]; then \
		printf 'docker-check: image exposes network ports: %s\n' "$$exposed" >&2; exit 1; \
	fi; \
	got="$$( $(DOCKER) run --rm \
		--network none \
		--read-only \
		--cap-drop ALL \
		--security-opt no-new-privileges=true \
		"$(DOCKER_IMAGE)" -version )"; \
	want='xtest-server version=$(DOCKER_VERSION) revision=$(DOCKER_REVISION) dirty=$(DOCKER_DIRTY) protocols=v2,dest-c3 backend=x11'; \
	if [[ "$$got" != "$$want" ]]; then \
		printf 'docker-check: version mismatch: got %q want %q\n' "$$got" "$$want" >&2; exit 1; \
	fi; \
	printf '%s\n' 'docker-check: PASS (non-root, Unix-only, no exposed ports, restricted runtime)'

docker-smoke: docker-build
	./scripts/docker-smoke.sh "$(DOCKER_IMAGE)"
