package main

import (
	"os"
	"testing"
)

func TestWarpVersusFakeMotionMeasurement(t *testing.T) {
	closeDisplay()
	display, stop := startXvfb(t)
	defer stop()
	t.Setenv("DISPLAY", display)
	if initDisplay() != 0 {
		t.Skip("XOpenDisplay failed on private Xvfb")
	}
	if internDestAtoms() != 0 {
		t.Skip("intern atoms failed")
	}
	if moveMouse(20, 30) != 0 {
		t.Fatal("warp failed")
	}
	wx, wy, werr := queryPointer()
	if moveMouseFake(40, 50) != 0 {
		t.Fatal("FakeMotion failed")
	}
	fx, fy, ferr := queryPointer()
	t.Logf("warp pointer=(%d,%d) err=%d fake=(%d,%d) err=%d; destination motion remains XWarpPointer", wx, wy, werr, fx, fy, ferr)
}

func TestInitDisplayRefusesXwayland(t *testing.T) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no Wayland session (host Xwayland is intentionally refused)")
	}
	closeDisplay()
	rc := initDisplay()
	if rc != -2 {
		t.Fatalf("initDisplay on host DISPLAY=%s: rc=%d want -2", os.Getenv("DISPLAY"), rc)
	}
}
