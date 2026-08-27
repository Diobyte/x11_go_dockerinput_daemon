package vnext

// Backend is the X-facing destination actuator. Tests use Fake; the daemon
// may wrap warp/XTEST. Dest motion is XWarpPointer; FakeMotion is measurement-only.
// No XSendEvent of key/button/motion. No uinput. EWMH root ClientMessage is
// the window-client path.
type Backend interface {
	KeycodeFor(name string) uint
	WarpMouse(x, y int) error
	SendKey(keycode uint, press bool) error
	SendButton(button uint, press bool) error
	ScreenGeometry() (int, int, error)
	ManagedClients() ([]WindowInfo, error)
	Revalidate(ref WindowRef) error
	ActivateAlwaysBoth(ref WindowRef) error
	SetFullscreen(ref WindowRef, add bool) error
	ActiveWindow() (uint32, error)
}

// WindowInfo is inspect output for the dest client. Do not log Name/Class/PID/XID.
type WindowInfo struct {
	XID        uint32   `json:"xid"`
	Class      string   `json:"class,omitempty"`
	PID        uint32   `json:"pid,omitempty"`
	Name       string   `json:"name,omitempty"`
	X          int      `json:"x"`
	Y          int      `json:"y"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	State      []string `json:"state,omitempty"`
	DisplayGen uint64   `json:"displayGen"`
	ObserveGen uint64   `json:"observeGen"`
}
