package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Diobyte/x11_go_dockerinput_daemon/internal/v2line"
)

var xmu sync.Mutex

var debugLog bool

type xActuator struct{}

func (xActuator) KeycodeFor(name string) uint { return keysymToKeycode(name) }

func (xActuator) SendKey(keycode uint, press bool) { sendKey(keycode, press) }

func (xActuator) MoveMouse(x, y int) { moveMouse(x, y) }

func (xActuator) SendButton(button uint, press bool) { sendButton(button, press) }

func (xActuator) ReleaseAll() { releaseAll() }

func handleCommand(cmd string) {
	if debugLog {
		fmt.Fprintf(os.Stderr, "xtest: <- %s\n", v2line.RedactLine(cmd))
	}
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}

	xmu.Lock()
	defer xmu.Unlock()
	v2line.Dispatch(cmd, xActuator{}, func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format, args...)
	})
}

func releaseAllHeld() {
	xmu.Lock()
	defer xmu.Unlock()
	releaseAll()
}
