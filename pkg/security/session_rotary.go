package security

// SessionRotary stores bounded session IDs in a ring buffer.
type SessionRotary struct {
	sessions []string
	cap      int
}

func NewSessionRotary(cap int) *SessionRotary {
	if cap <= 0 {
		cap = 1
	}
	return &SessionRotary{cap: cap}
}

func (r *SessionRotary) Record(sessionID string) {
	if len(r.sessions) >= r.cap {
		r.sessions = r.sessions[1:]
	}
	r.sessions = append(r.sessions, sessionID)
}

func (r *SessionRotary) Len() int { return len(r.sessions) }
