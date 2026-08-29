# Protocol reference

## Recommended transport

Start the daemon with one private Unix listener:

```text
xtest-server \
  -vnext unix:/absolute/path/input.sock \
  -vnext-allow euid \
  -lock-file /absolute/path/authority.lock
```

The socket parent must already exist, be owned by the daemon user, and have mode
`0700`. The socket is created with mode `0600`. `-vnext-allow` accepts `euid` or
a comma-separated UID list. `-vnext-allow-gid` accepts `egid` or a
comma-separated GID list. UID admission is always required; the GID list adds a
second constraint when configured.

An explicit lock file must already exist beside the socket as a regular,
non-symlink file owned by the daemon user with mode `0600`. Its parent must be
owned by that user with mode `0700`. The daemon never creates, replaces, or
falls back from an explicit lock file. Separate containers targeting the same
underlying X server must mount the same host lock inode.

The daemon accepts at most ten simultaneous clients. Silent clients are closed
after the bounded idle period. A client sends one JSON object followed by `LF`
and receives one JSON object followed by `LF`. Frames larger than 1 MiB are
rejected.

On `SIGINT` or `SIGTERM`, the daemon stops accepting connections, closes every
active command connection, and attempts each session's held-input release once.
Shutdown waits at most five seconds. A mutation already in flight when shutdown
starts can have an ambiguous outcome and must not be replayed automatically.

## Outcomes

Every response contains `code`:

| Code | Meaning |
|---|---|
| `Submitted` | The validated operation was submitted to the X11 backend |
| `NotSubmitted` | The operation name is unknown and no mutation occurred |
| `InvalidRequest` | The request is malformed, incomplete, or contains invalid fields |
| `Conflict` | A window reference changed before mutation and no mutation occurred |
| `Unavailable` | The X11 backend or required window-manager capability is unavailable |

`Submitted` is not application-level acknowledgement. If a connection fails
after a mutation is sent but before its response arrives, the outcome is
ambiguous and the caller must not replay it automatically.

## JSON requests

Unknown fields, duplicate fields, trailing JSON values, and non-object values
are rejected. Each operation accepts only its documented fields.

### Inspect

```json
{"op":"inspect"}
```

The response includes root-screen geometry and, when the window manager
supports the required EWMH properties, managed windows:

```json
{
  "code": "Submitted",
  "screenWidth": 1280,
  "screenHeight": 720,
  "windows": [
    {
      "xid": 4194307,
      "class": "Example",
      "pid": 1234,
      "name": "Example window",
      "x": 0,
      "y": 0,
      "width": 1280,
      "height": 720,
      "displayGen": 1,
      "observeGen": 1
    }
  ]
}
```

Treat the returned identity fields as a unit. Activation and fullscreen
requests revalidate them immediately before mutation.

### Absolute pointer motion

```json
{"op":"move","x":640,"y":360}
```

Coordinates are root-screen X11 coordinates.

### Key transition

```json
{"op":"key","name":"Return","press":true}
```

`name` is resolved by the X server. Send a matching `press:false` transition,
or use `release` to release input held by the current connection.

### Mouse-button transition

```json
{"op":"button","button":1,"press":true}
```

Buttons 1 through 3 are accepted. Wheel transitions are intentionally absent
from this protocol version.

### Activate a window

```json
{
  "op": "activate",
  "displayGen": 1,
  "observeGen": 1,
  "xid": 4194307,
  "expectedClass": "Example",
  "expectedPID": 1234
}
```

The response may include `requestedActive` and `observedActive`. A mismatch is
reported without claiming that the target application accepted focus.

### Set fullscreen state

```json
{
  "op": "fullscreen",
  "displayGen": 1,
  "observeGen": 1,
  "xid": 4194307,
  "expectedClass": "Example",
  "expectedPID": 1234,
  "add": true
}
```

Set `add` to `false` to remove fullscreen state.

### Release held input

```json
{"op":"release"}
```

Only keycodes and buttons tracked for this connection are released. Closing a
connection performs the same session-scoped cleanup.

## Legacy line protocol

The compatibility protocol is available over stdin, TCP, or WebSocket. It is
unauthenticated and should not be exposed outside a trusted local boundary.

Supported commands are:

```text
keydown <keysym>
keyup <keysym>
key <keysym> <hold-ms>
mousemove <x> <y>
mousedown <button>
mouseup <button>
click <button>
modclick <modifier-keysym> <button> <x> <y> <hold-ms>
releaseall
```

The hold duration is bounded to 20 through 500 milliseconds. Unknown legacy
commands preserve compatibility by producing no mutation. TCP mode exposes a
read-only `GET /healthz` response and a WebSocket upgrade on the same listener.

The legacy transport tracks held input globally and releases it when a command
connection closes. Do not run it alongside the Unix JSON transport on the same
display.
