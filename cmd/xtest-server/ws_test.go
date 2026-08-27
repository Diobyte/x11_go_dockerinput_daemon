package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Diobyte/x11_go_dockerinput_daemon/internal/v2line"
)

func TestConnReader_Prefix(t *testing.T) {
	data := []byte("hello world")
	cr := &connReader{prefix: data[:5], r: bytes.NewReader(data[5:])}
	out := make([]byte, len(data))
	n, err := io.ReadFull(cr, out)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Fatalf("read %d bytes, want %d", n, len(data))
	}
	if string(out) != "hello world" {
		t.Fatalf("got %q, want %q", out, "hello world")
	}
}

func TestConnReader_NoPrefix(t *testing.T) {
	data := []byte("no prefix")
	cr := &connReader{r: bytes.NewReader(data)}
	out := make([]byte, len(data))
	n, err := io.ReadFull(cr, out)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) || string(out) != "no prefix" {
		t.Fatalf("got %q", out)
	}
}

func TestWsConn_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	wc := &wsConn{r: &buf, w: &buf}

	msg := []byte("mousemove 640 360")
	n, err := wc.Write(msg)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(msg) {
		t.Fatalf("wrote %d, want %d", n, len(msg))
	}

	out := make([]byte, len(msg))
	n, err = io.ReadFull(wc, out)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(msg) {
		t.Fatalf("roundtrip: got %q, want %q", out, string(msg))
	}
}

func TestWsConn_MultipleFrames(t *testing.T) {
	var buf bytes.Buffer
	wc := &wsConn{r: &buf, w: &buf}

	messages := []string{"keydown F1", "keyup F1", "click 1", "mousemove 100 200"}
	for _, msg := range messages {
		if _, err := wc.Write([]byte(msg)); err != nil {
			t.Fatal(err)
		}
	}

	for _, want := range messages {
		out := make([]byte, len(want))
		n, err := io.ReadFull(wc, out)
		if err != nil {
			t.Fatalf("read frame %q: %v", want, err)
		}
		if string(out[:n]) != want {
			t.Fatalf("got %q, want %q", out[:n], want)
		}
	}
}

func TestWsConn_PingPong(t *testing.T) {
	var buf bytes.Buffer
	wc := &wsConn{r: &buf, w: &buf}

	ping := []byte{0x89, 0x00}
	buf.Write(ping)

	msg := []byte("hello")
	_, _ = wc.Write(msg)

	out := make([]byte, len(msg))
	_, _ = io.ReadFull(wc, out)
	if string(out) != "hello" {
		t.Fatalf("ping interfered: got %q", out)
	}
}

func TestTryUpgradeWS_PlainTCP(t *testing.T) {
	data := "keydown F1\n"
	r := strings.NewReader(data)
	var w bytes.Buffer

	reader, writer, isWS, handled := tryUpgradeWS(r, &w)
	if isWS {
		t.Fatal("plain TCP should not be upgraded")
	}
	if handled {
		t.Fatal("plain TCP command was treated as an HTTP control request")
	}
	if reader == nil || writer == nil {
		t.Fatal("plain TCP should return valid reader/writer")
	}

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != data {
		t.Fatalf("got %q, want %q", out, data)
	}
}

func TestTryUpgradeWS_WebSocket(t *testing.T) {
	wsKey := base64.StdEncoding.EncodeToString([]byte("the sample nonce"))
	req := "GET / HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + wsKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	r := strings.NewReader(req)
	var w bytes.Buffer

	reader, writer, isWS, handled := tryUpgradeWS(r, &w)
	if !isWS {
		t.Fatal("WebSocket upgrade should succeed")
	}
	if handled {
		t.Fatal("WebSocket upgrade was consumed as a completed control request")
	}
	if reader == nil || writer == nil {
		t.Fatal("WebSocket should return valid reader/writer")
	}

	resp := w.String()
	if !strings.Contains(resp, "101") {
		t.Fatalf("expected 101 Switching Protocols, got: %s", resp)
	}

	sum := sha1.Sum([]byte(wsKey + wsMagic)) //nolint:gosec // RFC6455
	expectedAccept := base64.StdEncoding.EncodeToString(sum[:])
	if !strings.Contains(resp, expectedAccept) {
		t.Fatalf("expected Sec-WebSocket-Accept: %s, got: %s", expectedAccept, resp)
	}
}

func TestTryUpgradeWS_ShortRead(t *testing.T) {
	r := strings.NewReader("GE")
	var w bytes.Buffer
	_, _, isWS, handled := tryUpgradeWS(r, &w)
	if isWS {
		t.Fatal("short read should not upgrade")
	}
	if !handled {
		t.Fatal("closed short read must be handled without constructing a nil scanner")
	}
}

func TestTryUpgradeWS_CurrentHealthContract(t *testing.T) {
	req := "GET " + protocolHealthPath + " HTTP/1.1\r\nHost: localhost\r\n\r\n"
	var response bytes.Buffer
	reader, writer, isWS, handled := tryUpgradeWS(strings.NewReader(req), &response)
	if isWS || !handled || reader != nil || writer != nil {
		t.Fatalf("health negotiation = reader:%v writer:%v ws:%v handled:%v", reader, writer, isWS, handled)
	}
	if got := response.String(); !strings.Contains(got, "HTTP/1.1 200 OK\r\n") || !strings.HasSuffix(got, "\r\n\r\n"+protocolHealthBody) {
		t.Fatalf("health response does not publish exact current contract: %q", got)
	}
}

func TestParseKeyHoldIsShippedDecoder(t *testing.T) {
	got, ok := v2line.ParseKeyHold(strings.Fields("key F1 20"))
	if !ok || got != 20*time.Millisecond {
		t.Fatalf("daemon must use v2line.ParseKeyHold; got %v %v", got, ok)
	}
}
