package v2line

import "strings"

var knownOps = map[string]struct{}{
	"keydown":    {},
	"keyup":      {},
	"key":        {},
	"mousemove":  {},
	"mousedown":  {},
	"mouseup":    {},
	"click":      {},
	"modclick":   {},
	"releaseall": {},
}

// RedactLine returns a log token for a v2 command that cannot reconstruct
// typed text, credentials, or other payloads. Unknown first fields
// are fully redacted so a secret-only line cannot leak as an "opcode".
func RedactLine(cmd string) string {
	parts := strings.Fields(strings.TrimSpace(cmd))
	if len(parts) == 0 {
		return "<empty>"
	}
	op := parts[0]
	if _, ok := knownOps[op]; !ok {
		return "<redacted>"
	}
	if len(parts) == 1 {
		return op
	}
	return op + " <redacted>"
}
