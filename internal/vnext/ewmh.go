package vnext

// EWMH/X constants used by the destination window-client.
// Values match X.h / EWMH source indication for pagers.
const (
	SubstructureNotifyMask   = 1 << 19
	SubstructureRedirectMask = 1 << 20
	ActivateEventMask        = SubstructureRedirectMask | SubstructureNotifyMask
	NetActiveWindowSource    = 2 // EWMH pager source indication
	RevertToParent           = 2
	NetActiveWindowAtom      = "_NET_ACTIVE_WINDOW"
	NetWMStateAtom           = "_NET_WM_STATE"
	NetWMStateFullscreenAtom = "_NET_WM_STATE_FULLSCREEN"
	NetClientListAtom        = "_NET_CLIENT_LIST"
	NetSupportedAtom         = "_NET_SUPPORTED"
	NetWMNameAtom            = "_NET_WM_NAME"
	NetWMPIDAtom             = "_NET_WM_PID"
	NetWMStateAdd            = 1
	NetWMStateRemove         = 0
)
