package vnext

import (
	"errors"
	"os"
	"testing"
)

func TestAdmitFlagsEmptyDefaultsV2(t *testing.T) {
	mode, addr, err := AdmitFlags("", "", "")
	if err != nil || mode != ModeV2 || addr != "" {
		t.Fatalf("empty flags: mode=%v addr=%q err=%v", mode, addr, err)
	}
}

func TestAdmitFlagsWSAliasesTCP(t *testing.T) {
	_, _, err := AdmitFlags("", "127.0.0.1:9999", "unix:/run/xtest.sock")
	if !errors.Is(err, ErrDualBind) {
		t.Fatalf("ws+vnext dual bind: %v", err)
	}
	mode, addr, err := AdmitFlags("", "127.0.0.1:0", "")
	if err != nil || mode != ModeV2 || addr != "127.0.0.1:0" {
		t.Fatalf("ws-only: mode=%v addr=%q err=%v", mode, addr, err)
	}
}

func TestSelectModeXOR(t *testing.T) {
	_, _, err := SelectMode("127.0.0.1:9999", "unix:/tmp/vnext.sock")
	if !errors.Is(err, ErrDualBind) {
		t.Fatalf("dual bind: %v", err)
	}
	mode, addr, err := SelectMode("127.0.0.1:0", "")
	if err != nil || mode != ModeV2 || addr != "127.0.0.1:0" {
		t.Fatalf("v2: mode=%v addr=%q err=%v", mode, addr, err)
	}
	mode, addr, err = SelectMode("", "unix:/run/xtest.sock")
	if err != nil || mode != ModeVNext || addr != "/run/xtest.sock" {
		t.Fatalf("vnext: mode=%v addr=%q err=%v", mode, addr, err)
	}
}

func TestParseUnixListenRejectsPlaintext(t *testing.T) {
	bads := []string{
		"", "tcp:127.0.0.1:9", ":9999", "0.0.0.0:9", "unix:", "unix:rel",
		"unix:tcp:9", "unix://tmp/x", "unix:*", "unix:0.0.0.0:9",
		"unix:/tmp/../run/x", "unix:/tmp/foo\nbar", "unix:/tmp/foo\x00bar",
	}
	for _, s := range bads {
		if _, err := ParseUnixListen(s); !errors.Is(err, ErrNotUnix) {
			t.Fatalf("%q: %v", s, err)
		}
	}
	got, err := ParseUnixListen("unix:/var/run/xtest.sock")
	if err != nil || got != "/var/run/xtest.sock" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestParseAllowlist(t *testing.T) {
	if _, err := ParseAllowlist(""); !errors.Is(err, ErrEmptyAllowlist) {
		t.Fatalf("empty: %v", err)
	}
	got, err := ParseAllowlist("euid")
	wantUID, wantErr := processID(os.Geteuid())
	if err != nil || wantErr != nil || len(got) != 1 || got[0] != wantUID {
		t.Fatalf("euid %v %v", got, err)
	}
	got, err = ParseAllowlist("1000,1001")
	if err != nil || len(got) != 2 || got[0] != 1000 || got[1] != 1001 {
		t.Fatalf("list %v %v", got, err)
	}
	if _, err := ParseAllowlist("nope"); !errors.Is(err, ErrBadAllowlist) {
		t.Fatalf("bad: %v", err)
	}
}

func TestParseGIDAllowlist(t *testing.T) {
	got, err := ParseGIDAllowlist("")
	if err != nil || got != nil {
		t.Fatalf("empty gid: %v %v", got, err)
	}
	got, err = ParseGIDAllowlist("egid")
	wantGID, wantErr := processID(os.Getegid())
	if err != nil || wantErr != nil || len(got) != 1 || got[0] != wantGID {
		t.Fatalf("egid %v %v", got, err)
	}
	got, err = ParseGIDAllowlist("10,20")
	if err != nil || len(got) != 2 || got[0] != 10 || got[1] != 20 {
		t.Fatalf("list %v %v", got, err)
	}
}
