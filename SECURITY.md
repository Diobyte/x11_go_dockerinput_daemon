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
