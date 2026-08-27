package vnext

import (
	"encoding/json"
	"strings"
)

// Op is a destination verb. Unknown ops are NotSubmitted (never silent no-ops).
type Op string

const (
	// OpMove warps the pointer to an absolute coordinate.
	OpMove Op = "move"
	// OpKey changes one key state.
	OpKey Op = "key"
	// OpButton changes one pointer button state.
	OpButton Op = "button"
	// OpInspect reads the bounded window snapshot.
	OpInspect Op = "inspect"
	// OpActivate activates a validated window reference.
	OpActivate Op = "activate"
	// OpFullscreen changes fullscreen state for a validated window.
	OpFullscreen Op = "fullscreen"
	// OpRelease releases holds owned by the current session.
	OpRelease Op = "release"
)

// maxJSONLine is the dest and v2 scanner bound. Decode must not accept a
// larger in-process line if a caller skips the scanner.
const maxJSONLine = 1 << 20

// WindowRef is durable window identity. Empty class and PID 0 skip those checks.
type WindowRef struct {
	DisplayGen    uint64 `json:"displayGen"`
	ObserveGen    uint64 `json:"observeGen"`
	XID           uint32 `json:"xid"`
	ExpectedClass string `json:"expectedClass"`
	ExpectedPID   uint32 `json:"expectedPID"`
}

// Request is a fully-validated destination command. Decode never returns a
// Request that is only partially populated for mutation ops.
type Request struct {
	Op       Op
	X, Y     int
	Key      string
	Press    bool
	Button   uint
	Window   WindowRef
	AddState bool
}

type wire struct {
	Op            string  `json:"op"`
	X             *int    `json:"x"`
	Y             *int    `json:"y"`
	Name          string  `json:"name"`
	Press         *bool   `json:"press"`
	Button        *uint   `json:"button"`
	DisplayGen    *uint64 `json:"displayGen"`
	ObserveGen    *uint64 `json:"observeGen"`
	XID           *uint32 `json:"xid"`
	ExpectedClass string  `json:"expectedClass"`
	ExpectedPID   uint32  `json:"expectedPID"`
	Add           *bool   `json:"add"`
}

var knownWireKeys = map[string]struct{}{
	"op": {}, "x": {}, "y": {}, "name": {}, "press": {}, "button": {},
	"displayGen": {}, "observeGen": {}, "xid": {}, "expectedClass": {},
	"expectedPID": {}, "add": {},
}

// Decode parses one destination JSON line and validates every field required
// by the op before returning a Request. The backend is not consulted.
func Decode(line string) (Request, Outcome) {
	line = strings.TrimSpace(line)
	if line == "" || len(line) > maxJSONLine {
		return Request{}, invalidRequest()
	}
	bad, seen := scanTopLevelJSON(line)
	if bad {
		return Request{}, invalidRequest()
	}
	dec := json.NewDecoder(strings.NewReader(line))
	dec.DisallowUnknownFields()
	var w wire
	if err := dec.Decode(&w); err != nil {
		return Request{}, invalidRequest()
	}
	if dec.More() {
		return Request{}, invalidRequest()
	}
	off := int(dec.InputOffset())
	if off >= 0 && off < len(line) && strings.TrimSpace(line[off:]) != "" {
		return Request{}, invalidRequest()
	}
	op := Op(w.Op)
	switch op {
	case OpMove:
		if extraSeen(seen, "op", "x", "y") || w.X == nil || w.Y == nil {
			return Request{}, invalidRequest()
		}
		return Request{Op: OpMove, X: *w.X, Y: *w.Y}, submitted()
	case OpKey:
		if extraSeen(seen, "op", "name", "press") || w.Name == "" || w.Press == nil || strings.ContainsRune(w.Name, 0) {
			return Request{}, invalidRequest()
		}
		return Request{Op: OpKey, Key: w.Name, Press: *w.Press}, submitted()
	case OpButton:
		if extraSeen(seen, "op", "button", "press") || w.Button == nil || w.Press == nil || *w.Button < 1 || *w.Button > 3 {
			return Request{}, invalidRequest()
		}
		return Request{Op: OpButton, Button: *w.Button, Press: *w.Press}, submitted()
	case OpInspect:
		if extraSeen(seen, "op") {
			return Request{}, invalidRequest()
		}
		return Request{Op: OpInspect}, submitted()
	case OpActivate:
		ref, ok := windowFromWire(w)
		if !ok || extraSeen(seen, "op", "displayGen", "observeGen", "xid", "expectedClass", "expectedPID") {
			return Request{}, invalidRequest()
		}
		return Request{Op: OpActivate, Window: ref}, submitted()
	case OpFullscreen:
		ref, ok := windowFromWire(w)
		if !ok || w.Add == nil || extraSeen(seen, "op", "displayGen", "observeGen", "xid", "expectedClass", "expectedPID", "add") {
			return Request{}, invalidRequest()
		}
		return Request{Op: OpFullscreen, Window: ref, AddState: *w.Add}, submitted()
	case OpRelease:
		if extraSeen(seen, "op") {
			return Request{}, invalidRequest()
		}
		return Request{Op: OpRelease}, submitted()
	default:
		return Request{}, notSubmitted()
	}
}

func windowFromWire(w wire) (WindowRef, bool) {
	if w.DisplayGen == nil || w.ObserveGen == nil || w.XID == nil || *w.XID == 0 {
		return WindowRef{}, false
	}
	return WindowRef{
		DisplayGen:    *w.DisplayGen,
		ObserveGen:    *w.ObserveGen,
		XID:           *w.XID,
		ExpectedClass: w.ExpectedClass,
		ExpectedPID:   w.ExpectedPID,
	}, true
}

func extraSeen(seen map[string]struct{}, allowed ...string) bool {
	allow := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allow[a] = struct{}{}
	}
	for k := range seen {
		if _, ok := allow[k]; !ok {
			return true
		}
	}
	return false
}

// scanTopLevelJSON rejects non-objects, unknown exact keys, and duplicate names.
// Case-folded keys such as "OP" are unknown. JSON null extra keys still appear in seen.
func scanTopLevelJSON(line string) (bad bool, seen map[string]struct{}) {
	seen = make(map[string]struct{})
	dec := json.NewDecoder(strings.NewReader(line))
	tok, err := dec.Token()
	if err != nil {
		return true, seen
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return true, seen
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return true, seen
		}
		k, ok := kt.(string)
		if !ok {
			return true, seen
		}
		if _, known := knownWireKeys[k]; !known {
			return true, seen
		}
		if _, dup := seen[k]; dup {
			return true, seen
		}
		seen[k] = struct{}{}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return true, seen
		}
	}
	return false, seen
}
