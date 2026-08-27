package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func startXvfb(t *testing.T) (string, func()) {
	t.Helper()
	xvfb := lookPath("Xvfb", "/tmp/xvfb-root/usr/bin/Xvfb")
	if xvfb == "" {
		t.Skip("Xvfb not available (private DISPLAY required; never use host Xwayland)")
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
	sc := bufio.NewScanner(r)
	if !sc.Scan() {
		stop()
		_ = r.Close()
		t.Skipf("Xvfb displayfd: %v", sc.Err())
	}
	_ = r.Close()
	n := sc.Text()
	if _, err := strconv.Atoi(n); err != nil {
		stop()
		t.Skipf("Xvfb displayfd %q", n)
	}
	return ":" + n, stop
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
