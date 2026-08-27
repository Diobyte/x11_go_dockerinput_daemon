//go:build !cgo

package main

func initDisplay() int { return -1 }

func closeDisplay() {}

func releaseAll() int { return -1 }

func sendKey(uint, bool) int { return -1 }

func moveMouse(int, int) int { return -1 }

func sendButton(uint, bool) int { return -1 }

func keysymToKeycode(string) uint { return 0 }

func internDestAtoms() int { return -1 }

func netSupportedHas(string) int { return -1 }

func refreshClientList() ([]uint32, int) { return nil, -1 }

func windowGeometry(uint32) (int, int, int, int, int) { return 0, 0, 0, 0, -1 }

func displayGeometry() (int, int) { return 0, 0 }

func windowPID(uint32) uint32 { return 0 }

func windowClass(uint32) string { return "" }

func windowName(uint32) string { return "" }

func windowHasFullscreen(uint32) bool { return false }

func sendNetActive(uint32) int { return -1 }

func sendNetWMStateFS(uint32, bool) int { return -1 }

func setInputFocus(uint32) int { return -1 }

func getActiveXID() uint32 { return 0 }

func queryPointer() (int, int, int) { return 0, 0, -1 }

func moveMouseFake(int, int) int { return -1 }

func peerUID(int) (uint32, uint32, bool) { return 0, 0, false }
