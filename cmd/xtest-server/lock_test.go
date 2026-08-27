package main

import "testing"

func TestDisplayLockNameSanitizes(t *testing.T) {
	t.Setenv("DISPLAY", ":99")
	got := displayLockName()
	if got != "x11-input--99.lock" {
		t.Fatalf("got %q", got)
	}
	paths := lockPaths()
	if paths[0] != "/run/x11-input--99.lock" || paths[1] != "/tmp/x11-input--99.lock" {
		t.Fatalf("%v", paths)
	}
}
