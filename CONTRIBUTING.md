# Contributing

Issues and pull requests are welcome for correctness, portability,
documentation, tests, and narrowly scoped protocol improvements.

## Before changing behavior

Describe the affected transport and X11 surface, the failure being addressed,
the compatibility impact, and the rollback path. Security-sensitive defaults
must fail closed.

## Development checks

Install the dependencies listed in the README, then run:

```sh
make check
```

For parser, framing, or strict-decoding changes, also run:

```sh
make fuzz
```

For `Dockerfile` or `.dockerignore` changes, also run:

```sh
make docker-check
make docker-smoke # Linux only
```

Tests should cover invalid input, disconnects, partial writes, held-input
cleanup, and ambiguous outcomes where relevant. Never make a test pass by
retrying a mutation whose submission status is unknown.

## Pull requests

Keep commits focused, update the protocol documentation for externally visible
changes, and state which live X11 checks were run. A skipped integration test is
not equivalent to a pass.
