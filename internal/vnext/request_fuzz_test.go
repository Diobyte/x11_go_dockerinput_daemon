package vnext

import "testing"

func FuzzDecode(f *testing.F) {
	for _, s := range []string{
		`{"op":"move","x":1,"y":2}`,
		`{"op":"key","name":"F1","press":true}`,
		`{"op":"button","button":1,"press":false}`,
		`{"op":"inspect"}`,
		`{"op":"release"}`,
		`{"op":"activate","displayGen":1,"observeGen":1,"xid":42}`,
		`{"op":"fullscreen","displayGen":1,"observeGen":1,"xid":42,"add":true}`,
		`{"op":"explode"}`,
		`{"op":"move","x":1,"y":2}{"op":"move","x":3,"y":4}`,
		`{"op":"move","x":1,"y":2,"extra":true}`,
		`{"op":"inspect","x":1}`,
		`{"OP":"move","x":1,"y":2}`,
		`{"xid":"42"}`,
		`null`,
		`[]`,
		`{`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		req, out := Decode(line)
		switch out.Code {
		case CodeSubmitted:
			switch req.Op {
			case OpMove, OpKey, OpButton, OpInspect, OpActivate, OpFullscreen, OpRelease:
			default:
				t.Fatalf("submitted unknown op %q", req.Op)
			}
		case CodeNotSubmitted, CodeInvalidRequest:
		default:
			t.Fatalf("decode code %q", out.Code)
		}
	})
}

func FuzzHandleLine(f *testing.F) {
	f.Add(`{"op":"move","x":1,"y":2}`)
	f.Add(`{"op":"explode"}`)
	f.Fuzz(func(t *testing.T, line string) {
		backend := NewFake()
		out := HandleLine(line, NewSession(), backend)
		if out.Code != CodeSubmitted && len(backend.Snapshot()) != 0 {
			t.Fatalf("non-submit mutated: code=%q calls=%v", out.Code, backend.Snapshot())
		}
	})
}
