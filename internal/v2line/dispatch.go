// Package v2line is the live v2 command parse and dispatch path used by
// cmd/xtest-server. Tests must call these functions rather than a second
// protocol implementation.
package v2line

import (
	"strconv"
	"strings"
	"time"
)

const (
	minKeyHoldMillis = 20
	maxKeyHoldMillis = 500
	modclickDefault  = 45
)

// Actuator is the X11-facing side of a parsed v2 command. The daemon's
// XWarpPointer / XTEST wrappers implement this; tests record calls.
type Actuator interface {
	KeycodeFor(name string) uint
	SendKey(keycode uint, press bool)
	MoveMouse(x, y int)
	SendButton(button uint, press bool)
	ReleaseAll()
}

// ParseKeyHold is the live "key <keysym> <hold-ms>" decoder: canonical
// decimal, inclusive 20..500 ms. Extra fields, leading zeros, and signs fail.
func ParseKeyHold(parts []string) (time.Duration, bool) {
	if len(parts) != 3 || parts[0] != "key" {
		return 0, false
	}
	holdMillis, err := strconv.Atoi(parts[2])
	if err != nil || strconv.Itoa(holdMillis) != parts[2] || holdMillis < minKeyHoldMillis || holdMillis > maxKeyHoldMillis {
		return 0, false
	}
	return time.Duration(holdMillis) * time.Millisecond, true
}

// Dispatch runs one trimmed v2 line against a. Unknown opcodes are no-ops.
// logf must not be given raw command text or keysym names.
func Dispatch(cmd string, a Actuator, logf func(string, ...any)) {
	if a == nil {
		return
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "keydown":
		if len(parts) < 2 {
			return
		}
		if kc := keycodeOrLog(a, logf, parts[1]); kc != 0 {
			a.SendKey(kc, true)
		}
	case "keyup":
		if len(parts) < 2 {
			return
		}
		if kc := keycodeOrLog(a, logf, parts[1]); kc != 0 {
			a.SendKey(kc, false)
		}
	case "key":
		hold, ok := ParseKeyHold(parts)
		if !ok {
			logf("xtest-server: invalid key command; expected key <keysym> <hold-ms %d..%d>\n", minKeyHoldMillis, maxKeyHoldMillis)
			return
		}
		if kc := keycodeOrLog(a, logf, parts[1]); kc != 0 {
			a.SendKey(kc, true)
			time.Sleep(hold)
			a.SendKey(kc, false)
		}
	case "mousemove":
		if len(parts) < 3 {
			return
		}
		x, _ := strconv.Atoi(parts[1])
		y, _ := strconv.Atoi(parts[2])
		a.MoveMouse(x, y)
	case "mousedown":
		if len(parts) < 2 {
			return
		}
		b, _ := strconv.Atoi(parts[1])
		a.SendButton(uint(b), true)
	case "mouseup":
		if len(parts) < 2 {
			return
		}
		b, _ := strconv.Atoi(parts[1])
		a.SendButton(uint(b), false)
	case "click":
		if len(parts) < 2 {
			return
		}
		b, _ := strconv.Atoi(parts[1])
		a.SendButton(uint(b), true)
		a.SendButton(uint(b), false)
	case "modclick":
		// modclick <mod-keysym> <button> <x> <y> <hold-ms>: serialized v2
		// sequence under xmu (modifier held across the button, bounded settle
		// between warp and press). Not transactional or globally atomic.
		if len(parts) < 6 {
			return
		}
		if kc := keycodeOrLog(a, logf, parts[1]); kc != 0 {
			b, err := strconv.Atoi(parts[2])
			if err != nil {
				return
			}
			x, err := strconv.Atoi(parts[3])
			if err != nil {
				return
			}
			y, err := strconv.Atoi(parts[4])
			if err != nil {
				return
			}
			holdMillis, err := strconv.Atoi(parts[5])
			if err != nil || holdMillis < minKeyHoldMillis || holdMillis > maxKeyHoldMillis {
				holdMillis = modclickDefault
			}
			a.SendKey(kc, true)
			a.MoveMouse(x, y)
			time.Sleep(time.Duration(holdMillis) * time.Millisecond)
			a.SendButton(uint(b), true)
			time.Sleep(time.Duration(holdMillis) * time.Millisecond)
			a.SendButton(uint(b), false)
			a.SendKey(kc, false)
		}
	case "releaseall":
		a.ReleaseAll()
	}
}

func keycodeOrLog(a Actuator, logf func(string, ...any), name string) uint {
	kc := a.KeycodeFor(name)
	if kc == 0 {
		// Do not log the keysym: TypeText sends character names.
		logf("xtest-server: unknown keysym (key dropped)\n")
	}
	return kc
}
