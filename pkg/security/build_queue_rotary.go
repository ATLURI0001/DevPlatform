package security

// BuildQueueRotary tracks bounded build queue depth samples.
type BuildQueueRotary struct {
	samples []int
	cap     int
}

func NewBuildQueueRotary(capacity int) *BuildQueueRotary {
	return &BuildQueueRotary{cap: capacity}
}

func (r *BuildQueueRotary) Record(depth int) {
	if len(r.samples) >= r.cap {
		r.samples = r.samples[1:]
	}
	r.samples = append(r.samples, depth)
}

func (r *BuildQueueRotary) Avg() float64 {
	if len(r.samples) == 0 {
		return 0
	}
	sum := 0
	for _, s := range r.samples {
		sum += s
	}
	return float64(sum) / float64(len(r.samples))
}
