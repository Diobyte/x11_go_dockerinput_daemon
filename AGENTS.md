# Repository instructions

This repository owns a reusable X11/XTEST input daemon and its legacy Go
client. Keep the public surface narrow and the security model explicit.

## Change discipline

- Preserve one mutation authority per X11 display.
- Never retry a mutation after an ambiguous transport failure.
- Validate a complete request before dispatching its first input event.
- Keep frames, connections, waits, batches, and shutdown bounded.
- Keep Xlib calls serialized and treat fatal X I/O loss as daemon-fatal.
- Do not add shell execution, arbitrary commands, uinput, evdev, libei, or
  `XSendEvent` mutation paths.
- Do not log request payloads, Xauthority data, tokens, or credentials.
- Preserve per-session release semantics on shared displays.
- Update public documentation and tests with every contract change.

## Required checks

Run `make check` before handoff. Run `make fuzz` for parser or framing changes.
Report skipped live X11 tests as skipped, never as passing evidence.
