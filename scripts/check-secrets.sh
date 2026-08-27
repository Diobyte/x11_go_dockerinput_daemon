#!/usr/bin/env bash
set -euo pipefail

repository_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
    printf '%s\n' 'check-secrets: repository root could not be determined' >&2
    exit 1
}
cd "$repository_root"

if command -v gitleaks >/dev/null 2>&1; then
    # Full redaction prevents findings from echoing credential material. Directory
    # mode includes non-ignored worktree files before the first commit.
    gitleaks dir --no-banner --redact=100 --exit-code 1 .
    printf '%s\n' 'check-secrets: PASS (gitleaks)'
    exit 0
fi

# This fallback catches common high-confidence formats without displaying matching
# lines. CI always installs and runs pinned gitleaks; fallback success is not a
# substitute for that required vetted scan.
mapfile -d '' -t candidates < <(
    git ls-files --cached --others --exclude-standard -z
)
((${#candidates[@]} > 0)) || {
    printf '%s\n' 'check-secrets: no tracked or non-ignored files were found' >&2
    exit 1
}

# Build signatures from fragments so the scanner does not flag its own source.
private_key_pattern='-----BEGIN ([A-Z0-9]+ )?PRIVATE'' KEY-----'
xauth_pattern='MIT-MAGIC-COOKIE-1[[:space:]]+[0-9a-fA-F]{32,}'
aws_access_pattern='AKIA''[A-Z0-9]{16}'
github_pattern='gh[pousr]_[A-Za-z0-9_]''{30,}'
jwt_pattern='eyJ[A-Za-z0-9_-]{8,}\.''eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}'
assignment_pattern="(api|auth|access|refresh|secret)[_-]?(key|token|secret)?[[:space:]]*[:=][[:space:]]*['\"]?[^[:space:]'\"]{16,}"
password_pattern="(password|passwd|pwd)[[:space:]]*[:=][[:space:]]*['\"]?[^[:space:]'\"]{12,}"
secret_pattern="(${private_key_pattern}|${xauth_pattern}|${aws_access_pattern}|${github_pattern}|${jwt_pattern}|${assignment_pattern}|${password_pattern})"

failed=0
for path in "${candidates[@]}"; do
    [[ -f "$path" && ! -L "$path" ]] || continue
    safe_path=./$path
    if LC_ALL=C grep -Iq -- . "$safe_path" && \
        LC_ALL=C grep -Eq -- "$secret_pattern" "$safe_path"; then
        printf 'check-secrets: possible credential in %s (content redacted)\n' "$path" >&2
        failed=1
    fi
done

if ((failed != 0)); then
    exit 1
fi

printf '%s\n' 'check-secrets: SKIP VETTED (gitleaks unavailable; fallback passed)'
