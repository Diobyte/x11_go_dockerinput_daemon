// Package main provides the bounded X11/XTEST input daemon.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"

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
	}

	lock, ok := acquireSingletonLock()
	if !ok {
		fmt.Fprintln(os.Stderr, "xtest-server: another instance already holds the singleton lock; exiting")
		os.Exit(exitLockHeld)
	}
	defer func() { _ = lock.Close() }()

	if mode == vnext.ModeVNext {
		ensureDestOwner()
		var rc int
		destOwner.call(func() {
			xmu.Lock()
			defer xmu.Unlock()
			rc = initDisplay()
		})
		exitIfDisplayInit(rc)
		serveVNext(listenAddr, allow, gids)
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
}
