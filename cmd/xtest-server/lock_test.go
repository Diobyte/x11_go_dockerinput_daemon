package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDisplayLockNameSanitizes(t *testing.T) {
	t.Setenv("DISPLAY", ":99")
	got := displayLockName()
	if got != "x11-input--99.lock" {
		t.Fatalf("got %q", got)
	}
	paths := lockPaths()
	if paths[0] != "/run/x11-input--99.lock" || paths[1] != "/tmp/x11-input--99.lock" {
		t.Fatalf("%v", paths)
	}
}

func TestOpenExplicitLockFileRejectsUnsafePaths(t *testing.T) {
	privateDir := privateTempDir(t)
	good := filepath.Join(privateDir, "authority.lock")
	if err := os.WriteFile(good, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	wrongMode := filepath.Join(privateDir, "wrong-mode.lock")
	if err := os.WriteFile(wrongMode, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Chmod(wrongMode, 0o640); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(privateDir, "directory.lock")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(privateDir, "symlink.lock")
	if err := os.Symlink(good, symlink); err != nil {
		t.Fatal(err)
	}

	nonPrivateDir := t.TempDir()
	if err := syscall.Chmod(nonPrivateDir, 0o750); err != nil {
		t.Fatal(err)
	}
	nonPrivate := filepath.Join(nonPrivateDir, "authority.lock")
	if err := os.WriteFile(nonPrivate, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	parentTarget := privateTempDir(t)
	parentLink := filepath.Join(privateDir, "parent-link")
	if err := os.Symlink(parentTarget, parentLink); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(parentLink, "authority.lock")
	if err := os.WriteFile(filepath.Join(parentTarget, "authority.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "relative", path: "authority.lock"},
		{name: "unclean", path: privateDir + "/./authority.lock"},
		{name: "missing", path: filepath.Join(privateDir, "missing.lock")},
		{name: "wrong mode", path: wrongMode},
		{name: "directory", path: directory},
		{name: "symlink", path: symlink},
		{name: "non-private parent", path: nonPrivate},
		{name: "symlink parent", path: linkedParent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := openExplicitLockFile(test.path)
			if file != nil {
				_ = file.Close()
				t.Fatal("unsafe explicit lock opened")
			}
			if !errors.Is(err, errLockConfig) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestExplicitSingletonLockContendsOnSharedInode(t *testing.T) {
	dir := privateTempDir(t)
	path := filepath.Join(dir, "authority.lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "authority-alias.lock")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}

	first, err := acquireSingletonLock(path)
	if err != nil {
		t.Fatal(err)
	}

	second, err := openExplicitLockFile(alias)
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}
	if err := takeSingletonLock(second, 1, 0); !errors.Is(err, errLockHeld) {
		_ = first.Close()
		t.Fatalf("shared inode contention got %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	third, err := acquireSingletonLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitLockFailureDoesNotFallBack(t *testing.T) {
	t.Setenv("DISPLAY", ":explicit-lock-no-fallback")
	dir := privateTempDir(t)
	_, err := acquireSingletonLock(filepath.Join(dir, "missing.lock"))
	if !errors.Is(err, errLockConfig) {
		t.Fatalf("got %v", err)
	}
	for _, path := range lockPaths() {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("explicit failure created fallback %s", path)
		}
	}
}
