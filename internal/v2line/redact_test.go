package v2line

import (
	"strings"
	"testing"
)

func TestRedactLine(t *testing.T) {
	sensitive := strings.Join([]string{"X11", "INPUT", "REDACT", "FIXTURE", "TOKEN"}, "_")
	tests := []struct {
		in   string
		want string
	}{
		{"", "<empty>"},
		{"   ", "<empty>"},
		{"releaseall", "releaseall"},
		{"keydown F1", "keydown <redacted>"},
		{"key F1 100", "key <redacted>"},
		{"mousemove 640 360", "mousemove <redacted>"},
		{"modclick Shift_L 1 10 20 45", "modclick <redacted>"},
		{"keydown " + sensitive, "keydown <redacted>"},
		{sensitive, "<redacted>"},
		{"  " + sensitive + " leftover", "<redacted>"},
	}
	for _, tt := range tests {
		got := RedactLine(tt.in)
		if got != tt.want {
			if strings.Contains(tt.in, sensitive) || strings.Contains(got, sensitive) || strings.Contains(tt.want, sensitive) {
				t.Fatal("RedactLine mismatch on planted token")
			}
			t.Fatalf("RedactLine(%q)=%q want %q", tt.in, got, tt.want)
		}
		if strings.Contains(got, sensitive) {
			t.Fatal("RedactLine contained planted token")
		}
	}
}
