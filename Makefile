SHELL := /bin/bash

.PHONY: check check-docs check-secrets fmt-check tidy-check build test lint fuzz

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
