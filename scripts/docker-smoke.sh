#!/usr/bin/env bash
set -euo pipefail

fail() {
    printf 'docker-smoke: %s\n' "$1" >&2
    exit 1
}

if (($# != 1)) || [[ -z $1 ]]; then
    printf 'usage: %s IMAGE\n' "${0##*/}" >&2
    exit 2
fi

[[ $(uname -s) == Linux ]] || fail 'Linux is required'

for command_name in docker Xvfb python3 stat timeout; do
    command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
done

docker info >/dev/null 2>&1 || fail 'Docker daemon is unavailable'

image=$1
docker image inspect "$image" >/dev/null 2>&1 || fail "image is unavailable: $image"

run_uid=$(id -u)
run_gid=$(id -g)
((run_uid != 0)) || fail 'run this smoke test as a non-root user'

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/x11-input-docker-smoke.XXXXXXXX")
chmod 0700 "$work_dir"
runtime_dir=$work_dir/runtime
mkdir -m 0700 "$runtime_dir"

authority_lock=$runtime_dir/authority.lock
display_file=$work_dir/xvfb.display
xvfb_log=$work_dir/xvfb.log
second_stdout=$work_dir/second.stdout
second_stderr=$work_dir/second.stderr

(umask 077 && : >"$authority_lock")
: >"$display_file"
: >"$xvfb_log"
: >"$second_stdout"
: >"$second_stderr"
chmod 0600 "$authority_lock" "$display_file" "$xvfb_log" "$second_stdout" "$second_stderr"

suffix=${work_dir##*.}
first_name=x11-input-smoke-first-$suffix
second_name=x11-input-smoke-second-$suffix
first_id=
second_id=
xvfb_pid=

cleanup() {
    status=$?
    trap - EXIT
    set +e

    if [[ -n $first_id ]]; then
        timeout 10s docker rm --force "$first_id" >/dev/null 2>&1
    fi
    if [[ -n $second_id ]]; then
        timeout 10s docker rm --force "$second_id" >/dev/null 2>&1
    fi
    if [[ -n $xvfb_pid ]] && kill -0 "$xvfb_pid" 2>/dev/null; then
        kill "$xvfb_pid" 2>/dev/null
        wait "$xvfb_pid" 2>/dev/null
    fi

    rm -f -- \
        "$runtime_dir/input.sock" \
        "$authority_lock" \
        "$display_file" \
        "$xvfb_log" \
        "$second_stdout" \
        "$second_stderr"
    rmdir -- "$runtime_dir" 2>/dev/null
    rmdir -- "$work_dir" 2>/dev/null

    exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP

Xvfb -displayfd 3 -screen 0 1280x720x24 -nolisten tcp -ac \
    3>"$display_file" 2>"$xvfb_log" &
xvfb_pid=$!

display_number=
for _ in {1..100}; do
    if ! kill -0 "$xvfb_pid" 2>/dev/null; then
        printf '%s\n' 'docker-smoke: Xvfb exited during startup' >&2
        sed -n '1,20p' "$xvfb_log" >&2
        exit 1
    fi
    if IFS= read -r display_number <"$display_file" && [[ $display_number =~ ^[0-9]+$ ]]; then
        break
    fi
    display_number=
    sleep 0.05
done
[[ -n $display_number ]] || fail 'timed out waiting for private Xvfb display'

display=:$display_number
socket_path=$runtime_dir/input.sock

common_args=(
    --network none
    --read-only
    --cap-drop ALL
    --security-opt no-new-privileges=true
    --user "$run_uid:$run_gid"
    --env "DISPLAY=$display"
    --mount "type=bind,source=/tmp/.X11-unix,target=/tmp/.X11-unix,readonly"
    --mount "type=bind,source=$runtime_dir,target=/run/x11-input"
)

first_id=$(docker create --name "$first_name" "${common_args[@]}" "$image")
timeout 10s docker start "$first_id" >/dev/null || fail 'first container failed to start'

for _ in {1..100}; do
    if [[ $(docker inspect --format '{{.State.Running}}' "$first_id" 2>/dev/null) != true ]]; then
        printf '%s\n' 'docker-smoke: first container exited before becoming ready' >&2
        docker logs "$first_id" >&2 || true
        exit 1
    fi
    [[ -S $socket_path ]] && break
    sleep 0.05
done
[[ -S $socket_path ]] || fail 'timed out waiting for destination socket'

socket_mode=$(stat -c '%a' "$socket_path")
[[ $socket_mode == 600 ]] || fail "destination socket mode is $socket_mode, want 600"
socket_uid=$(stat -c '%u' "$socket_path")
[[ $socket_uid == "$run_uid" ]] || fail "destination socket UID is $socket_uid, want $run_uid"

second_id=$(docker create --name "$second_name" "${common_args[@]}" "$image")
timeout 10s docker start "$second_id" >"$second_stdout" 2>"$second_stderr" || {
    printf '%s\n' 'docker-smoke: second container failed to start' >&2
    sed -n '1,20p' "$second_stderr" >&2
    exit 1
}

if ! second_status=$(timeout 10s docker wait "$second_id"); then
    printf '%s\n' 'docker-smoke: timed out waiting for second container' >&2
    docker logs "$second_id" >&2 || true
    exit 1
fi
[[ $second_status == 75 ]] || {
    printf 'docker-smoke: second container exit=%s, want 75\n' "$second_status" >&2
    docker logs "$second_id" >&2 || true
    exit 1
}

[[ $(docker inspect --format '{{.State.Running}}' "$first_id") == true ]] || {
    printf '%s\n' 'docker-smoke: first container stopped during lock contention' >&2
    docker logs "$first_id" >&2 || true
    exit 1
}

python3 - "$socket_path" <<'PY'
import json
import socket
import sys

socket_path = sys.argv[1]
requests = (
    {"op": "move", "x": 1, "y": 1},
    {"op": "key", "name": "F1", "press": True},
    {"op": "key", "name": "F1", "press": False},
    {"op": "release"},
)

with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as connection:
    connection.settimeout(5)
    connection.connect(socket_path)
    for request in requests:
        encoded = json.dumps(request, separators=(",", ":")).encode("utf-8")
        connection.sendall(encoded + b"\n")

    reader = connection.makefile("rb")
    for index in range(len(requests)):
        line = reader.readline(1_048_577)
        if not line:
            raise SystemExit(f"docker-smoke: response {index + 1} is missing")
        if len(line) > 1_048_576:
            raise SystemExit(f"docker-smoke: response {index + 1} exceeds 1 MiB")
        try:
            response = json.loads(line)
        except json.JSONDecodeError as error:
            raise SystemExit(
                f"docker-smoke: response {index + 1} is not JSON: {error}"
            ) from error
        if response.get("code") != "Submitted":
            raise SystemExit(
                f"docker-smoke: response {index + 1} code is "
                f"{response.get('code')!r}, want 'Submitted'"
            )
PY

[[ $(docker inspect --format '{{.State.Running}}' "$first_id") == true ]] || {
    printf '%s\n' 'docker-smoke: first container stopped after requests' >&2
    docker logs "$first_id" >&2 || true
    exit 1
}

timeout 10s docker stop --time 7 "$first_id" >/dev/null || \
    fail 'first container did not stop within its shutdown bound'
first_status=$(docker inspect --format '{{.State.ExitCode}}' "$first_id")
[[ $first_status == 0 ]] || fail "first container exit=$first_status, want 0"
[[ ! -S $socket_path ]] || fail 'graceful shutdown left the destination socket'

timeout 10s docker start "$second_id" >/dev/null || \
    fail 'replacement container failed to start after lock release'
for _ in {1..100}; do
    if [[ $(docker inspect --format '{{.State.Running}}' "$second_id" 2>/dev/null) != true ]]; then
        printf '%s\n' 'docker-smoke: replacement container exited before becoming ready' >&2
        docker logs "$second_id" >&2 || true
        exit 1
    fi
    [[ -S $socket_path ]] && break
    sleep 0.05
done
[[ -S $socket_path ]] || fail 'timed out waiting for replacement socket'
timeout 10s docker stop --time 7 "$second_id" >/dev/null || \
    fail 'replacement container did not stop within its shutdown bound'
second_status=$(docker inspect --format '{{.State.ExitCode}}' "$second_id")
[[ $second_status == 0 ]] || fail "replacement container exit=$second_status, want 0"

printf '%s\n' \
    'docker-smoke: PASS (private Xvfb, four submissions, shared lock, bounded shutdown)'
