package v2live_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Diobyte/x11_go_dockerinput_daemon/x11input"
)

const plantedSecret = "X11_INPUT_REDACT_FIXTURE_TOKEN"

func TestDaemonV2Contract(t *testing.T) {
	display, _ := startXvfb(t)
	bin := buildDaemon(t)

	stderr := &safeBuffer{}
	cmd, addr, stop := startDaemon(t, bin, display, []string{"-debug", "-tcp", "127.0.0.1:0"}, stderr)
	defer stop()
	_ = cmd

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	health := dialRaw(t, addr)
	_, err := io.WriteString(health, "GET /healthz HTTP/1.1\r\nHost: localhost\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(health)
	_ = health.Close()
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "HTTP/1.1 200 OK\r\n") || !strings.HasSuffix(got, "x11-input/2\n") {
		t.Fatalf("healthz contract mismatch: %q", got)
	}

	client, err := x11input.Dial(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	client.SetReady("READY tcp:")
	lines := []string{
		"keydown F1",
		"keyup F1",
		"key F1 20",
		"mousemove 1 1",
		"click 1",
		"modclick Shift_L 1 10 20 20",
		"releaseall",
		"not-a-command",
		"keydown " + plantedSecret,
	}
	for _, line := range lines {
		if err := client.WriteLine(ctx, line); err != nil {
			t.Fatalf("WriteLine failed (command redacted): %v", err)
		}
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)
	logs := stderr.String()
	if strings.Contains(logs, plantedSecret) {
		t.Fatalf("leak in daemon stderr")
	}
	if !strings.Contains(logs, "xtest: <- keydown <redacted>") {
		t.Fatalf("debug log missing redacted keydown: %q", logs)
	}
}

func TestDaemonStdinReadyAndLock(t *testing.T) {
	display, stopDisplay := startXvfb(t)
	defer stopDisplay()
	bin := buildDaemon(t)

	stderr1 := &safeBuffer{}
	cmd1, addr, stop1 := startDaemon(t, bin, display, []string{"-tcp", "127.0.0.1:0"}, stderr1)
	defer stop1()
	if addr == "" {
		t.Fatal("missing tcp ready")
	}

	cmd2 := exec.Command(bin, "-tcp", "127.0.0.1:0")
	cmd2.Env = append(os.Environ(), "DISPLAY="+display)
	out, err := cmd2.CombinedOutput()
	if cmd2.ProcessState == nil || cmd2.ProcessState.ExitCode() != 75 {
		t.Fatalf("second daemon exit=%v err=%v out=%s", exitOf(cmd2), err, out)
	}
	_ = cmd1
}

func TestClientWriteAgainstRealDaemon(t *testing.T) {
	display, stopDisplay := startXvfb(t)
	defer stopDisplay()
	bin := buildDaemon(t)
	stderr := &safeBuffer{}
	_, addr, stop := startDaemon(t, bin, display, []string{"-tcp", "127.0.0.1:0"}, stderr)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := x11input.Dial(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.WriteLine(ctx, "mousemove 1 1"); err != nil {
		t.Fatal(err)
	}
}

func TestStdinReady(t *testing.T) {
	display, stopDisplay := startXvfb(t)
	defer stopDisplay()
	bin := buildDaemon(t)

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "DISPLAY="+display)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	line, err := readLine(stdout, 5*time.Second)
	if err != nil {
		t.Fatalf("stdin READY: %v stderr=%s", err, stderr.String())
	}
	if line != "READY" {
		t.Fatalf("stdin handshake %q", line)
	}
}

var (
	daemonOnce sync.Once
	daemonBin  string
	daemonErr  error
)

func buildDaemon(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not on PATH")
	}
	daemonOnce.Do(func() {
		dir, err := os.MkdirTemp("", "xtest-server-build-")
		if err != nil {
			daemonErr = err
			return
		}
		bin := filepath.Join(dir, "xtest-server")
		cmd := exec.Command("go", "build", "-o", bin, "github.com/Diobyte/x11_go_dockerinput_daemon/cmd/xtest-server")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=1", "CC=gcc")
		out, err := cmd.CombinedOutput()
		if err != nil {
			daemonErr = fmt.Errorf("build xtest-server: %w\n%s", err, out)
			return
		}
		daemonBin = bin
	})
	if daemonErr != nil {
		t.Fatal(daemonErr)
	}
	return daemonBin
}

func startXvfb(t *testing.T) (string, func()) {
	t.Helper()
	xvfb := lookPath("Xvfb", "/tmp/xvfb-root/usr/bin/Xvfb")
	if xvfb == "" {
		t.Skip("Xvfb not available")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	proc := exec.Command(xvfb, "-displayfd", "3", "-screen", "0", "1280x720x24", "-nolisten", "tcp", "-ac")
	proc.Stderr = io.Discard
	proc.ExtraFiles = []*os.File{w}
	if err := proc.Start(); err != nil {
		_ = r.Close()
		_ = w.Close()
		t.Skipf("Xvfb start: %v", err)
	}
	_ = w.Close()
	stop := func() {
		if proc.Process != nil {
			_ = proc.Process.Kill()
			_, _ = proc.Process.Wait()
		}
	}
	t.Cleanup(stop)
	_ = r.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := readLine(r, 3*time.Second)
	_ = r.Close()
	if err != nil {
		stop()
		t.Skipf("Xvfb displayfd: %v", err)
	}
	if _, err := strconv.Atoi(n); err != nil {
		stop()
		t.Skipf("Xvfb displayfd %q", n)
	}
	return ":" + n, stop
}

func startDaemon(t *testing.T, bin, display string, args []string, stderr io.Writer) (*exec.Cmd, string, func()) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "DISPLAY="+display)
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stop := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}
	t.Cleanup(stop)
	line, err := readLine(stdout, 5*time.Second)
	if err != nil {
		stop()
		t.Fatalf("daemon READY: %v stderr=%s", err, readString(stderr))
	}
	if !strings.HasPrefix(line, "READY tcp:") {
		stop()
		t.Fatalf("daemon handshake %q", line)
	}
	port := strings.TrimPrefix(line, "READY tcp:")
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatalf("port %q", port)
	}
	return cmd, net.JoinHostPort("127.0.0.1", port), stop
}

func dialRaw(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func readLine(r io.Reader, d time.Duration) (string, error) {
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		sc := bufio.NewScanner(r)
		if !sc.Scan() {
			ch <- result{err: sc.Err()}
			return
		}
		ch <- result{line: sc.Text()}
	}()
	select {
	case r := <-ch:
		if r.line == "" && r.err == nil {
			return "", io.EOF
		}
		return r.line, r.err
	case <-time.After(d):
		return "", context.DeadlineExceeded
	}
}

func lookPath(names ...string) string {
	for _, n := range names {
		if n == "" {
			continue
		}
		if filepath.IsAbs(n) {
			if st, err := os.Stat(n); err == nil && !st.IsDir() {
				return n
			}
			continue
		}
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func exitOf(cmd *exec.Cmd) int {
	if cmd.ProcessState == nil {
		return -1
	}
	return cmd.ProcessState.ExitCode()
}

func readString(w io.Writer) string {
	if b, ok := w.(*safeBuffer); ok {
		return b.String()
	}
	return ""
}

type safeBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
