#!/usr/bin/env bash
set -euo pipefail

fuzz_time=${FUZZ_TIME:-20s}
total_timeout=${FUZZ_TOTAL_TIMEOUT:-10m}

[[ "$fuzz_time" =~ ^[1-9][0-9]*(s|m)$ ]] || {
    printf 'fuzz: invalid FUZZ_TIME: %s\n' "$fuzz_time" >&2
    exit 2
}
[[ "$total_timeout" =~ ^[1-9][0-9]*(s|m)$ ]] || {
    printf 'fuzz: invalid FUZZ_TOTAL_TIMEOUT: %s\n' "$total_timeout" >&2
    exit 2
}
command -v timeout >/dev/null 2>&1 || {
    printf '%s\n' 'fuzz: GNU timeout is required' >&2
    exit 1
}

run_all() {
    local package listing fuzz_name count=0
    while IFS= read -r package; do
        listing=$(go test -mod=readonly -run '^$' -list '^Fuzz' "$package")
        while IFS= read -r fuzz_name; do
            [[ "$fuzz_name" =~ ^Fuzz[A-Za-z0-9_]+$ ]] || continue
            printf 'fuzz: running %s in %s for %s\n' "$fuzz_name" "$package" "$fuzz_time"
            go test -mod=readonly -run '^$' -fuzz "^${fuzz_name}$" -fuzztime "$fuzz_time" "$package"
            ((count += 1))
        done <<< "$listing"
    done < <(go list -mod=readonly ./...)

    if ((count == 0)); then
        printf '%s\n' 'fuzz: FAIL (Go module has no fuzz targets)' >&2
        return 1
    else
        printf 'fuzz: PASS (%d targets)\n' "$count"
    fi
}

export GOFLAGS=-mod=readonly
export -f run_all
export fuzz_time
timeout --foreground "$total_timeout" bash -c run_all
