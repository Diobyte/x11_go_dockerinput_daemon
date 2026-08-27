package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Diobyte/x11_go_dockerinput_daemon/internal/vnext"
)

const destMaxConns = 10

// destReadIdle bounds a silent peer so a hung client cannot occupy a slot
// until process exit. Zero disables the deadline (tests may shorten it).
var destReadIdle = 30 * time.Second

var (
	errLivePeer         = errors.New("destination socket has a live peer")
	errParentNotPrivate = errors.New("destination socket parent must be owner-only mode 0700")
	errXCall            = errors.New("X call failed")
	errCoordOutOfRange  = errors.New("coordinate exceeds Xlib int")
)

var destOwnerOnce sync.Once
var destOwner *xOwner

// xDestBackend wraps warp/XTEST and live EWMH. Every Xlib call, including
// KeycodeFor, takes the process-global mutex and runs on the dest X owner
// thread when present.
type xDestBackend struct {
	lk         sync.Locker
	owner      *xOwner
	displayGen uint64
	observeGen uint64
	listHash   uint64
	allow      []uint32
}

func ensureDestOwner() {
	destOwnerOnce.Do(func() {
		destOwner = startXOwner()
	})
}

func destBackend(allow []uint32) xDestBackend {
	ensureDestOwner()
	destOwner.call(func() {
		xmu.Lock()
		defer xmu.Unlock()
		_ = internDestAtoms()
	})
	return xDestBackend{lk: &xmu, owner: destOwner, displayGen: 1, observeGen: 1, allow: allow}
}

func (b xDestBackend) locker() sync.Locker {
	if b.lk == nil {
		return &xmu
	}
	return b.lk
}

func (b xDestBackend) run(fn func()) {
	if b.owner == nil {
		panic("xtest-server: dest X owner missing")
	}
	b.owner.call(fn)
}

func (b xDestBackend) runErr(fn func() error) error {
	var err error
	b.run(func() { err = fn() })
	return err
}

func (b xDestBackend) KeycodeFor(name string) uint {
	var kc uint
	b.run(func() {
		lk := b.locker()
		lk.Lock()
		defer lk.Unlock()
		kc = keysymToKeycode(name)
	})
	return kc
}

func (b xDestBackend) WarpMouse(x, y int) error {
	if x < math.MinInt32 || x > math.MaxInt32 || y < math.MinInt32 || y > math.MaxInt32 {
		return errCoordOutOfRange
	}
	return b.runErr(func() error {
		lk := b.locker()
		lk.Lock()
		err := xerr(moveMouse(x, y))
		lk.Unlock()
		if err != nil {
			return err
		}
		waitPointer(x, y, 50*time.Millisecond)
		return nil
	})
}

func waitPointer(x, y int, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		px, py, err := queryPointer()
		if err == 0 && px == x && py == y {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (b xDestBackend) SendKey(keycode uint, press bool) error {
	return b.runErr(func() error {
		lk := b.locker()
		lk.Lock()
		defer lk.Unlock()
		return xerr(sendKey(keycode, press))
	})
}

func (b xDestBackend) SendButton(button uint, press bool) error {
	return b.runErr(func() error {
		lk := b.locker()
		lk.Lock()
		defer lk.Unlock()
		return xerr(sendButton(button, press))
	})
}

func (b xDestBackend) ScreenGeometry() (int, int, error) {
	var width, height int
	b.run(func() {
		lk := b.locker()
		lk.Lock()
		defer lk.Unlock()
		width, height = displayGeometry()
	})
	if width <= 0 || height <= 0 {
		return 0, 0, vnext.ErrUnavailable
	}
	return width, height, nil
}

func xerr(n int) error {
	if n != 0 {
		return errXCall
	}
	return nil
}

func (b *xDestBackend) ManagedClients() ([]vnext.WindowInfo, error) {
	var (
		out []vnext.WindowInfo
		err error
	)
	runErr := b.runErr(func() error {
		lk := b.locker()
		lk.Lock()
		defer lk.Unlock()
		if internDestAtoms() != 0 {
			return vnext.ErrUnavailable
		}
		if netSupportedHas(vnext.NetClientListAtom) != 1 {
			return vnext.ErrUnavailable
		}
		xids, rc := refreshClientList()
		if rc < 0 {
			return vnext.ErrUnavailable
		}
		h := fnv.New64a()
		for _, id := range xids {
			_, _ = h.Write([]byte{byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24)})
		}
		sum := h.Sum64()
		if b.listHash != 0 && sum != b.listHash {
			b.observeGen++
		}
		b.listHash = sum
		out = make([]vnext.WindowInfo, 0, len(xids))
		for _, id := range xids {
			info := vnext.WindowInfo{
				XID:        id,
				Class:      windowClass(id),
				PID:        windowPID(id),
				Name:       windowName(id),
				DisplayGen: b.displayGen,
				ObserveGen: b.observeGen,
			}
			if x, y, w, h, ge := windowGeometry(id); ge == 0 {
				info.X, info.Y, info.Width, info.Height = x, y, w, h
			}
			if windowHasFullscreen(id) {
				info.State = []string{vnext.NetWMStateFullscreenAtom}
			}
			out = append(out, info)
		}
		return nil
	})
	if runErr != nil {
		err = runErr
	}
	return out, err
}

func (b *xDestBackend) Revalidate(ref vnext.WindowRef) error {
	return b.runErr(func() error {
		lk := b.locker()
		lk.Lock()
		defer lk.Unlock()
		if internDestAtoms() != 0 {
			return vnext.ErrUnavailable
		}
		if netSupportedHas(vnext.NetClientListAtom) != 1 {
			return vnext.ErrUnavailable
		}
		xids, rc := refreshClientList()
		if rc < 0 {
			return vnext.ErrUnavailable
		}
		h := fnv.New64a()
		for _, id := range xids {
			_, _ = h.Write([]byte{byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24)})
		}
		sum := h.Sum64()
		if b.listHash != 0 && sum != b.listHash {
			b.observeGen++
			b.listHash = sum
			return vnext.ErrRefMismatch
		}
		b.listHash = sum
		if ref.DisplayGen != b.displayGen || ref.ObserveGen != b.observeGen {
			return vnext.ErrRefMismatch
		}
		found := false
		for _, id := range xids {
			if id == ref.XID {
				found = true
				break
			}
		}
		if !found {
			return vnext.ErrRefMismatch
		}
		if ref.ExpectedClass != "" && windowClass(ref.XID) != ref.ExpectedClass {
			return vnext.ErrRefMismatch
		}
		if ref.ExpectedPID != 0 && windowPID(ref.XID) != ref.ExpectedPID {
			return vnext.ErrRefMismatch
		}
		return nil
	})
}

func (b *xDestBackend) ActivateAlwaysBoth(ref vnext.WindowRef) error {
	return b.runErr(func() error {
		lk := b.locker()
		lk.Lock()
		defer lk.Unlock()
		if internDestAtoms() != 0 {
			return vnext.ErrUnavailable
		}
		if netSupportedHas(vnext.NetActiveWindowAtom) != 1 {
			return vnext.ErrUnavailable
		}
		if sendNetActive(ref.XID) != 0 {
			return errXCall
		}
		if setInputFocus(ref.XID) != 0 {
			return errXCall
		}
		return nil
	})
}

func (b *xDestBackend) SetFullscreen(ref vnext.WindowRef, add bool) error {
	return b.runErr(func() error {
		lk := b.locker()
		lk.Lock()
		defer lk.Unlock()
		if internDestAtoms() != 0 {
			return vnext.ErrUnavailable
		}
		if netSupportedHas(vnext.NetWMStateAtom) != 1 || netSupportedHas(vnext.NetWMStateFullscreenAtom) != 1 {
			return vnext.ErrUnavailable
		}
		return xerr(sendNetWMStateFS(ref.XID, add))
	})
}

func (b *xDestBackend) ActiveWindow() (uint32, error) {
	var xid uint32
	err := b.runErr(func() error {
		lk := b.locker()
		lk.Lock()
		defer lk.Unlock()
		if internDestAtoms() != 0 {
			return vnext.ErrUnavailable
		}
		if netSupportedHas(vnext.NetActiveWindowAtom) != 1 {
			return vnext.ErrUnavailable
		}
		xid = getActiveXID()
		return nil
	})
	return xid, err
}

func listenUnixDest(path string) (net.Listener, error) {
	if err := checkUnixParent(path); err != nil {
		return nil, err
	}
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: %s", errLivePeer, path)
		}
		if fi.Mode()&os.ModeSocket != 0 {
			c, derr := net.DialTimeout("unix", path, 200*time.Millisecond)
			if derr == nil {
				_ = c.Close()
				return nil, fmt.Errorf("%w: %s", errLivePeer, path)
			}
			if !isConnRefused(derr) {
				return nil, fmt.Errorf("%w: %s", errLivePeer, path)
			}
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return ln, nil
}

func isConnRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}

func checkUnixParent(path string) error {
	dir := filepath.Dir(path)
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return errParentNotPrivate
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return errParentNotPrivate
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return errParentNotPrivate
	}
	return nil
}

func serveVNext(path string, allow, gids []uint32) {
	if len(allow) == 0 {
		fmt.Fprintln(os.Stderr, vnext.ErrEmptyAllowlist)
		os.Exit(2)
	}
	ln, err := listenUnixDest(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xtest-server: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()
	fmt.Fprintln(os.Stderr, destModeLine(path))
	backend := destBackend(allow)
	connSem := make(chan struct{}, destMaxConns)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			fmt.Fprintf(os.Stderr, "xtest-server: dest accept: %v\n", err)
			os.Exit(1)
		}
		if !allowPeer(conn, allow, gids) {
			_ = conn.Close()
			continue
		}
		select {
		case connSem <- struct{}{}:
		default:
			fmt.Fprintf(os.Stderr, "xtest-server: dest connection limit reached; dropping\n")
			_ = conn.Close()
			continue
		}
		go func(c net.Conn) {
			defer func() { <-connSem }()
			serveVNextConn(c, &backend)
		}(conn)
	}
}

func allowPeer(conn net.Conn, allow, gids []uint32) bool {
	if len(allow) == 0 {
		return false
	}
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return false
	}
	var uid, gid uint32
	var got bool
	raw, err := uc.SyscallConn()
	if err != nil {
		return false
	}
	_ = raw.Control(func(fd uintptr) {
		uid, gid, got = peerUID(int(fd))
	})
	if !got {
		return false
	}
	uidOK := false
	for _, a := range allow {
		if a == uid {
			uidOK = true
			break
		}
	}
	if !uidOK {
		return false
	}
	if len(gids) == 0 {
		return true
	}
	for _, g := range gids {
		if g == gid {
			return true
		}
	}
	return false
}

func serveVNextConn(conn net.Conn, backend vnext.Backend) {
	defer conn.Close()
	sess := vnext.NewSession()
	defer sess.Release(backend)
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for {
		if destReadIdle > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(destReadIdle))
		}
		if !sc.Scan() {
			break
		}
		out := vnext.HandleLine(sc.Text(), sess, backend)
		raw, err := out.WireJSON()
		if err != nil {
			raw, _ = json.Marshal(struct {
				Code string `json:"code"`
			}{Code: out.Code})
		}
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := fmt.Fprintf(conn, "%s\n", raw); err != nil {
			break
		}
		_ = conn.SetWriteDeadline(time.Time{})
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "xtest-server: dest scanner: %v\n", err)
	}
}
