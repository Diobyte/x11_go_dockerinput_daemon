package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Diobyte/x11_go_dockerinput_daemon/internal/vnext"
)

// countLock records Lock calls so tests can prove shipped X methods take the gate.
type countLock struct {
	mu      sync.Mutex
	n       atomic.Int32
	held    atomic.Int32
	overlap atomic.Int32
}

func (c *countLock) Lock() {
	c.mu.Lock()
	c.n.Add(1)
	if c.held.Add(1) > 1 {
		c.overlap.Add(1)
	}
}

func (c *countLock) Unlock() {
	c.held.Add(-1)
	c.mu.Unlock()
}

func TestDestKeycodeForTakesXLock(t *testing.T) {
	lk := &countLock{}
	b := xDestBackend{lk: lk, owner: startXOwner()}
	_ = b.KeycodeFor("F1")
	if lk.n.Load() < 1 {
		t.Fatal("KeycodeFor entered Xlib without the process-global mutex")
	}
}

func TestDestXMethodsShareOneLock(t *testing.T) {
	lk := &countLock{}
	b := xDestBackend{lk: lk, owner: startXOwner()}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = b.KeycodeFor("F1")
		}()
		go func() {
			defer wg.Done()
			_ = b.WarpMouse(1, 1)
		}()
		go func() {
			defer wg.Done()
			_ = b.SendKey(67, true)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.SendButton(1, true)
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
	if lk.overlap.Load() != 0 {
		t.Fatalf("concurrent Xlib: overlap=%d", lk.overlap.Load())
	}
	if lk.n.Load() < 32 {
		t.Fatalf("lock calls=%d want >= 32", lk.n.Load())
	}
}

func TestDestWindowMethodsWithoutDisplay(t *testing.T) {
	lk := &countLock{}
	b := &xDestBackend{lk: lk, owner: startXOwner()}
	sess := vnext.NewSession()
	for i, line := range []string{
		`{"op":"activate","displayGen":1,"observeGen":1,"xid":42}`,
		`{"op":"fullscreen","displayGen":1,"observeGen":1,"xid":42,"add":true}`,
		`{"op":"inspect"}`,
	} {
		out := vnext.HandleLine(line, sess, b)
		if out.Code != vnext.CodeUnavailable {
			t.Fatalf("case %d: got %q", i, out.Code)
		}
	}
}

func TestListenUnixDestStaleUnlink(t *testing.T) {
	dir := privateTempDir(t)
	path := filepath.Join(dir, "xtest.sock")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := listenUnixDest(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("stale leftover not replaced by socket: %v", fi.Mode())
	}
}

func TestListenUnixDestSymlinkRefused(t *testing.T) {
	dir := privateTempDir(t)
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "xtest.sock")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	_, err := listenUnixDest(path)
	if !errors.Is(err, errLivePeer) {
		t.Fatalf("got %v", err)
	}
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was removed")
	}
}

func TestListenUnixDestDeadSocketUnlinkThenBind(t *testing.T) {
	dir := privateTempDir(t)
	path := filepath.Join(dir, "xtest.sock")
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		_ = syscall.Close(fd)
		t.Fatal(err)
	}
	_ = syscall.Close(fd)
	ln2, err := listenUnixDest(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln2.Close() }()
}

func TestListenUnixDestLivePeer(t *testing.T) {
	dir := privateTempDir(t)
	path := filepath.Join(dir, "xtest.sock")
	ln, err := listenUnixDest(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	_, err = listenUnixDest(path)
	if !errors.Is(err, errLivePeer) {
		t.Fatalf("got %v", err)
	}
}

func TestListenUnixDestParentWorldWritable(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil { //nolint:gosec // verifies fail-closed parent permissions
		t.Fatal(err)
	}
	_, err := listenUnixDest(filepath.Join(dir, "xtest.sock"))
	if !errors.Is(err, errParentNotPrivate) {
		t.Fatalf("got %v", err)
	}
}

func TestListenUnixDestChmod0600(t *testing.T) {
	dir := privateTempDir(t)
	path := filepath.Join(dir, "xtest.sock")
	ln, err := listenUnixDest(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", fi.Mode().Perm())
	}
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // Unix socket parent requires search permission
		t.Fatal(err)
	}
	return dir
}

func TestServeVNextConnEOFReleasesSession(t *testing.T) {
	c1, c2 := net.Pipe()
	f := vnext.NewFake()
	done := make(chan struct{})
	go func() {
		serveVNextConn(c2, f)
		close(done)
	}()
	if _, err := fmt.Fprintln(c1, `{"op":"key","name":"F1","press":true}`); err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(c1)
	if !sc.Scan() {
		t.Fatal("no ack")
	}
	var reply struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(sc.Text()), &reply); err != nil {
		t.Fatalf("wire %q: %v", sc.Text(), err)
	}
	if reply.Code != vnext.CodeSubmitted {
		t.Fatalf("ack %q", sc.Text())
	}
	_ = c1.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	got := strings.Join(f.Snapshot(), " ")
	if !strings.Contains(got, "key:67:up") {
		t.Fatalf("missing session up: %q", got)
	}
	if strings.Contains(got, "releaseall") {
		t.Fatal("dest used global releaseall")
	}
}

func TestServeVNextSlowClientIdle(t *testing.T) {
	old := destReadIdle
	destReadIdle = 50 * time.Millisecond
	defer func() { destReadIdle = old }()
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	f := vnext.NewFake()
	done := make(chan struct{})
	go func() {
		serveVNextConn(c2, f)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("idle dest client was not dropped")
	}
}

func TestDestShutdownReleasesHeldSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	fake := vnext.NewFake()
	connections := newDestConnSet()
	if !connections.add(c2) {
		t.Fatal("connection set rejected before shutdown")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer connections.remove(c2)
		serveVNextConnContext(ctx, c2, fake)
	}()

	scanner := bufio.NewScanner(c1)
	requests := []string{
		`{"op":"key","name":"F1","press":true}`,
		`{"op":"button","button":1,"press":true}`,
	}
	for i, request := range requests {
		if _, err := fmt.Fprintln(c1, request); err != nil {
			t.Fatal(err)
		}
		if !scanner.Scan() {
			t.Fatalf("missing response %d", i)
		}
	}

	cancel()
	if err := connections.shutdown(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not stop")
	}
	calls := strings.Join(fake.Snapshot(), " ")
	if strings.Count(calls, "key:67:up") != 1 || strings.Count(calls, "button:1:up") != 1 {
		t.Fatalf("session release calls %q", calls)
	}
	if strings.Contains(calls, "releaseall") {
		t.Fatalf("global release during shutdown: %q", calls)
	}
}

type blockingReleaseFake struct {
	*vnext.Fake
	releaseStarted chan struct{}
	releaseBlock   chan struct{}
	startOnce      sync.Once
	unblockOnce    sync.Once
	upCalls        atomic.Int32
}

func (f *blockingReleaseFake) SendKey(keycode uint, press bool) error {
	if press {
		return f.Fake.SendKey(keycode, true)
	}
	f.upCalls.Add(1)
	f.startOnce.Do(func() { close(f.releaseStarted) })
	<-f.releaseBlock
	return f.Fake.SendKey(keycode, false)
}

func (f *blockingReleaseFake) unblock() {
	f.unblockOnce.Do(func() { close(f.releaseBlock) })
}

func TestDestShutdownBoundsBlockedReleaseAndDoesNotRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c1, c2 := net.Pipe()
	fake := &blockingReleaseFake{
		Fake:           vnext.NewFake(),
		releaseStarted: make(chan struct{}),
		releaseBlock:   make(chan struct{}),
	}
	defer fake.unblock()
	connections := newDestConnSet()
	if !connections.add(c2) {
		t.Fatal("connection set rejected before shutdown")
	}
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		defer connections.remove(c2)
		serveVNextConnContext(ctx, c2, fake)
	}()
	if _, err := fmt.Fprintln(c1, `{"op":"key","name":"F1","press":true}`); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(c1)
	if !scanner.Scan() {
		t.Fatal("missing key down response")
	}
	cancel()
	if err := c1.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fake.releaseStarted:
	case <-time.After(time.Second):
		t.Fatal("session release did not start")
	}

	start := time.Now()
	err := connections.shutdown(20 * time.Millisecond)
	if !errors.Is(err, errDestShutdownTimeout) {
		t.Fatalf("got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("shutdown exceeded bound: %s", elapsed)
	}
	if calls := fake.upCalls.Load(); calls != 1 {
		t.Fatalf("release attempts=%d want 1", calls)
	}
	if connections.add(c1) {
		t.Fatal("connection set admitted after shutdown")
	}
	fake.unblock()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after release unblocked")
	}
	if calls := fake.upCalls.Load(); calls != 1 {
		t.Fatalf("release retried after timeout: %d", calls)
	}
}

func TestServeVNextConnDoesNotDispatchAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	fake := vnext.NewFake()
	done := make(chan struct{})
	go func() {
		serveVNextConnContext(ctx, c2, fake)
		close(done)
	}()
	if _, err := fmt.Fprintln(c1, `{"op":"move","x":10,"y":20}`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled handler did not stop")
	}
	if calls := fake.Snapshot(); len(calls) != 0 {
		t.Fatalf("dispatched after cancellation: %v", calls)
	}
}
