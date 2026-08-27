package vnext

// PanicBackend fails the test process if mutation is attempted. Used to prove
// invalid/unknown HandleLine never reaches a backend.
type PanicBackend struct{}

func (PanicBackend) KeycodeFor(string) uint { panic("vnext: backend KeycodeFor") }

func (PanicBackend) WarpMouse(int, int) error { panic("vnext: backend WarpMouse") }

func (PanicBackend) SendKey(uint, bool) error { panic("vnext: backend SendKey") }

func (PanicBackend) SendButton(uint, bool) error { panic("vnext: backend SendButton") }

func (PanicBackend) ScreenGeometry() (int, int, error) {
	panic("vnext: backend ScreenGeometry")
}

func (PanicBackend) ManagedClients() ([]WindowInfo, error) {
	panic("vnext: backend ManagedClients")
}

func (PanicBackend) Revalidate(WindowRef) error { panic("vnext: backend Revalidate") }

func (PanicBackend) ActivateAlwaysBoth(WindowRef) error {
	panic("vnext: backend ActivateAlwaysBoth")
}

func (PanicBackend) SetFullscreen(WindowRef, bool) error { panic("vnext: backend SetFullscreen") }

func (PanicBackend) ActiveWindow() (uint32, error) { panic("vnext: backend ActiveWindow") }
