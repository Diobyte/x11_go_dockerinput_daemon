// Package x11input is the public v2 line client for xtest-server.
//
// It connects and writes command lines. READY is a process-stdout handshake
// ("READY" or "READY tcp:<port>"), not a per-connection greeting; TCP adopt
// matches live dialExistingTCP and does not send /healthz. A failed mutating
// write is returned to the caller and is never retried by this package.
package x11input

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// Client is a v2 line writer on a command connection.
type Client struct {
	mu    sync.Mutex
	conn  net.Conn
	ready string
}

// Dial connects over TCP. It does not read READY from the socket, does not
// send /healthz, and does not retry.
func Dial(ctx context.Context, address string) (*Client, error) {
	if ctx == nil {
		return nil, errors.New("x11input: nil context")
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("x11input: dial: %w", err)
	}
	return New(conn), nil
}

// New wraps an existing command connection (TCP or pipe). The caller observes
// process-stdout READY with WaitReady when they launched the daemon.
func New(conn net.Conn) *Client {
	return &Client{conn: conn}
}

// WaitReady reads one handshake line from daemon stdout. It accepts only
// v2 "READY" and "READY tcp:<port>". Destination stderr identity
// ("xtest-server: mode dest unix …") is not a v2 handshake.
func WaitReady(ctx context.Context, r io.Reader) (string, error) {
	if ctx == nil {
		return "", errors.New("x11input: nil context")
	}
	if r == nil {
		return "", errors.New("x11input: nil reader")
	}
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		sc := bufio.NewScanner(r)
		if !sc.Scan() {
			err := sc.Err()
			if err == nil {
				err = io.EOF
			}
			ch <- result{err: err}
			return
		}
		ch <- result{line: sc.Text()}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return "", fmt.Errorf("x11input: read READY: %w", res.err)
		}
		if res.line != "READY" && !strings.HasPrefix(res.line, "READY tcp:") {
			return "", errors.New("x11input: unexpected handshake")
		}
		return res.line, nil
	}
}

// Ready is the last WaitReady line attached with SetReady, if any.
func (c *Client) Ready() string {
	if c == nil {
		return ""
	}
	return c.ready
}

// SetReady records a process-stdout handshake observed by the caller.
func (c *Client) SetReady(ready string) {
	if c == nil {
		return
	}
	c.ready = ready
}

// WriteLine writes one v2 command followed by a newline. It does not retry
// after a short write or connection error: the daemon may have already
// executed a prefix of the line.
func (c *Client) WriteLine(ctx context.Context, line string) error {
	if ctx == nil {
		return errors.New("x11input: nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return errors.New("x11input: nil client")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return net.ErrClosed
	}
	trimmed := strings.TrimRight(line, "\n")
	if strings.ContainsAny(trimmed, "\n\x00") {
		return errors.New("x11input: line must be a single command")
	}
	if len(trimmed) > 1<<20 {
		return errors.New("x11input: line too long")
	}
	payload := trimmed + "\n"
	if deadline, ok := ctx.Deadline(); ok {
		if err := c.conn.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("x11input: write deadline: %w", err)
		}
		defer func() { _ = c.conn.SetWriteDeadline(time.Time{}) }()
	}
	n, err := c.conn.Write([]byte(payload))
	if err != nil {
		return fmt.Errorf("x11input: write: %w", err)
	}
	if n != len(payload) {
		return fmt.Errorf("x11input: write: %w", io.ErrShortWrite)
	}
	return nil
}

// Close closes the command connection. The daemon releases held keys/buttons
// on command-connection EOF.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}
