package vnext

import "sync"

// Session tracks keys and buttons held by one destination client.
// Release releases only this session. It never calls a global releaseall.
type Session struct {
	mu      sync.Mutex
	keys    map[uint]bool
	buttons map[uint]bool
}

// NewSession creates empty per-connection hold state.
func NewSession() *Session {
	return &Session{
		keys:    make(map[uint]bool),
		buttons: make(map[uint]bool),
	}
}

func (s *Session) noteKey(kc uint, press bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if press {
		s.keys[kc] = true
		return
	}
	delete(s.keys, kc)
}

func (s *Session) noteButton(b uint, press bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if press {
		s.buttons[b] = true
		return
	}
	delete(s.buttons, b)
}

// Release sends key/button ups for this session only. It does not retry.
func (s *Session) Release(b Backend) Outcome {
	if s == nil {
		return invalidRequest()
	}
	if b == nil {
		return Outcome{Code: CodeUnavailable}
	}
	s.mu.Lock()
	keys := make([]uint, 0, len(s.keys))
	for kc, down := range s.keys {
		if down {
			keys = append(keys, kc)
		}
	}
	btns := make([]uint, 0, len(s.buttons))
	for btn, down := range s.buttons {
		if down {
			btns = append(btns, btn)
		}
	}
	s.mu.Unlock()
	var first error
	for _, kc := range keys {
		if err := b.SendKey(kc, false); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		s.mu.Lock()
		delete(s.keys, kc)
		s.mu.Unlock()
	}
	for _, btn := range btns {
		if err := b.SendButton(btn, false); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		s.mu.Lock()
		delete(s.buttons, btn)
		s.mu.Unlock()
	}
	if first != nil {
		return mapBackendError(first)
	}
	return submitted()
}

// KeyDown reports whether this session owns a key hold.
func (s *Session) KeyDown(kc uint) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keys[kc]
}

// ButtonDown reports whether this session owns a button hold.
func (s *Session) ButtonDown(b uint) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buttons[b]
}
