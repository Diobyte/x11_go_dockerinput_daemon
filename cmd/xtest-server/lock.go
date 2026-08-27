package main

import (
	"math"
	"os"
	"strings"
	"syscall"
	"time"
)

const exitLockHeld = 75

func displayLockName() string {
	d := os.Getenv("DISPLAY")
	if d == "" {
		d = "default"
	}
	var b strings.Builder
	b.WriteString("x11-input-")
	for _, r := range d {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	b.WriteString(".lock")
	return b.String()
}

func lockPaths() []string {
	name := displayLockName()
	return []string{"/run/" + name, "/tmp/" + name}
}

func openLockFile(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, err
	}
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	euid, ok := currentEUID()
	if !ok || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Uid != euid {
		_ = syscall.Close(fd)
		return nil, syscall.EPERM
	}
	return os.NewFile(uintptr(fd), path), nil
}

func currentEUID() (uint32, bool) {
	id := os.Geteuid()
	if id < 0 || uint64(id) > math.MaxUint32 {
		return 0, false
	}
	return uint32(id), true
}

func acquireSingletonLock() (*os.File, bool) {
	var f *os.File
	for _, p := range lockPaths() {
		if fh, err := openLockFile(p); err == nil {
			f = fh
			break
		}
	}
	if f == nil {
		return nil, false
	}
	for i := 0; i < 20; i++ {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return f, true
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = f.Close()
	return nil, false
}
