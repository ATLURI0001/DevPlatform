package security

// DeployDurationRotary tracks bounded deployment duration samples in seconds.
type DeployDurationRotary struct {
	samples []float64
	cap     int
}

func NewDeployDurationRotary(capacity int) *DeployDurationRotary {
	return &DeployDurationRotary{cap: capacity}
}

func (r *DeployDurationRotary) Record(sec float64) {
	if len(r.samples) >= r.cap {
		r.samples = r.samples[1:]
	}
	r.samples = append(r.samples, sec)
}

func (r *DeployDurationRotary) Avg() float64 {
	if len(r.samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range r.samples {
		sum += s
	}
	return sum / float64(len(r.samples))
}
