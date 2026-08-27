package v2live_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDaemonDestUnixLaunch(t *testing.T) {
	display, stopDisplay := startXvfb(t)
	defer stopDisplay()
	bin := buildDaemon(t)

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "xtest.sock")
	stderr := &safeBuffer{}
	cmd := exec.Command(bin, "-vnext", "unix:"+sock)
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
		c, err := net.DialTimeout("unix", sock, 200*time.Millisecond)
		if err == nil {
			conn = c
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if conn == nil {
		t.Fatalf("dest unix dial failed stderr=%s", stderr.String())
	}
	defer conn.Close()
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

func TestEmptyAllowlistDoesNotListen(t *testing.T) {
	bin := buildDaemon(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "xtest.sock")
	cmd := exec.Command(bin, "-vnext", "unix:"+sock, "-vnext-allow", "")
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
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "xtest.sock")
	cmd := exec.Command(bin, "-vnext", "unix:"+sock)
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
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "xtest.sock")
	stderr := &safeBuffer{}
	cmd := exec.Command(bin, "-vnext", "unix:"+sock)
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
		c, err := net.DialTimeout("unix", sock, 200*time.Millisecond)
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
