package main

import "fmt"

func stdinReadyLine() string { return "READY" }

func tcpReadyLine(port int) string { return fmt.Sprintf("READY tcp:%d", port) }

// destModeLine is stderr identity for XOR dest. It is not a v2 stdout READY
// and must not be accepted by x11input.WaitReady.
func destModeLine(path string) string {
	return fmt.Sprintf("xtest-server: mode dest unix %s", path)
}
