package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const exitLockHeld = 75

const (
	lockAttempts = 20
	lockRetry    = 100 * time.Millisecond
)

var (
	errLockConfig      = errors.New("invalid singleton lock configuration")
	errLockHeld        = errors.New("singleton lock held")
	errLockUnavailable = errors.New("singleton lock unavailable")
)

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

func openDefaultLockFile(path string) (*os.File, error) {
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

func openExplicitLockFile(path string) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("%w: path must be absolute and clean", errLockConfig)
	}
	parent := filepath.Dir(path)
	fi, err := os.Lstat(parent)
	if err != nil {
		return nil, fmt.Errorf("%w: private parent is unavailable", errLockConfig)
	}
	parentStat, ok := fi.Sys().(*syscall.Stat_t)
	euid, euidOK := currentEUID()
	if !ok || !euidOK || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 ||
		parentStat.Uid != euid || fi.Mode().Perm() != 0o700 {
		return nil, fmt.Errorf("%w: parent must be owned by euid with mode 0700", errLockConfig)
	}

	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: lock file must already exist", errLockConfig)
	}
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("%w: cannot inspect lock file", errLockConfig)
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Uid != euid || st.Mode&0o7777 != 0o600 {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("%w: file must be owned by euid with mode 0600", errLockConfig)
	}
	return os.NewFile(uintptr(fd), path), nil
}

func takeSingletonLock(f *os.File, attempts int, retry time.Duration) error {
	for i := 0; i < attempts; i++ {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = f.Close()
			return fmt.Errorf("%w: flock failed", errLockUnavailable)
		}
		if i+1 < attempts {
			time.Sleep(retry)
		}
	}
	_ = f.Close()
	return errLockHeld
}

func acquireSingletonLock(explicitPath string) (*os.File, error) {
	if explicitPath != "" {
		f, err := openExplicitLockFile(explicitPath)
		if err != nil {
			return nil, err
		}
		if err := takeSingletonLock(f, lockAttempts, lockRetry); err != nil {
			return nil, err
		}
		return f, nil
	}

	var f *os.File
	for _, path := range lockPaths() {
		if candidate, err := openDefaultLockFile(path); err == nil {
			f = candidate
			break
		}
	}
	if f == nil {
		return nil, errLockUnavailable
	}
	if err := takeSingletonLock(f, lockAttempts, lockRetry); err != nil {
		return nil, err
	}
	return f, nil
}
