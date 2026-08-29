package v2live_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDaemonDestUnixLaunch(t *testing.T) {
	display, stopDisplay := startXvfb(t)
	defer stopDisplay()
	bin := buildDaemon(t)

	dir := privateTempDir(t)
	sock := filepath.Join(dir, "xtest.sock")
	stderr := &safeBuffer{}
	cmd := exec.CommandContext(t.Context(), bin, "-vnext", "unix:"+sock) //nolint:gosec // test-built binary
	cmd.Env = append(os.Environ(), "DISPLAY="+display)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	var conn net.Conn
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		dialer := net.Dialer{Timeout: 200 * time.Millisecond}
		c, err := dialer.DialContext(context.Background(), "unix", sock)
		if err == nil {
			conn = c
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if conn == nil {
		t.Fatalf("dest unix dial failed stderr=%s", stderr.String())
	}
	defer func() { _ = conn.Close() }()
	if !strings.Contains(stderr.String(), "mode dest unix") {
		t.Fatalf("missing dest mode line: %s", stderr.String())
	}

	if _, err := fmt.Fprintln(conn, `{"op":"inspect"}`); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	sc := bufio.NewScanner(conn)
	if !sc.Scan() {
		t.Fatalf("no inspect reply stderr=%s", stderr.String())
	}
	var reply struct {
		Code    string `json:"code"`
		Windows []any  `json:"windows"`
	}
	if err := json.Unmarshal([]byte(sc.Text()), &reply); err != nil {
		t.Fatalf("inspect wire not JSON")
	}
	if reply.Code != "Submitted" && reply.Code != "Unavailable" {
		t.Fatalf("inspect code %q", reply.Code)
	}
}

func TestExplicitLockFailurePrecedesDisplayInitialization(t *testing.T) {
	bin := buildDaemon(t)
	dir := privateTempDir(t)
	sock := filepath.Join(dir, "xtest.sock")
	missingLock := filepath.Join(dir, "missing.lock")
	args := []string{
		"-vnext", "unix:" + sock,
		"-lock-file", missingLock,
	}
	cmd := exec.CommandContext(t.Context(), bin, args...) //nolint:gosec // test-built binary and test-owned paths
	cmd.Env = append(os.Environ(), "DISPLAY=:65534")
	out, err := cmd.CombinedOutput()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 1 {
		t.Fatalf("invalid lock exit=%v err=%v out=%s", exitOf(cmd), err, out)
	}
	if !strings.Contains(string(out), "lock file must already exist") {
		t.Fatalf("missing explicit lock diagnostic: %s", out)
	}
	if strings.Contains(string(out), "failed to open X11 display") {
		t.Fatalf("display initialized before lock validation: %s", out)
	}
	if _, statErr := os.Lstat(sock); !os.IsNotExist(statErr) {
		t.Fatalf("invalid lock created socket %s", sock)
	}
}

func TestLockFileCannotBeDestinationSocket(t *testing.T) {
	bin := buildDaemon(t)
	dir := privateTempDir(t)
	path := filepath.Join(dir, "same-path.sock")
	args := []string{
		"-vnext", "unix:" + path,
		"-lock-file", path,
	}
	cmd := exec.CommandContext(t.Context(), bin, args...) //nolint:gosec // test-built binary and test-owned paths
	out, err := cmd.CombinedOutput()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 2 {
		t.Fatalf("same path exit=%v err=%v out=%s", exitOf(cmd), err, out)
	}
	if !strings.Contains(string(out), "lock file and Unix socket must differ") {
		t.Fatalf("missing same-path diagnostic: %s", out)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("same lock and socket path created %s", path)
	}
}

func TestDaemonDestSIGTERMDrainsSessionAndReleasesLock(t *testing.T) {
	display, stopDisplay := startXvfb(t)
	defer stopDisplay()
	bin := buildDaemon(t)
	dir := privateTempDir(t)
	lockPath := filepath.Join(dir, "authority.lock")
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "xtest.sock")
	args := []string{
		"-vnext", "unix:" + sock,
		"-lock-file", lockPath,
	}

	cmd, conn, stderr := startDestProcess(t, bin, display, args, sock)
	if _, err := fmt.Fprintln(conn, `{"op":"key","name":"F1","press":true}`); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() || !strings.Contains(scanner.Text(), `"code":"Submitted"`) {
		t.Fatalf("key down response=%q stderr=%s", scanner.Text(), stderr.String())
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitForCleanExit(t, cmd, stderr)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if scanner.Scan() {
		t.Fatalf("connection remained open after shutdown: %q", scanner.Text())
	}
	_ = conn.Close()
	if _, err := os.Lstat(sock); !os.IsNotExist(err) {
		t.Fatalf("shutdown left destination socket %s", sock)
	}

	replacement, replacementConn, replacementStderr := startDestProcess(t, bin, display, args, sock)
	if err := replacement.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitForCleanExit(t, replacement, replacementStderr)
	_ = replacementConn.Close()
}

func startDestProcess(
	t *testing.T,
	bin string,
	display string,
	args []string,
	sock string,
) (*exec.Cmd, net.Conn, *safeBuffer) {
	t.Helper()
	stderr := &safeBuffer{}
	cmd := exec.CommandContext(t.Context(), bin, args...) //nolint:gosec // test-built binary and fixed arguments
	cmd.Env = append(os.Environ(), "DISPLAY="+display)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		dialer := net.Dialer{Timeout: 200 * time.Millisecond}
		conn, err := dialer.DialContext(context.Background(), "unix", sock)
		if err == nil {
			return cmd, conn, stderr
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("destination dial failed stderr=%s", stderr.String())
	return nil, nil, nil
}

func waitForCleanExit(t *testing.T, cmd *exec.Cmd, stderr *safeBuffer) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("daemon shutdown: %v stderr=%s", err, stderr.String())
		}
	case <-time.After(7 * time.Second):
		t.Fatalf("daemon shutdown timed out stderr=%s", stderr.String())
	}
}

func TestEmptyAllowlistDoesNotListen(t *testing.T) {
	bin := buildDaemon(t)
	dir := privateTempDir(t)
	sock := filepath.Join(dir, "xtest.sock")
	cmd := exec.CommandContext(t.Context(), bin, "-vnext", "unix:"+sock, "-vnext-allow", "") //nolint:gosec // test-built binary
	out, err := cmd.CombinedOutput()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() == 0 {
		t.Fatalf("empty allowlist listened: err=%v out=%s", err, out)
	}
	if _, statErr := os.Lstat(sock); !os.IsNotExist(statErr) {
		t.Fatalf("empty allowlist bound %s", sock)
	}
}

func TestDaemonRefusesHostXwayland(t *testing.T) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no Wayland session")
	}
	bin := buildDaemon(t)
	dir := privateTempDir(t)
	sock := filepath.Join(dir, "xtest.sock")
	cmd := exec.CommandContext(t.Context(), bin, "-vnext", "unix:"+sock) //nolint:gosec // test-built binary
	cmd.Env = append(os.Environ(), "DISPLAY="+os.Getenv("DISPLAY"))
	out, err := cmd.CombinedOutput()
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 75 {
		t.Skip("host DISPLAY singleton lock held")
	}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 0 {
		t.Fatalf("Xwayland dest started: err=%v out=%s", err, out)
	}
	if !strings.Contains(string(out), "Xwayland") {
		t.Fatalf("want Xwayland refuse, got err=%v out=%s", err, out)
	}
	if _, statErr := os.Lstat(sock); !os.IsNotExist(statErr) {
		t.Fatalf("Xwayland refuse still bound %s", sock)
	}
}

func TestDestDisplayRestartIsFatal(t *testing.T) {
	display, stopDisplay := startXvfb(t)
	bin := buildDaemon(t)
	dir := privateTempDir(t)
	sock := filepath.Join(dir, "xtest.sock")
	stderr := &safeBuffer{}
	cmd := exec.CommandContext(t.Context(), bin, "-vnext", "unix:"+sock) //nolint:gosec // test-built binary
	cmd.Env = append(os.Environ(), "DISPLAY="+display)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()
	var conn net.Conn
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		dialer := net.Dialer{Timeout: 200 * time.Millisecond}
		c, err := dialer.DialContext(context.Background(), "unix", sock)
		if err == nil {
			conn = c
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if conn == nil {
		t.Fatalf("dest unix dial failed stderr=%s", stderr.String())
	}
	stopDisplay()
	_, _ = fmt.Fprintln(conn, `{"op":"inspect"}`)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("daemon survived display loss stderr=%s", stderr.String())
	}
	_ = conn.Close()
}

func TestXephyrWMLiveEWMH(t *testing.T) {
	if lookPath("Xephyr") == "" || (lookPath("openbox") == "" && lookPath("fluxbox") == "" && lookPath("xfwm4") == "") {
		t.Skip("Xephyr and a compatible window manager are unavailable")
	}
	t.Skip("Xephyr+WM fixture not wired (recorded remainder; skip-not-pass)")
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // Unix socket parent requires search permission
		t.Fatal(err)
	}
	return dir
}
