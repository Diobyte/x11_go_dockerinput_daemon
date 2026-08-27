package vnext

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Mode is XOR mutation admission for one DISPLAY.
type Mode int

const (
	// ModeNone indicates that no mutation transport was selected.
	ModeNone Mode = iota
	// ModeV2 selects the legacy TCP or standard-input transport.
	ModeV2
	// ModeVNext selects the private Unix destination transport.
	ModeVNext
)

var (
	// ErrDualBind rejects simultaneous legacy and destination transports.
	ErrDualBind = errors.New("vnext: -tcp/-ws and -vnext cannot both be set")
	// ErrNotUnix rejects destination transports other than an absolute Unix socket.
	ErrNotUnix = errors.New("vnext: destination listen must be unix:path (no wildcard plaintext)")
	// ErrEmptyAllowlist rejects destination mode without an admitted user.
	ErrEmptyAllowlist = errors.New("vnext: empty dest allowlist does not listen")
	// ErrBadAllowlist rejects malformed user or group identifiers.
	ErrBadAllowlist = errors.New("vnext: dest allowlist must be euid or comma-separated UIDs")
)

// AdmitFlags applies the daemon flag composition: -ws aliases -tcp, then XOR
// with -vnext. Tests must call this, not invent a second alias.
func AdmitFlags(tcpAddr, wsAddr, vnextSpec string) (Mode, string, error) {
	if strings.TrimSpace(wsAddr) != "" {
		tcpAddr = wsAddr
	}
	return SelectMode(tcpAddr, vnextSpec)
}

// SelectMode chooses legacy v2 or destination. Both non-empty is dual-bind.
func SelectMode(v2TCP, vnextSpec string) (Mode, string, error) {
	v2TCP = strings.TrimSpace(v2TCP)
	vnextSpec = strings.TrimSpace(vnextSpec)
	if v2TCP != "" && vnextSpec != "" {
		return ModeNone, "", ErrDualBind
	}
	if vnextSpec != "" {
		addr, err := ParseUnixListen(vnextSpec)
		if err != nil {
			return ModeNone, "", err
		}
		return ModeVNext, addr, nil
	}
	return ModeV2, v2TCP, nil
}

// ParseUnixListen accepts only unix:/absolute/path. TCP, wildcards, and empty
// paths fail closed (no destination plaintext bind).
func ParseUnixListen(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if !strings.HasPrefix(spec, "unix:") {
		return "", ErrNotUnix
	}
	path := strings.TrimPrefix(spec, "unix:")
	if path == "" || strings.ContainsAny(path, "\x00\n\r") {
		return "", ErrNotUnix
	}
	if strings.Contains(path, "://") || strings.HasPrefix(path, "//") {
		return "", ErrNotUnix
	}
	if strings.HasPrefix(path, "tcp") || strings.Contains(path, "0.0.0.0") || path == "*" {
		return "", ErrNotUnix
	}
	if !strings.HasPrefix(path, "/") {
		return "", ErrNotUnix
	}
	if filepath.Clean(path) != path {
		return "", ErrNotUnix
	}
	return path, nil
}

// ParseAllowlist parses -vnext-allow. "euid" is the process euid. Empty is
// fail-closed and does not listen.
func ParseAllowlist(spec string) ([]uint32, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, ErrEmptyAllowlist
	}
	var out []uint32
	for _, p := range strings.Split(spec, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "euid" {
			u, err := processID(os.Geteuid())
			if err != nil {
				return nil, err
			}
			out = append(out, u)
			continue
		}
		u, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return nil, ErrBadAllowlist
		}
		out = append(out, uint32(u))
	}
	if len(out) == 0 {
		return nil, ErrEmptyAllowlist
	}
	return out, nil
}

// ParseGIDAllowlist parses -vnext-allow-gid. Empty means any GID of an
// allowlisted UID. "egid" is the process egid.
func ParseGIDAllowlist(spec string) ([]uint32, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var out []uint32
	for _, p := range strings.Split(spec, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "egid" {
			u, err := processID(os.Getegid())
			if err != nil {
				return nil, err
			}
			out = append(out, u)
			continue
		}
		u, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return nil, ErrBadAllowlist
		}
		out = append(out, uint32(u))
	}
	if len(out) == 0 {
		return nil, ErrBadAllowlist
	}
	return out, nil
}

func processID(id int) (uint32, error) {
	if id < 0 || uint64(id) > math.MaxUint32 {
		return 0, ErrBadAllowlist
	}
	return uint32(id), nil
}
