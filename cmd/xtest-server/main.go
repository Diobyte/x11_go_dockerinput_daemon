// Package main provides the bounded X11/XTEST input daemon.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/Diobyte/x11_go_dockerinput_daemon/internal/vnext"
)

var buildRevision = "unknown"
var buildDirty = "unknown"
var buildVersion = "devel"

func exitIfDisplayInit(rc int) {
	if rc == 0 {
		return
	}
	// -2 already logged from Xlib: Xwayland / host-seat EI portal.
	if rc != -2 {
		fmt.Fprintln(os.Stderr, "xtest-server: failed to open X11 display")
	}
	os.Exit(1)
}

func main() {
	debug := flag.Bool("debug", false, "log every command to stderr (matches x11vnc -debug_pointer/keyboard)")
	tcpAddr := flag.String("tcp", "", "listen on TCP address:port (legacy v2)")
	wsAddr := flag.String("ws", "", "listen on WebSocket address:port (aliases -tcp, then XOR with -vnext)")
	vnextSpec := flag.String("vnext", "", "destination listen unix:/abs/path (XOR with -tcp and -ws; default empty)")
	allowSpec := flag.String("vnext-allow", "euid", "dest unix peer UIDs (euid or comma list; empty does not listen)")
	gidSpec := flag.String("vnext-allow-gid", "", "dest unix peer GIDs (egid or comma list; empty allows any GID of an allowlisted UID)")
	lockFile := flag.String("lock-file", "", "pre-created shared singleton lock file (absolute path; no fallback)")
	showVersion := flag.Bool("version", false, "print build identity")
	flag.Parse()
	if *showVersion {
		fmt.Printf("xtest-server version=%s revision=%s dirty=%s protocols=v2,dest-c3 backend=x11\n", buildVersion, buildRevision, buildDirty)
		return
	}
	debugLog = *debug

	mode, listenAddr, err := vnext.AdmitFlags(*tcpAddr, *wsAddr, *vnextSpec)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var allow, gids []uint32
	if mode == vnext.ModeVNext {
		allow, err = vnext.ParseAllowlist(*allowSpec)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		gids, err = vnext.ParseGIDAllowlist(*gidSpec)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if *lockFile != "" && *lockFile == listenAddr {
			fmt.Fprintln(os.Stderr, "xtest-server: lock file and Unix socket must differ")
			os.Exit(2)
		}
	}

	lock, err := acquireSingletonLock(*lockFile)
	if errors.Is(err, errLockHeld) {
		fmt.Fprintln(os.Stderr, "xtest-server: another instance already holds the singleton lock; exiting")
		os.Exit(exitLockHeld)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "xtest-server: singleton lock: %v\n", err)
		os.Exit(1)
	}
	if mode == vnext.ModeVNext {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		ensureDestOwner()
		var rc int
		destOwner.call(func() {
			xmu.Lock()
			defer xmu.Unlock()
			rc = initDisplay()
		})
		exitIfDisplayInit(rc)
		err = serveVNext(ctx, listenAddr, allow, gids)
		if errors.Is(err, errDestShutdownTimeout) {
			fmt.Fprintln(os.Stderr, "xtest-server: shutdown timed out before session cleanup completed")
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "xtest-server: destination server: %v\n", err)
			os.Exit(1)
		}
		stop()
		if err := lock.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "xtest-server: closing singleton lock")
		}
		return
	}
	exitIfDisplayInit(initDisplay())
	if listenAddr != "" {
		serveTCP(listenAddr)
	} else {
		fmt.Println(stdinReadyLine())
		serveScanner(bufio.NewScanner(os.Stdin))
		releaseAllHeld()
	}
	// Keep the file reachable while a long-lived legacy listener is serving.
	runtime.KeepAlive(lock)
	if err := lock.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "xtest-server: closing singleton lock")
	}
}
