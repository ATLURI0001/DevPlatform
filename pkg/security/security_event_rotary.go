package security

// SecurityEventRotary is a bounded ring buffer for security audit events.
type SecurityEventRotary struct {
	buf  []string
	size int
	head int
	cnt  int
}

func NewSecurityEventRotary(size int) *SecurityEventRotary {
	return &SecurityEventRotary{buf: make([]string, size), size: size}
}

func (r *SecurityEventRotary) Push(ev string) {
	r.buf[r.head%r.size] = ev
	r.head++
	if r.cnt < r.size {
		r.cnt++
	}
}

func (r *SecurityEventRotary) Count() int {
	return r.cnt
}
