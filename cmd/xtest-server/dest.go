package main

import (
	"bufio"
	"context"
	"encoding/binary"
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

const (
	destAcceptPoll      = 100 * time.Millisecond
	destShutdownTimeout = 5 * time.Second
)

// destReadIdle bounds a silent peer so a hung client cannot occupy a slot
// until process exit. Zero disables the deadline (tests may shorten it).
var destReadIdle = 30 * time.Second

var (
	errLivePeer            = errors.New("destination socket has a live peer")
	errParentNotPrivate    = errors.New("destination socket parent must be owner-only mode 0700")
	errXCall               = errors.New("x call failed")
	errCoordOutOfRange     = errors.New("coordinate exceeds Xlib int")
	errDestShutdownTimeout = errors.New("destination shutdown timed out")
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
		var encoded [4]byte
		for _, id := range xids {
			binary.LittleEndian.PutUint32(encoded[:], id)
			_, _ = h.Write(encoded[:])
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
		var encoded [4]byte
		for _, id := range xids {
			binary.LittleEndian.PutUint32(encoded[:], id)
			_, _ = h.Write(encoded[:])
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
			dialer := net.Dialer{Timeout: 200 * time.Millisecond}
			c, derr := dialer.DialContext(context.Background(), "unix", path)
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
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
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
	euid, euidOK := currentEUID()
	if !ok || !euidOK || st.Uid != euid {
		return errParentNotPrivate
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return errParentNotPrivate
	}
	return nil
}

type destConnSet struct {
	lock     sync.Mutex
	conns    map[net.Conn]struct{}
	closing  bool
	drained  chan struct{}
	drainOne sync.Once
}

func newDestConnSet() *destConnSet {
	return &destConnSet{
		conns:   make(map[net.Conn]struct{}, destMaxConns),
		drained: make(chan struct{}),
	}
}

func (s *destConnSet) add(conn net.Conn) bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.closing {
		return false
	}
	s.conns[conn] = struct{}{}
	return true
}

func (s *destConnSet) remove(conn net.Conn) {
	s.lock.Lock()
	delete(s.conns, conn)
	if s.closing && len(s.conns) == 0 {
		s.drainOne.Do(func() { close(s.drained) })
	}
	s.lock.Unlock()
}

func (s *destConnSet) shutdown(timeout time.Duration) error {
	s.lock.Lock()
	s.closing = true
	conns := make([]net.Conn, 0, len(s.conns))
	for conn := range s.conns {
		conns = append(conns, conn)
	}
	if len(conns) == 0 {
		s.drainOne.Do(func() { close(s.drained) })
	}
	s.lock.Unlock()

	for _, conn := range conns {
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			fmt.Fprintln(os.Stderr, "xtest-server: closing destination connection during shutdown")
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.drained:
		return nil
	case <-timer.C:
		return errDestShutdownTimeout
	}
}

func serveVNext(ctx context.Context, path string, allow, gids []uint32) error {
	if len(allow) == 0 {
		return vnext.ErrEmptyAllowlist
	}
	ln, err := listenUnixDest(path)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, destModeLine(path))
	backend := destBackend(allow)
	connSem := make(chan struct{}, destMaxConns)
	connections := newDestConnSet()
	unixListener, ok := ln.(*net.UnixListener)
	if !ok {
		_ = ln.Close()
		_ = os.Remove(path)
		return errors.New("destination listener is not Unix")
	}
	var serveErr error
	for ctx.Err() == nil {
		if err := unixListener.SetDeadline(time.Now().Add(destAcceptPoll)); err != nil {
			serveErr = fmt.Errorf("setting accept deadline: %w", err)
			break
		}
		conn, err := ln.Accept()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				serveErr = fmt.Errorf("accepting destination connection: %w", err)
			}
			break
		}
		if ctx.Err() != nil {
			_ = conn.Close()
			break
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
		if !connections.add(conn) {
			<-connSem
			_ = conn.Close()
			break
		}
		go func(c net.Conn) {
			defer func() {
				connections.remove(c)
				<-connSem
				if recover() != nil {
					fmt.Fprintln(os.Stderr, "xtest-server: destination handler panic")
				}
			}()
			serveVNextConnContext(ctx, c, &backend)
		}(conn)
	}
	if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) && serveErr == nil {
		serveErr = fmt.Errorf("closing destination listener: %w", err)
	}
	if err := connections.shutdown(destShutdownTimeout); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) && serveErr == nil {
		serveErr = fmt.Errorf("removing destination socket: %w", err)
	}
	return serveErr
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
	serveVNextConnContext(context.Background(), conn, backend)
}

func serveVNextConnContext(ctx context.Context, conn net.Conn, backend vnext.Backend) {
	defer func() { _ = conn.Close() }()
	sess := vnext.NewSession()
	defer func() {
		if out := sess.Release(backend); out.Code != vnext.CodeSubmitted {
			fmt.Fprintln(os.Stderr, "xtest-server: destination session release incomplete")
		}
	}()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for {
		if destReadIdle > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(destReadIdle))
		}
		if !sc.Scan() {
			break
		}
		if ctx.Err() != nil {
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
	if err := sc.Err(); err != nil && ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
		fmt.Fprintf(os.Stderr, "xtest-server: dest scanner: %v\n", err)
	}
}
