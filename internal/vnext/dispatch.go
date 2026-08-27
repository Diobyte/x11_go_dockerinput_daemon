package vnext

import "errors"

// HandleLine is the shipped destination entry point: decode+validate the
// complete request, then dispatch. Invalid/unknown never call the backend.
// A failed backend call is returned; this function does not retry.
func HandleLine(line string, sess *Session, b Backend) Outcome {
	req, out := Decode(line)
	if out.Code != CodeSubmitted {
		return out
	}
	return Dispatch(req, sess, b)
}

// Dispatch runs a validated request. Incomplete requests must not reach here.
func Dispatch(req Request, sess *Session, b Backend) Outcome {
	if b == nil {
		return Outcome{Code: CodeUnavailable}
	}
	switch req.Op {
	case OpMove:
		if err := b.WarpMouse(req.X, req.Y); err != nil {
			return mapBackendError(err)
		}
		return submitted()
	case OpKey:
		kc := b.KeycodeFor(req.Key)
		if kc == 0 {
			return invalidRequest()
		}
		if err := b.SendKey(kc, req.Press); err != nil {
			return mapBackendError(err)
		}
		if sess != nil {
			sess.noteKey(kc, req.Press)
		}
		return submitted()
	case OpButton:
		if err := b.SendButton(req.Button, req.Press); err != nil {
			return mapBackendError(err)
		}
		if sess != nil {
			sess.noteButton(req.Button, req.Press)
		}
		return submitted()
	case OpInspect:
		width, height, err := b.ScreenGeometry()
		if err != nil {
			return mapBackendError(err)
		}
		list, err := b.ManagedClients()
		if err != nil {
			return mapBackendError(err)
		}
		return Outcome{Code: CodeSubmitted, Windows: list, ScreenWidth: width, ScreenHeight: height}
	case OpActivate:
		if err := b.Revalidate(req.Window); err != nil {
			return mapBackendError(err)
		}
		if err := b.ActivateAlwaysBoth(req.Window); err != nil {
			return mapBackendError(err)
		}
		obs, err := b.ActiveWindow()
		if err != nil {
			return mapBackendError(err)
		}
		return Outcome{Code: CodeSubmitted, RequestedActive: req.Window.XID, ObservedActive: obs}
	case OpFullscreen:
		if err := b.Revalidate(req.Window); err != nil {
			return mapBackendError(err)
		}
		if err := b.SetFullscreen(req.Window, req.AddState); err != nil {
			return mapBackendError(err)
		}
		return submitted()
	case OpRelease:
		if sess == nil {
			return invalidRequest()
		}
		return sess.Release(b)
	default:
		return notSubmitted()
	}
}

func mapBackendError(err error) Outcome {
	if err == nil {
		return Outcome{Code: CodeUnavailable}
	}
	if errors.Is(err, ErrRefMismatch) {
		return conflict()
	}
	return Outcome{Code: CodeUnavailable, Err: err}
}
