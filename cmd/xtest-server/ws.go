package main

import (
	"bufio"
	"crypto/sha1" //nolint:gosec // RFC6455 magic is SHA-1
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	protocolHealthPath = "/healthz"
	protocolHealthBody = "x11-input/2\n"
)

var (
	wsMagic            = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	errWSFrameTooLarge = errors.New("websocket frame exceeds 1 MiB")
	errWSControlLarge  = errors.New("websocket control frame exceeds 125 bytes")
)

func tryUpgradeWS(r io.Reader, w io.Writer) (io.Reader, io.Writer, bool, bool) {
	buf := make([]byte, 4)
	n, err := io.ReadFull(r, buf)
	if err != nil || n < 4 {
		return nil, nil, false, true
	}
	cr := &connReader{buf[:n], r}

	if string(buf) != "GET " {
		return cr, w, false, false
	}

	sc := bufio.NewScanner(cr)
	sc.Buffer(make([]byte, 0, 4096), 4096)
	requestLine := ""
	var wsKey string
	headersComplete := false
	for sc.Scan() {
		line := sc.Text()
		if requestLine == "" {
			requestLine = line
			continue
		}
		if line == "" {
			headersComplete = true
			break
		}
		if strings.HasPrefix(line, "Sec-WebSocket-Key:") {
			wsKey = strings.TrimSpace(line[len("Sec-WebSocket-Key:"):])
		}
	}
	if sc.Err() != nil || !headersComplete {
		return nil, nil, false, true
	}
	if requestLine == "GET "+protocolHealthPath+" HTTP/1.1" {
		resp := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(protocolHealthBody), protocolHealthBody)
		_, _ = io.WriteString(w, resp)
		return nil, nil, false, true
	}
	if wsKey == "" {
		return nil, nil, false, true
	}

	sum := sha1.Sum([]byte(wsKey + wsMagic)) //nolint:gosec // RFC6455
	accept := base64.StdEncoding.EncodeToString(sum[:])
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := io.WriteString(w, resp); err != nil {
		return nil, nil, false, true
	}

	wc := &wsConn{r: cr, w: w}
	return wc, wc, true, false
}

type connReader struct {
	prefix []byte
	r      io.Reader
}

func (cr *connReader) Read(p []byte) (int, error) {
	if len(cr.prefix) > 0 {
		n := copy(p, cr.prefix)
		cr.prefix = cr.prefix[n:]
		return n, nil
	}
	return cr.r.Read(p)
}

type wsConn struct {
	r io.Reader
	w io.Writer
}

func (wc *wsConn) Read(p []byte) (int, error) {
	for {
		hdr := make([]byte, 2)
		if _, err := io.ReadFull(wc.r, hdr); err != nil {
			return 0, err
		}
		opcode := hdr[0] & 0x0F
		masked := hdr[1]&0x80 != 0
		payLen := uint64(hdr[1] & 0x7F)

		switch payLen {
		case 126:
			ext := make([]byte, 2)
			if _, err := io.ReadFull(wc.r, ext); err != nil {
				return 0, err
			}
			payLen = uint64(binary.BigEndian.Uint16(ext))
		case 127:
			ext := make([]byte, 8)
			if _, err := io.ReadFull(wc.r, ext); err != nil {
				return 0, err
			}
			payLen = binary.BigEndian.Uint64(ext)
		}
		if payLen > 1<<20 {
			return 0, errWSFrameTooLarge
		}
		if opcode >= 0x08 && payLen > 125 {
			return 0, errWSControlLarge
		}

		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(wc.r, mask[:]); err != nil {
				return 0, err
			}
		}

		payload := make([]byte, int(payLen))
		if payLen > 0 {
			if _, err := io.ReadFull(wc.r, payload); err != nil {
				return 0, err
			}
		}

		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}

		switch opcode {
		case 0x01:
			return copy(p, payload), nil
		case 0x08:
			return 0, io.EOF
		case 0x09:
			pong := make([]byte, 2+len(payload))
			pong[0] = 0x8A
			pong[1] = hdr[1] & 0x7F
			copy(pong[2:], payload)
			_, _ = wc.w.Write(pong)
			continue
		default:
			continue
		}
	}
}

func (wc *wsConn) Write(p []byte) (int, error) {
	n := len(p)
	frame := make([]byte, 0, 2+8+n)
	frame = append(frame, 0x81)
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(n))
	switch {
	case n < 126:
		frame = append(frame, encoded[7])
	case n < 65536:
		frame = append(frame, 126)
		frame = append(frame, encoded[6:]...)
	default:
		frame = append(frame, 127)
		frame = append(frame, encoded[:]...)
	}
	frame = append(frame, p...)
	_, err := wc.w.Write(frame)
	return n, err
}

func serveScannerWithAck(sc *bufio.Scanner, w io.Writer) {
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		handleCommand(sc.Text())
		_, _ = io.WriteString(w, "ok\n")
	}
}
