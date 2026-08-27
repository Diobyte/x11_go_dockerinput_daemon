package v2line

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

type recActuator struct {
	keys  map[string]uint
	calls []string
}

func (r *recActuator) KeycodeFor(name string) uint {
	if r.keys == nil {
		return 0
	}
	return r.keys[name]
}

func (r *recActuator) SendKey(keycode uint, press bool) {
	if press {
		r.calls = append(r.calls, "keydown:"+itoa(keycode))
		return
	}
	r.calls = append(r.calls, "keyup:"+itoa(keycode))
}

func (r *recActuator) MoveMouse(x, y int) {
	r.calls = append(r.calls, "move:"+itoa(uint(x))+","+itoa(uint(y)))
}

func (r *recActuator) SendButton(button uint, press bool) {
	if press {
		r.calls = append(r.calls, "btndown:"+itoa(button))
		return
	}
	r.calls = append(r.calls, "btnup:"+itoa(button))
}

func (r *recActuator) ReleaseAll() {
	r.calls = append(r.calls, "releaseall")
}

func itoa(v uint) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

func TestParseKeyHoldRequiresCurrentProtocol(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want time.Duration
		ok   bool
	}{
		{name: "minimum", cmd: "key F1 20", want: 20 * time.Millisecond, ok: true},
		{name: "normal", cmd: "key Escape 100", want: 100 * time.Millisecond, ok: true},
		{name: "maximum", cmd: "key F12 500", want: 500 * time.Millisecond, ok: true},
		{name: "retired missing hold", cmd: "key F1"},
		{name: "extra field", cmd: "key F1 100 ignored"},
		{name: "wrong operation", cmd: "keydown F1 100"},
		{name: "below minimum", cmd: "key F1 19"},
		{name: "above maximum", cmd: "key F1 501"},
		{name: "not a number", cmd: "key F1 fast"},
		{name: "noncanonical plus", cmd: "key F1 +100"},
		{name: "noncanonical leading zero", cmd: "key F1 0100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseKeyHold(strings.Fields(tt.cmd))
			if ok != tt.ok || got != tt.want {
				t.Fatalf("ParseKeyHold(%q) = %v, %v; want %v, %v", tt.cmd, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestDispatchRepresentativeLines(t *testing.T) {
	a := &recActuator{keys: map[string]uint{"F1": 67, "Shift_L": 50}}
	var logs []string
	logf := func(format string, _ ...any) {
		logs = append(logs, format)
	}

	Dispatch("keydown F1", a, logf)
	Dispatch("keyup F1", a, logf)
	Dispatch("key F1 20", a, logf)
	Dispatch("mousemove 1 1", a, logf)
	Dispatch("click 1", a, logf)
	Dispatch("mousedown 3", a, logf)
	Dispatch("mouseup 3", a, logf)
	Dispatch("releaseall", a, logf)
	Dispatch("not-a-command", a, logf)
	Dispatch("  keydown F1  extra", a, logf)

	want := []string{
		"keydown:67",
		"keyup:67",
		"keydown:67",
		"keyup:67",
		"move:1,1",
		"btndown:1",
		"btnup:1",
		"btndown:3",
		"btnup:3",
		"releaseall",
		"keydown:67",
	}
	if len(a.calls) != len(want) {
		t.Fatalf("calls=%v want=%v", a.calls, want)
	}
	for i := range want {
		if a.calls[i] != want[i] {
			t.Fatalf("call[%d]=%q want %q (all=%v)", i, a.calls[i], want[i], a.calls)
		}
	}
	if len(logs) != 0 {
		t.Fatalf("unexpected logs: %v", logs)
	}
}

func TestDispatchUnknownKeysymDoesNotLogName(t *testing.T) {
	a := &recActuator{}
	var logs []string
	sensitive := strings.Join([]string{"X11", "INPUT", "REDACT", "FIXTURE", "TOKEN"}, "_")
	Dispatch("keydown "+sensitive, a, func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})
	if len(a.calls) != 0 {
		t.Fatalf("unknown keysym must not send: %v", a.calls)
	}
	joined := strings.Join(logs, "\n")
	if strings.Contains(joined, sensitive) {
		t.Fatal("leak in dispatch log")
	}
}

func TestDispatchInvalidKeyDoesNotLogLine(t *testing.T) {
	a := &recActuator{keys: map[string]uint{"F1": 67}}
	var b strings.Builder
	sensitive := strings.Join([]string{"X11", "INPUT", "REDACT", "FIXTURE", "TOKEN"}, "_")
	Dispatch("key "+sensitive+" 19", a, func(format string, args ...any) {
		_, _ = fmt.Fprintf(&b, format, args...)
	})
	if strings.Contains(b.String(), sensitive) {
		t.Fatal("leak in invalid-key log")
	}
}
