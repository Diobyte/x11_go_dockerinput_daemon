package vnext

import (
	"strings"
	"testing"
)

func TestHandleLineUnknownNeverMutates(t *testing.T) {
	out := HandleLine(`{"op":"explode"}`, NewSession(), PanicBackend{})
	if out.Code != CodeNotSubmitted {
		t.Fatalf("unknown op: got %q want %s", out.Code, CodeNotSubmitted)
	}
	for _, line := range []string{`{}`} {
		out := HandleLine(line, NewSession(), PanicBackend{})
		if out.Code != CodeNotSubmitted && out.Code != CodeInvalidRequest {
			t.Fatalf("empty object: got %q", out.Code)
		}
		if out.Code == CodeSubmitted {
			t.Fatal("empty/unknown submitted")
		}
	}
}

func TestHandleLineIncompleteNeverMutates(t *testing.T) {
	cases := []string{
		"",
		`{`,
		`{"op":"move"}`,
		`{"op":"move","x":1}`,
		`{"op":"key","name":"F1"}`,
		`{"op":"button","button":1}`,
		`{"op":"activate","xid":1}`,
		`{"op":"fullscreen","xid":1,"displayGen":1,"observeGen":1}`,
		`{"op":"move","x":1,"y":2,"extra":true}`,
		`{"op":"move","x":1,"y":2}{"op":"move","x":3,"y":4}`,
		`{"op":"move","x":1,"y":2}]`,
		`{"op":"explode","op":"move","x":1,"y":2}`,
		`{"OP":"move","x":1,"y":2}`,
		`{"op":"inspect","x":1,"y":2}`,
		`{"op":"button","button":0,"press":true}`,
		`{"op":"button","button":4,"press":true}`,
		`{"op":"move","x":1,"y":2,"name":null}`,
		`{"op":"move","x":1,"y":2,"name":""}`,
		`{"op":"inspect","add":null}`,
		`{"op":"key","name":"F1","press":true,"x":null}`,
		`null`,
	}
	for i, line := range cases {
		out := HandleLine(line, NewSession(), PanicBackend{})
		if out.Code != CodeInvalidRequest {
			t.Fatalf("incomplete case %d: got %q want %s", i, out.Code, CodeInvalidRequest)
		}
	}
}

func TestHandleLineValidMoveKeyButtonAfterValidation(t *testing.T) {
	f := NewFake()
	sess := NewSession()
	if out := HandleLine(`{"op":"move","x":10,"y":20}`, sess, f); !out.Submitted() {
		t.Fatalf("move: %+v", out)
	}
	if out := HandleLine(`{"op":"key","name":"F1","press":true}`, sess, f); !out.Submitted() {
		t.Fatalf("key: %+v", out)
	}
	if out := HandleLine(`{"op":"button","button":1,"press":true}`, sess, f); !out.Submitted() {
		t.Fatalf("button: %+v", out)
	}
	got := strings.Join(f.Snapshot(), " ")
	if got != "warp:10,20 key:67:down button:1:down" {
		t.Fatalf("calls %q", got)
	}
	if !sess.KeyDown(67) || !sess.ButtonDown(1) {
		t.Fatal("session did not track holds")
	}
}

func TestHandleLineIncompleteAfterValidDoesNotMutate(t *testing.T) {
	f := NewFake()
	sess := NewSession()
	if out := HandleLine(`{"op":"move","x":1,"y":1}`, sess, f); !out.Submitted() {
		t.Fatalf("move: %+v", out)
	}
	n := len(f.Snapshot())
	out := HandleLine(`{"op":"move","x":2}`, sess, f)
	if out.Code != CodeInvalidRequest {
		t.Fatalf("got %q", out.Code)
	}
	if len(f.Snapshot()) != n {
		t.Fatalf("incomplete mutated backend: %v", f.Snapshot())
	}
}

func TestHandleLineDoesNotRetryBackendError(t *testing.T) {
	f := &onceFailFake{Fake: NewFake()}
	out := HandleLine(`{"op":"move","x":1,"y":1}`, NewSession(), f)
	if out.Err == nil {
		t.Fatal("expected backend error")
	}
	if out.Code != CodeUnavailable {
		t.Fatalf("backend error code %q want %s", out.Code, CodeUnavailable)
	}
	if out.Submitted() {
		t.Fatal("failed backend must not report Submitted")
	}
	if f.warps != 1 {
		t.Fatalf("retried warp: %d", f.warps)
	}
}

type onceFailFake struct {
	*Fake
	warps int
}

func (o *onceFailFake) WarpMouse(_, _ int) error {
	o.warps++
	return errSentinel
}

var errSentinel = errString("backend failed once")

type errString string

func (e errString) Error() string { return string(e) }

func TestHandleLineUnavailableNotConflict(t *testing.T) {
	b := unavailFake{Fake: NewFake()}
	for _, line := range []string{
		`{"op":"activate","displayGen":1,"observeGen":1,"xid":42}`,
		`{"op":"fullscreen","displayGen":1,"observeGen":1,"xid":42,"add":true}`,
		`{"op":"inspect"}`,
	} {
		b.Calls = nil
		out := HandleLine(line, NewSession(), b)
		if out.Code != CodeUnavailable {
			t.Fatalf("%s: got %q want %s", line, out.Code, CodeUnavailable)
		}
		if len(b.Snapshot()) != 0 {
			t.Fatalf("%s: unavailable mutated", line)
		}
	}
}

type unavailFake struct{ *Fake }

func (unavailFake) Revalidate(WindowRef) error { return ErrUnavailable }

func (unavailFake) ManagedClients() ([]WindowInfo, error) { return nil, ErrUnavailable }
