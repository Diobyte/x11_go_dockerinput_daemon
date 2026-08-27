package vnext

import (
	"strings"
	"sync"
	"testing"
)

func TestSessionReleaseOnlyOwnHolds(t *testing.T) {
	f := NewFake()
	a := NewSession()
	b := NewSession()
	if out := HandleLine(`{"op":"key","name":"F1","press":true}`, a, f); !out.Submitted() {
		t.Fatalf("A key: %+v", out)
	}
	if out := HandleLine(`{"op":"key","name":"Shift_L","press":true}`, b, f); !out.Submitted() {
		t.Fatalf("B key: %+v", out)
	}
	if out := HandleLine(`{"op":"button","button":1,"press":true}`, a, f); !out.Submitted() {
		t.Fatalf("A button: %+v", out)
	}
	out := a.Release(f)
	if !out.Submitted() {
		t.Fatalf("release A: %+v", out)
	}
	if a.KeyDown(67) || a.ButtonDown(1) {
		t.Fatal("A still tracking holds")
	}
	if !b.KeyDown(50) {
		t.Fatal("B Shift_L was released by A's close")
	}
	got := strings.Join(f.Snapshot(), " ")
	if !strings.Contains(got, "key:67:up") || !strings.Contains(got, "button:1:up") {
		t.Fatalf("missing A's ups: %q", got)
	}
	if strings.Contains(got, "key:50:up") {
		t.Fatalf("released B's key: %q", got)
	}
	if strings.Contains(got, "releaseall") {
		t.Fatalf("destination used global releaseall: %q", got)
	}
}

func TestHandleLineReleaseUsesSessionNotGlobal(t *testing.T) {
	f := NewFake()
	a := NewSession()
	b := NewSession()
	HandleLine(`{"op":"key","name":"F1","press":true}`, a, f)
	HandleLine(`{"op":"key","name":"Shift_L","press":true}`, b, f)
	out := HandleLine(`{"op":"release"}`, a, f)
	if !out.Submitted() {
		t.Fatalf("%+v", out)
	}
	if b.KeyDown(50) == false {
		t.Fatal("sibling session cleared")
	}
}

func TestConcurrentTwoSessionHandleLine(t *testing.T) {
	f := NewFake()
	a := NewSession()
	b := NewSession()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		HandleLine(`{"op":"key","name":"F1","press":true}`, a, f)
	}()
	go func() {
		defer wg.Done()
		HandleLine(`{"op":"key","name":"Shift_L","press":true}`, b, f)
	}()
	wg.Wait()
	HandleLine(`{"op":"release"}`, a, f)
	if !b.KeyDown(50) {
		t.Fatal("concurrent release crossed sessions")
	}
	got := strings.Join(f.Snapshot(), " ")
	if strings.Contains(got, "releaseall") {
		t.Fatal("global releaseall")
	}
}

func TestSessionReleaseKeepsHoldIfUpFails(t *testing.T) {
	f := &failUpFake{Fake: NewFake(), failKey: 67}
	sess := NewSession()
	if out := HandleLine(`{"op":"key","name":"F1","press":true}`, sess, f); !out.Submitted() {
		t.Fatalf("down: %+v", out)
	}
	if out := HandleLine(`{"op":"key","name":"Shift_L","press":true}`, sess, f); !out.Submitted() {
		t.Fatalf("shift down: %+v", out)
	}
	out := sess.Release(f)
	if out.Code != CodeUnavailable {
		t.Fatalf("release code %q", out.Code)
	}
	if !sess.KeyDown(67) {
		t.Fatal("failed F1 up deleted the hold")
	}
	if sess.KeyDown(50) {
		t.Fatal("Shift_L should have been released after F1 failure (best-effort rest)")
	}
	if f.keyUps[67] != 1 {
		t.Fatalf("retried F1 up: %d", f.keyUps[67])
	}
}

type failUpFake struct {
	*Fake
	failKey uint
	keyUps  map[uint]int
}

func (f *failUpFake) SendKey(keycode uint, press bool) error {
	if !press {
		if f.keyUps == nil {
			f.keyUps = map[uint]int{}
		}
		f.keyUps[keycode]++
		if keycode == f.failKey && f.keyUps[keycode] == 1 {
			return errSentinel
		}
	}
	return f.Fake.SendKey(keycode, press)
}
