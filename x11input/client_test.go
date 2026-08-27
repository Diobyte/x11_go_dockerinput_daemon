package x11input_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Diobyte/x11_go_dockerinput_daemon/x11input"
)

func TestDialWritesOnceWithoutReadyOnSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var writes atomic.Int32
	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer c.Close()
		buf := make([]byte, 64)
		n, err := c.Read(buf)
		if err != nil {
			errCh <- err
			return
		}
		writes.Add(1)
		if string(buf[:n]) != "mousemove 1 1\n" {
			errCh <- errors.New("unexpected payload")
			return
		}
		errCh <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := x11input.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.WriteLine(ctx, "mousemove 1 1"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("server did not observe write")
	}
	if writes.Load() != 1 {
		t.Fatalf("writes=%d want 1", writes.Load())
	}
}

func TestWriteLineDoesNotRetry(t *testing.T) {
	server, clientConn := net.Pipe()
	defer server.Close()

	go func() {
		buf := make([]byte, 32)
		_, _ = server.Read(buf)
		_ = server.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := x11input.New(clientConn)
	defer client.Close()

	if err := client.WriteLine(ctx, "mousemove 1 1"); err != nil {
		t.Fatalf("first write should succeed: %v", err)
	}
	err := client.WriteLine(ctx, "mousemove 2 2")
	if err == nil {
		t.Fatal("second write must fail after peer close, without retry")
	}
	if strings.Contains(err.Error(), "mousemove 2 2") {
		t.Fatalf("error must not contain payload: %v", err)
	}
}

func TestWaitReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := x11input.WaitReady(ctx, strings.NewReader("READY tcp:9999\n"))
	if err != nil || got != "READY tcp:9999" {
		t.Fatalf("WaitReady=%q %v", got, err)
	}
	_, err = x11input.WaitReady(ctx, strings.NewReader("HELLO\n"))
	if err == nil {
		t.Fatal("expected handshake error")
	}
	_, err = x11input.WaitReady(ctx, strings.NewReader("READY unix:/run/xtest.sock\n"))
	if err == nil {
		t.Fatal("dest unix READY must not satisfy WaitReady")
	}
}

func TestWriteLineRejectsEmbeddedNewline(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client := x11input.New(a)
	defer client.Close()
	if err := client.WriteLine(ctx, "keydown F1\nkeyup F1"); err == nil {
		t.Fatal("embedded newline must fail")
	}
}
