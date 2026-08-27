package main

import (
	"strings"
	"testing"
)

func TestStdinReadyLine(t *testing.T) {
	if stdinReadyLine() != "READY" {
		t.Fatalf("got %q", stdinReadyLine())
	}
}

func TestBuildIdentityDefaultsAreExplicit(t *testing.T) {
	for name, value := range map[string]string{"version": buildVersion, "revision": buildRevision, "dirty": buildDirty} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("%s build identity is empty", name)
		}
	}
}

func TestTCPReadyLine(t *testing.T) {
	got := tcpReadyLine(40731)
	if got != "READY tcp:40731" {
		t.Fatalf("got %q", got)
	}
}

func TestDestModeLineIsNotV2Ready(t *testing.T) {
	got := destModeLine("/run/xtest.sock")
	if got != "xtest-server: mode dest unix /run/xtest.sock" {
		t.Fatalf("got %q", got)
	}
	if len(got) >= 5 && got[:5] == "READY" {
		t.Fatal("dest mode line must not be a v2 READY handshake")
	}
}
