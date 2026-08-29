# X11 Input Daemon

A small Linux daemon for injecting keyboard, pointer, and window-manager
operations into one X11 display. It uses XTEST for keys and buttons,
`XWarpPointer` for absolute motion, and EWMH for window inspection, activation,
and fullscreen state.

The project is designed for containers, test rigs, kiosks, remote desktops, and
other environments where an application needs a narrow input service without
shell execution or host-wide input devices.

## Highlights

- One mutation authority per X11 `DISPLAY`, enforced by an advisory lock.
- Private Unix-socket transport with peer UID and optional GID admission.
- Strict newline-delimited JSON requests with a 1 MiB frame limit.
- Typed outcomes: `Submitted`, `NotSubmitted`, `InvalidRequest`, `Conflict`,
  and `Unavailable`.
- Per-connection tracking and release of held keys and mouse buttons.
- Window references are revalidated immediately before activation or
  fullscreen changes.
- No shell execution, arbitrary command execution, uinput, evdev, or
  `XSendEvent` mutation path.
- Legacy line, TCP, and WebSocket modes remain available for existing clients.
- Race, fuzz, fault, and optional Xvfb-backed integration tests.

## Requirements

- Linux with an Xorg-compatible X11 server and the XTEST extension.
- Go 1.26 or newer.
- A C compiler plus X11 and Xtst development headers.
- Xvfb and xauth for the optional live integration tests.

On Debian or Ubuntu:

```sh
sudo apt-get update
sudo apt-get install --no-install-recommends \
  build-essential libx11-dev libxtst-dev xvfb xauth
```

## Build

```sh
git clone https://github.com/Diobyte/x11_go_dockerinput_daemon.git
cd x11_go_dockerinput_daemon
make check
go build -o bin/xtest-server ./cmd/xtest-server
```

Release builds can embed their identity:

```sh
go build -trimpath \
  -ldflags "-X main.buildVersion=v0.1.0 -X main.buildRevision=$(git rev-parse HEAD) -X main.buildDirty=false" \
  -o bin/xtest-server ./cmd/xtest-server
```

Inspect the embedded identity with `bin/xtest-server -version`.

## Container image

The image contains the daemon and its runtime X11 libraries. It does not start
an X server, window manager, VNC server, screen stream, or web interface. The
target Xorg, Xvfb, or Xephyr server remains outside the container. Host
Xwayland remains intentionally unsupported.

Build and inspect the image with:

```sh
make docker-build
make docker-check
make docker-smoke # Linux, Docker, Xvfb, and Python 3
```

The image runs as a non-root user, exposes no ports, and fixes its entrypoint to
the private Unix JSON mode with an explicit shared lock file. It fails before
opening the X display when that pre-created lock file is absent. Choose one
stable private runtime directory for each underlying X server, and reuse it for
every container that could target that server.

To use the numeric UID of the target X session, override the image user and
bind a runtime directory owned by that UID. Create a dedicated Xauthority file
rather than mounting the session owner's complete authority file:

```sh
: "${XDG_RUNTIME_DIR:?XDG_RUNTIME_DIR must be set}"
runtime_dir="$XDG_RUNTIME_DIR/x11-input-daemon"
install -d -m 0700 "$runtime_dir"
(umask 077; : >> "$runtime_dir/authority.lock")
chmod 0600 "$runtime_dir/authority.lock"

auth_file="$(mktemp)"
chmod 0600 "$auth_file"
cleanup() {
  rm -f -- "$auth_file"
}
trap cleanup EXIT
xauth nlist "$DISPLAY" |
  sed -e 's/^..../ffff/' |
  xauth -f "$auth_file" nmerge -

docker run --rm --name x11-input-daemon \
  --network none \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges=true \
  --stop-timeout 7 \
  --user "$(id -u):$(id -g)" \
  --env DISPLAY \
  --mount type=bind,source=/tmp/.X11-unix,target=/tmp/.X11-unix,readonly \
  --mount type=bind,source="$auth_file",target=/run/secrets/xauthority,readonly \
  --mount type=bind,source="$runtime_dir",target=/run/x11-input \
  x11-input-daemon:local
```

The daemon socket is available on the host at
`$runtime_dir/input.sock`. Do not publish a legacy TCP or WebSocket listener
from the image.

All containers targeting the same underlying X server must mount the same
host `authority.lock` inode and run with the same numeric UID. Keep the lock on
a local filesystem with reliable `flock` behavior, and never replace it while a
daemon is running. A differently configured lock file cannot coordinate, so
the deployment must still define exactly one shared authority path per server.

On `SIGINT` or `SIGTERM`, Unix JSON mode stops admission, closes active
connections, attempts each session's held-input release once, and waits at most
five seconds. An in-flight mutation can still have an ambiguous outcome and
must not be replayed automatically.

## Quick start: private Unix socket

The Unix transport is the recommended mode. Its parent directory must be owned
by the daemon user and have mode `0700`.

```sh
runtime_dir="/run/user/$(id -u)/x11-input"
install -d -m 0700 "$runtime_dir"
(umask 077; : >> "$runtime_dir/authority.lock")
chmod 0600 "$runtime_dir/authority.lock"
DISPLAY=:99 ./bin/xtest-server \
  -vnext "unix:$runtime_dir/input.sock" \
  -vnext-allow euid \
  -lock-file "$runtime_dir/authority.lock"
```

Send one JSON object per line and read one JSON result per line. For example,
with `socat`:

```sh
printf '%s\n' '{"op":"inspect"}' |
  socat - "UNIX-CONNECT:$runtime_dir/input.sock"
```

```json
{"code":"Submitted","screenWidth":1280,"screenHeight":720}
```

See [the protocol reference](docs/PROTOCOL.md) for every request, response,
validation rule, and transport boundary.

## Operating modes

| Mode | Invocation | Intended use |
|---|---|---|
| Private Unix JSON | `-vnext unix:/absolute/path -vnext-allow euid` | Recommended local/container integration |
| Standard input | no listener flag | Supervised process with a private stdin pipe |
| Legacy TCP | `-tcp 127.0.0.1:9999` | Compatibility on a trusted local network namespace |
| Legacy WebSocket | `-ws 127.0.0.1:9999` | Existing WebSocket clients |

Listener modes are mutually exclusive. The daemon refuses to start a Unix JSON
listener alongside TCP or WebSocket input.

The public `x11input` Go package is the compatibility client for the legacy
line protocol:

```sh
go get github.com/Diobyte/x11_go_dockerinput_daemon/x11input
```

The JSON protocol intentionally remains a small wire contract rather than a
large exported Go object model, making it straightforward to consume from any
language.

## Security model

Input injection is security-sensitive. Run the daemon as the same unprivileged
user that owns the target X session, keep the socket directory private, and
allow only the UID or GIDs that require access. Possession of the Xauthority
cookie already grants broad X11 capabilities; this daemon does not turn X11
into a sandbox.

The legacy TCP and WebSocket listeners do not authenticate clients. Bind them
only to loopback or an isolated container network. Never publish their ports to
an untrusted host or LAN.

The daemon rejects host Xwayland displays to avoid accidentally requesting
seat-global remote-desktop authority. Native Wayland and host-global input
injection are intentionally unsupported.

Read [SECURITY.md](SECURITY.md) before deploying the service.

## Correctness boundaries

- A successful XTEST/Xlib submission is not proof that an application consumed
  the event.
- Mutating requests are never automatically replayed after an ambiguous
  connection failure.
- Disconnect cleanup releases only keys and buttons tracked for that client.
- Unix JSON shutdown is bounded and never retries a failed session release.
- Window names, classes, process IDs, and XIDs may be sensitive operational
  data and should not be logged indiscriminately.

## Development

```sh
make check
make fuzz
make docker-check # for Dockerfile or .dockerignore changes
make docker-smoke # Linux container/Xvfb integration
```

`make check` validates documentation and secrets, verifies formatting and the
module graph, builds every package, runs `go vet`, and runs the complete test
suite with the race detector. Live X11 tests use a private Xvfb display when it
is available and report a skip when the required X server is absent.

Contribution guidelines are in [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
