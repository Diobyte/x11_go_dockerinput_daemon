#!/usr/bin/env bash
set -euo pipefail

repository_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
    printf '%s\n' 'check-docs: repository root could not be determined' >&2
    exit 1
}
cd "$repository_root"

required=(
    README.md
    CONTRIBUTING.md
    SECURITY.md
    LICENSE
    docs/PROTOCOL.md
)

for path in "${required[@]}"; do
    if [[ ! -s "$path" ]]; then
        printf 'check-docs: required file is missing or empty: %s\n' "$path" >&2
        exit 1
    fi
done

mapfile -d '' -t candidates < <(
    git ls-files --cached --others --exclude-standard -z
)

if ((${#candidates[@]} == 0)); then
    printf '%s\n' 'check-docs: no tracked or non-ignored files were found' >&2
    exit 1
fi

is_reviewed_text() {
    case "$1" in
        *.md | *.markdown | *.yml | *.yaml | *.sh | *.bash | *.go | *.json | \
            *.toml | *.ini | *.env | *.txt | *.mod | *.sum | Dockerfile | \
            Dockerfile.* | Makefile | .editorconfig | .gitattributes | .gitignore)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

failed=0
for path in "${candidates[@]}"; do
    is_reviewed_text "$path" || continue
    [[ -f "$path" ]] || continue
    safe_path=./$path

    if LC_ALL=C awk '/[[:blank:]]$/ { found=1 } END { exit !found }' "$safe_path"; then
        printf 'check-docs: trailing whitespace: %s\n' "$path" >&2
        failed=1
    fi

    if [[ -s "$safe_path" ]] && [[ $(tail -c 1 -- "$safe_path" | wc -l) -eq 0 ]]; then
        printf 'check-docs: missing final newline: %s\n' "$path" >&2
        failed=1
    fi
done

if ((failed != 0)); then
    exit 1
fi

bash -n scripts/*.sh
printf '%s\n' 'check-docs: passed'
