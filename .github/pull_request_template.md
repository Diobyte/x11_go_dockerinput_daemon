## Summary

- What changed?
- Why is this the smallest safe change?

## Security and compatibility

- Which transport, protocol, X11, or session-state contracts are affected?
- Can a failure leave input held or a mutation outcome ambiguous?
- Is rollback compatible with the previous binary and protocol?

## Verification

- [ ] `make check`
- [ ] `make fuzz` when parsing or framing changed
- [ ] Live X11 checks, or an explicit explanation of why they were unavailable
- [ ] No credentials, Xauthority data, or sensitive request payloads added
- [ ] Documentation updated for public behavior changes
