// Package vnext is the private Unix JSON validate-then-dispatch path.
// Unknown, incomplete, and invalid requests never reach a backend.
package vnext

import "encoding/json"

// Outcome is a destination result. Code is a stable token.
type Outcome struct {
	Code            string
	Err             error
	Windows         []WindowInfo
	ScreenWidth     int
	ScreenHeight    int
	RequestedActive uint32
	ObservedActive  uint32
}

const (
	// CodeSubmitted means the request was accepted and dispatched.
	CodeSubmitted = "Submitted"
	// CodeNotSubmitted means the operation is not supported.
	CodeNotSubmitted = "NotSubmitted"
	// CodeInvalidRequest means validation failed before dispatch.
	CodeInvalidRequest = "InvalidRequest"
	// CodeConflict means a durable window reference no longer matches.
	CodeConflict = "Conflict"
	// CodeUnavailable means the backend could not complete the request.
	CodeUnavailable = "Unavailable"
)

// Submitted reports whether the outcome represents successful dispatch.
func (o Outcome) Submitted() bool { return o.Code == CodeSubmitted && o.Err == nil }

func submitted() Outcome { return Outcome{Code: CodeSubmitted} }

func notSubmitted() Outcome { return Outcome{Code: CodeNotSubmitted} }

func invalidRequest() Outcome { return Outcome{Code: CodeInvalidRequest} }

func conflict() Outcome { return Outcome{Code: CodeConflict} }

// WireJSON is the dest unix reply. It never includes Err or payloads for logs.
func (o Outcome) WireJSON() ([]byte, error) {
	w := struct {
		Code            string       `json:"code"`
		Windows         []WindowInfo `json:"windows,omitempty"`
		ScreenWidth     int          `json:"screenWidth,omitempty"`
		ScreenHeight    int          `json:"screenHeight,omitempty"`
		RequestedActive *uint32      `json:"requestedActive,omitempty"`
		ObservedActive  *uint32      `json:"observedActive,omitempty"`
	}{
		Code:         o.Code,
		Windows:      o.Windows,
		ScreenWidth:  o.ScreenWidth,
		ScreenHeight: o.ScreenHeight,
	}
	if o.RequestedActive != 0 {
		r := o.RequestedActive
		w.RequestedActive = &r
	}
	if o.ObservedActive != 0 {
		obs := o.ObservedActive
		w.ObservedActive = &obs
	}
	return json.Marshal(w)
}
