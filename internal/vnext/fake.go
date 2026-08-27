package vnext

import (
	"errors"
	"fmt"
	"sync"
)

// ErrRefMismatch is returned by Fake.Revalidate when WindowRef does not match.
var ErrRefMismatch = errors.New("vnext: window ref mismatch")

// ErrUnavailable is window-client not ready (missing _NET_SUPPORTED or X down).
var ErrUnavailable = errors.New("vnext: window-client not ready")

// Fake is a recording destination backend. Tests must drive HandleLine/Dispatch
// against this type — it is the shipped fake, not a test-only reimplementation.
type Fake struct {
	mu        sync.Mutex
	keys      map[string]uint
	Calls     []string
	Managed   []WindowInfo
	Display   uint64
	Observe   uint64
	QueryTree int
	// Supported is the fake _NET_SUPPORTED list. nil means the mutate
	// atoms are present. A non-nil empty slice is window-not-ready.
	Supported []string
	ScreenW   int
	ScreenH   int
	active    uint32
}

func NewFake() *Fake {
	return &Fake{
		keys:    map[string]uint{"F1": 67, "Shift_L": 50},
		Display: 1,
		Observe: 1,
		ScreenW: 1280,
		ScreenH: 720,
	}
}

func (f *Fake) ScreenGeometry() (int, int, error) {
	if f.ScreenW <= 0 || f.ScreenH <= 0 {
		return 0, 0, ErrUnavailable
	}
	return f.ScreenW, f.ScreenH, nil
}

func (f *Fake) record(s string) {
	f.mu.Lock()
	f.Calls = append(f.Calls, s)
	f.mu.Unlock()
}

func (f *Fake) Snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.Calls))
	copy(out, f.Calls)
	return out
}

func (f *Fake) KeycodeFor(name string) uint {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.keys == nil {
		return 0
	}
	return f.keys[name]
}

func (f *Fake) WarpMouse(x, y int) error {
	f.record(fmt.Sprintf("warp:%d,%d", x, y))
	return nil
}

func (f *Fake) SendKey(keycode uint, press bool) error {
	dir := "up"
	if press {
		dir = "down"
	}
	f.record(fmt.Sprintf("key:%d:%s", keycode, dir))
	return nil
}

func (f *Fake) SendButton(button uint, press bool) error {
	dir := "up"
	if press {
		dir = "down"
	}
	f.record(fmt.Sprintf("button:%d:%s", button, dir))
	return nil
}

func (f *Fake) ManagedClients() ([]WindowInfo, error) {
	if !f.atomOK(NetClientListAtom) {
		return nil, ErrUnavailable
	}
	f.record("inspect:" + NetClientListAtom)
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]WindowInfo, len(f.Managed))
	copy(out, f.Managed)
	for i := range out {
		if out[i].DisplayGen == 0 {
			out[i].DisplayGen = f.Display
		}
		if out[i].ObserveGen == 0 {
			out[i].ObserveGen = f.Observe
		}
	}
	return out, nil
}

func (f *Fake) Revalidate(ref WindowRef) error {
	if !f.atomOK(NetClientListAtom) {
		return ErrUnavailable
	}
	f.mu.Lock()
	display, observe := f.Display, f.Observe
	f.mu.Unlock()
	if ref.XID == 0 {
		return ErrRefMismatch
	}
	if ref.DisplayGen != display || ref.ObserveGen != observe {
		return ErrRefMismatch
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.Managed {
		if w.XID != ref.XID {
			continue
		}
		if ref.ExpectedClass != "" && w.Class != ref.ExpectedClass {
			return ErrRefMismatch
		}
		if ref.ExpectedPID != 0 && w.PID != ref.ExpectedPID {
			return ErrRefMismatch
		}
		return nil
	}
	return ErrRefMismatch
}

func (f *Fake) ActivateAlwaysBoth(ref WindowRef) error {
	if !f.atomOK(NetActiveWindowAtom) {
		return ErrUnavailable
	}
	f.record(fmt.Sprintf("clientmessage:%s:source=%d:mask=%d:xid=%d",
		NetActiveWindowAtom, NetActiveWindowSource, ActivateEventMask, ref.XID))
	f.record(fmt.Sprintf("setinputfocus:revert=%d:xid=%d", RevertToParent, ref.XID))
	f.mu.Lock()
	f.active = ref.XID
	f.mu.Unlock()
	return nil
}

func (f *Fake) ActiveWindow() (uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active, nil
}

func (f *Fake) SetFullscreen(ref WindowRef, add bool) error {
	if !f.atomOK(NetWMStateAtom) || !f.atomOK(NetWMStateFullscreenAtom) {
		return ErrUnavailable
	}
	action := "remove"
	if add {
		action = "add"
	}
	f.record(fmt.Sprintf("clientmessage:%s:%s:%s:mask=%d:xid=%d",
		NetWMStateAtom, NetWMStateFullscreenAtom, action, ActivateEventMask, ref.XID))
	return nil
}

// WalkQueryTree is not part of Backend. Calling it from tests proves inspect
// did not go through root QueryTree.
func (f *Fake) WalkQueryTree() {
	f.mu.Lock()
	f.QueryTree++
	f.mu.Unlock()
}

func (f *Fake) atomOK(atom string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Supported == nil {
		return true
	}
	for _, a := range f.Supported {
		if a == atom {
			return true
		}
	}
	return false
}
