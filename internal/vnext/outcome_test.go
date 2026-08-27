package vnext

import (
	"strings"
	"testing"
)

func TestOutcomeWireJSONInspectAndActivate(t *testing.T) {
	insp := Outcome{
		Code: CodeSubmitted,
		Windows: []WindowInfo{{
			XID: 42, Class: "game", PID: 9, Name: "win",
			X: 10, Y: 20, Width: 30, Height: 40,
			State:      []string{NetWMStateFullscreenAtom},
			DisplayGen: 1, ObserveGen: 2,
		}},
	}
	raw, err := insp.WireJSON()
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, need := range []string{
		`"code":"Submitted"`, `"windows":`, `"xid":42`, `"class":"game"`,
		`"pid":9`, `"name":"win"`, `"x":10`, `"y":20`, `"width":30`, `"height":40`,
		`"_NET_WM_STATE_FULLSCREEN"`, `"displayGen":1`, `"observeGen":2`,
	} {
		if !strings.Contains(s, need) {
			t.Fatalf("inspect wire missing field")
		}
	}
	if strings.Contains(s, `"Err"`) || strings.Contains(s, `"error"`) {
		t.Fatalf("wire leaked Err: %s", s)
	}

	act := Outcome{Code: CodeSubmitted, RequestedActive: 42, ObservedActive: 42}
	raw, err = act.WireJSON()
	if err != nil {
		t.Fatal(err)
	}
	s = string(raw)
	if !strings.Contains(s, `"requestedActive":42`) || !strings.Contains(s, `"observedActive":42`) {
		t.Fatalf("activate wire %s", s)
	}

	failed := Outcome{Code: CodeUnavailable, Err: errString("backend failed once")}
	raw, err = failed.WireJSON()
	if err != nil {
		t.Fatal(err)
	}
	s = string(raw)
	if strings.Contains(s, "backend failed") || strings.Contains(s, `"Err"`) || strings.Contains(s, `"error"`) {
		t.Fatalf("unavailable wire leaked err: %s", s)
	}
}
