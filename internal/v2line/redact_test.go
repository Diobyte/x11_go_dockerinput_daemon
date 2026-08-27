package v2line

import (
	"strings"
	"testing"
)

func TestRedactLine(t *testing.T) {
	secret := "X11_INPUT_REDACT_FIXTURE_TOKEN"
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
		{"keydown " + secret, "keydown <redacted>"},
		{secret, "<redacted>"},
		{"  " + secret + " leftover", "<redacted>"},
	}
	for _, tt := range tests {
		got := RedactLine(tt.in)
		if got != tt.want {
			if strings.Contains(tt.in, secret) || strings.Contains(got, secret) || strings.Contains(tt.want, secret) {
				t.Fatal("RedactLine mismatch on planted token")
			}
			t.Fatalf("RedactLine(%q)=%q want %q", tt.in, got, tt.want)
		}
		if strings.Contains(got, secret) {
			t.Fatal("RedactLine contained planted token")
		}
	}
}
