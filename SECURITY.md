# Security policy

## Supported versions

Security fixes are applied to the latest release and the default branch. Older
releases may require an upgrade to receive a fix.

## Reporting a vulnerability

Use GitHub's private security-advisory feature for vulnerabilities. Do not open
a public issue for a flaw that could expose an X11 session, permit unauthorized
input, leak request data, or leave held input after disconnect.

Include the affected version or commit, transport mode, operating system,
reproduction steps, expected behavior, and observed behavior. Remove
credentials, Xauthority cookies, window titles, and application data from all
attachments.

## Deployment boundary

The recommended Unix listener authenticates peers by operating-system
credentials and requires a private parent directory. The legacy TCP and
WebSocket listeners are unauthenticated compatibility interfaces and must stay
on loopback or an isolated network namespace.

X11 is not a security sandbox. Any process holding the same Xauthority cookie
may be able to observe or inject events independently of this daemon.

Container deployments must pre-create one mode-`0600` lock file inside a
mode-`0700` runtime directory and pass it with `-lock-file`. Every daemon that
could target the same underlying X server must mount the same host lock inode
and run with the same numeric UID. Do not place the lock on a filesystem whose
`flock` behavior is uncertain, and do not replace it while the daemon runs.

Unix JSON mode handles `SIGINT` and `SIGTERM` with a five-second shutdown
bound. It stops admission, closes command connections, and attempts each
session's held-input release once. It never retries an ambiguous mutation or a
failed release.
