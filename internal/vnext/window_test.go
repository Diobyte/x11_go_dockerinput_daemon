package vnext

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestInspectUsesManagedListNotQueryTree(t *testing.T) {
	f := NewFake()
	f.Managed = []WindowInfo{{XID: 42, Class: "game", PID: 9, Name: "win"}}
	out := HandleLine(`{"op":"inspect"}`, NewSession(), f)
	if !out.Submitted() {
		t.Fatalf("code %s", out.Code)
	}
	if f.QueryTree != 0 {
		t.Fatal("inspect walked QueryTree")
	}
	got := f.Snapshot()
	if len(got) != 1 || got[0] != "inspect:"+NetClientListAtom {
		t.Fatalf("inspect calls %v", got)
	}
	if len(out.Windows) != 1 || out.Windows[0].XID != 42 || out.Windows[0].DisplayGen != 1 {
		t.Fatalf("inspect window count=%d xid_ok=%t gen_ok=%t", len(out.Windows), len(out.Windows) == 1 && out.Windows[0].XID == 42, len(out.Windows) == 1 && out.Windows[0].DisplayGen == 1)
	}
	if out.ScreenWidth != 1280 || out.ScreenHeight != 720 {
		t.Fatalf("screen geometry %dx%d", out.ScreenWidth, out.ScreenHeight)
	}
}

func TestInspectPayloadIncludesClassPIDNameGeometryState(t *testing.T) {
	f := NewFake()
	f.Managed = []WindowInfo{{
		XID: 42, Class: "game", PID: 9, Name: "win",
		X: 1, Y: 2, Width: 3, Height: 4,
		State:      []string{NetWMStateFullscreenAtom},
		DisplayGen: 1, ObserveGen: 7,
	}}
	out := HandleLine(`{"op":"inspect"}`, NewSession(), f)
	if !out.Submitted() || len(out.Windows) != 1 {
		t.Fatalf("inspect code %s n=%d", out.Code, len(out.Windows))
	}
	w := out.Windows[0]
	if w.XID != 42 || w.Class != "game" || w.PID != 9 || w.Name != "win" ||
		w.X != 1 || w.Y != 2 || w.Width != 3 || w.Height != 4 ||
		len(w.State) != 1 || w.State[0] != NetWMStateFullscreenAtom ||
		w.DisplayGen != 1 || w.ObserveGen != 7 {
		t.Fatalf("inspect field mismatch xid=%d class_ok=%t pid=%d", w.XID, w.Class == "game", w.PID)
	}
	raw, err := out.WireJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"screenWidth":1280`) || !strings.Contains(string(raw), `"screenHeight":720`) {
		t.Fatalf("inspect wire missing screen geometry: %s", raw)
	}
}

func TestActivateAlwaysBoth(t *testing.T) {
	f := NewFake()
	f.Managed = []WindowInfo{{XID: 42, Class: "game", PID: 9}}
	line, err := json.Marshal(map[string]any{
		"op": "activate", "displayGen": 1, "observeGen": 1, "xid": 42,
		"expectedClass": "game", "expectedPID": 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := HandleLine(string(line), NewSession(), f)
	if !out.Submitted() {
		t.Fatalf("code %s", out.Code)
	}
	got := strings.Join(f.Snapshot(), ";")
	if !strings.Contains(got, NetActiveWindowAtom) || !strings.Contains(got, fmt.Sprintf("source=%d", NetActiveWindowSource)) {
		t.Fatal("activate missing _NET_ACTIVE_WINDOW source")
	}
	if !strings.Contains(got, fmt.Sprintf("mask=%d", ActivateEventMask)) {
		t.Fatal("activate missing redirect+notify mask")
	}
	if !strings.Contains(got, fmt.Sprintf("setinputfocus:revert=%d", RevertToParent)) {
		t.Fatal("activate missing SetInputFocus RevertToParent")
	}
	if ActivateEventMask != SubstructureRedirectMask|SubstructureNotifyMask {
		t.Fatal("activate mask is not redirect+notify")
	}
	if strings.Index(got, NetActiveWindowAtom) > strings.Index(got, "setinputfocus:") {
		t.Fatal("SetInputFocus ran before _NET_ACTIVE_WINDOW")
	}
	if out.RequestedActive != 42 || out.ObservedActive != 42 {
		t.Fatalf("requested/observed %d/%d", out.RequestedActive, out.ObservedActive)
	}
}

func TestFullscreenRecordsNetWMState(t *testing.T) {
	f := NewFake()
	f.Managed = []WindowInfo{{XID: 42}}
	line := `{"op":"fullscreen","displayGen":1,"observeGen":1,"xid":42,"add":true}`
	out := HandleLine(line, NewSession(), f)
	if !out.Submitted() {
		t.Fatalf("code %s", out.Code)
	}
	got := strings.Join(f.Snapshot(), " ")
	if !strings.Contains(got, NetWMStateFullscreenAtom) || !strings.Contains(got, ":add:") {
		t.Fatal("fullscreen missing _NET_WM_STATE_FULLSCREEN add")
	}
	if !strings.Contains(got, fmt.Sprintf("mask=%d", ActivateEventMask)) {
		t.Fatal("fullscreen missing same root event mask")
	}
}

func TestFullscreenRemoveRecordsNetWMState(t *testing.T) {
	f := NewFake()
	f.Managed = []WindowInfo{{XID: 42}}
	line := `{"op":"fullscreen","displayGen":1,"observeGen":1,"xid":42,"add":false}`
	out := HandleLine(line, NewSession(), f)
	if !out.Submitted() {
		t.Fatalf("code %s", out.Code)
	}
	got := strings.Join(f.Snapshot(), " ")
	if !strings.Contains(got, NetWMStateFullscreenAtom) || !strings.Contains(got, ":remove:") {
		t.Fatal("fullscreen missing _NET_WM_STATE_FULLSCREEN remove")
	}
}

func TestInspectRequiresNetClientListOnSupported(t *testing.T) {
	f := NewFake()
	f.Managed = []WindowInfo{{XID: 42}}
	f.Supported = []string{NetActiveWindowAtom}
	out := HandleLine(`{"op":"inspect"}`, NewSession(), f)
	if out.Code != CodeUnavailable {
		t.Fatalf("code %s", out.Code)
	}
	if len(f.Snapshot()) != 0 {
		t.Fatal("inspect mutated without _NET_CLIENT_LIST")
	}
}

func TestUnsupportedAtomIsUnavailable(t *testing.T) {
	f := NewFake()
	f.Managed = []WindowInfo{{XID: 42}}
	f.Supported = []string{NetWMStateAtom}
	out := HandleLine(`{"op":"activate","displayGen":1,"observeGen":1,"xid":42}`, NewSession(), f)
	if out.Code != CodeUnavailable {
		t.Fatalf("got %q want %s", out.Code, CodeUnavailable)
	}
	if len(f.Snapshot()) != 0 {
		t.Fatalf("unsupported activate mutated: %v", f.Snapshot())
	}
}

func TestStaleWindowRefIsConflictZeroMutation(t *testing.T) {
	f := NewFake()
	f.Managed = []WindowInfo{{XID: 42, Class: "game", PID: 9}}
	cases := []struct {
		name string
		line string
	}{
		{"stale displayGen", `{"op":"activate","displayGen":99,"observeGen":1,"xid":42}`},
		{"xid mismatch", `{"op":"activate","displayGen":1,"observeGen":1,"xid":99}`},
		{"class mismatch", `{"op":"activate","displayGen":1,"observeGen":1,"xid":42,"expectedClass":"other"}`},
		{"pid mismatch", `{"op":"fullscreen","displayGen":1,"observeGen":1,"xid":42,"expectedPID":8,"add":true}`},
	}
	for _, tc := range cases {
		f.Calls = nil
		out := HandleLine(tc.line, NewSession(), f)
		if out.Code != CodeConflict {
			t.Fatalf("%s: got %q", tc.name, out.Code)
		}
		for _, c := range f.Snapshot() {
			if strings.HasPrefix(c, "clientmessage:") || strings.HasPrefix(c, "setinputfocus:") {
				t.Fatalf("%s: mutated on conflict", tc.name)
			}
		}
	}
}

func TestActivateDoesNotRetarget(t *testing.T) {
	f := NewFake()
	f.Managed = []WindowInfo{{XID: 7, Class: "game"}, {XID: 8, Class: "game"}}
	out := HandleLine(`{"op":"activate","displayGen":1,"observeGen":1,"xid":99,"expectedClass":"game"}`, NewSession(), f)
	if out.Code != CodeConflict {
		t.Fatalf("got %q", out.Code)
	}
	if len(f.Snapshot()) != 0 {
		t.Fatal("implicit retarget")
	}
}
