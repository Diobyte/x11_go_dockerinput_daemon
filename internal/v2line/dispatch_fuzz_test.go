package v2line

import (
	"strings"
	"testing"
)

func FuzzDispatch(f *testing.F) {
	for _, s := range []string{
		"mousemove 1 2",
		"keydown F1",
		"keyup F1",
		"click 1",
		"releaseall",
		"not-a-command",
		"",
	} {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, cmd string) {
		// Timed "key" holds sleep; fuzz must not block on them.
		if strings.HasPrefix(strings.TrimSpace(cmd), "key ") {
			_, ok := ParseKeyHold(strings.Fields(cmd))
			if ok {
				return
			}
		}
		a := &recActuator{keys: map[string]uint{"F1": 67}}
		Dispatch(cmd, a, func(string, ...any) {})
	})
}
