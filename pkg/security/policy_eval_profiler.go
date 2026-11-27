package security

import "time"

// PolicyEvalProfiler records security policy evaluation durations.
type PolicyEvalProfiler struct {
	samples []time.Duration
}

func NewPolicyEvalProfiler() *PolicyEvalProfiler {
	return &PolicyEvalProfiler{}
}

func (p *PolicyEvalProfiler) Record(d time.Duration) {
	p.samples = append(p.samples, d)
}

func (p *PolicyEvalProfiler) Count() int {
	return len(p.samples)
}
