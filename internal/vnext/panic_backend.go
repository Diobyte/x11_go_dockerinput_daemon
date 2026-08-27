package vnext

// PanicBackend fails the test process if mutation is attempted. Used to prove
// invalid/unknown HandleLine never reaches a backend.
type PanicBackend struct{}

// KeycodeFor panics on lookup.
func (PanicBackend) KeycodeFor(string) uint { panic("vnext: backend KeycodeFor") }

// WarpMouse panics on pointer mutation.
func (PanicBackend) WarpMouse(int, int) error { panic("vnext: backend WarpMouse") }

// SendKey panics on key mutation.
func (PanicBackend) SendKey(uint, bool) error { panic("vnext: backend SendKey") }

// SendButton panics on button mutation.
func (PanicBackend) SendButton(uint, bool) error { panic("vnext: backend SendButton") }

// ScreenGeometry panics on display inspection.
func (PanicBackend) ScreenGeometry() (int, int, error) {
	panic("vnext: backend ScreenGeometry")
}

// ManagedClients panics on window inspection.
func (PanicBackend) ManagedClients() ([]WindowInfo, error) {
	panic("vnext: backend ManagedClients")
}

// Revalidate panics on window validation.
func (PanicBackend) Revalidate(WindowRef) error { panic("vnext: backend Revalidate") }

// ActivateAlwaysBoth panics on activation.
func (PanicBackend) ActivateAlwaysBoth(WindowRef) error {
	panic("vnext: backend ActivateAlwaysBoth")
}

// SetFullscreen panics on state mutation.
func (PanicBackend) SetFullscreen(WindowRef, bool) error { panic("vnext: backend SetFullscreen") }

// ActiveWindow panics on active-window inspection.
func (PanicBackend) ActiveWindow() (uint32, error) { panic("vnext: backend ActiveWindow") }
