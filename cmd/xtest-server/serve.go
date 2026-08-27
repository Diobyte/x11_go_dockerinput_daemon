package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

func serveScanner(sc *bufio.Scanner) {
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		handleCommand(sc.Text())
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "xtest-server: scanner error: %v\n", err)
	}
}

func serveTCP(addr string) {
	var ln net.Listener
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		ln, err = net.Listen("tcp", addr) //nolint:noctx // live v2 listen has no deadline/context
		if err == nil {
			break
		}
		if attempt < 3 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "xtest-server: tcp listen %s: %v\n", addr, err)
		os.Exit(1)
	}
	fmt.Println(tcpReadyLine(ln.Addr().(*net.TCPAddr).Port))

	const maxConns = 10
	connSem := make(chan struct{}, maxConns)

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "xtest-server: accept error: %v\n", err)
			continue
		}
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(30 * time.Second)
		}
		select {
		case connSem <- struct{}{}:
		default:
			fmt.Fprintf(os.Stderr, "xtest-server: connection limit reached; dropping\n")
			_ = conn.Close()
			continue
		}
		go func(c net.Conn) {
			defer func() { <-connSem }()
			defer c.Close()

			r, w, isWS, handled := tryUpgradeWS(c, c)
			if handled {
				return
			}
			if isWS {
				serveScannerWithAck(bufio.NewScanner(r), w)
			} else {
				serveScanner(bufio.NewScanner(r))
			}
			releaseAllHeld()
		}(conn)
	}
}
