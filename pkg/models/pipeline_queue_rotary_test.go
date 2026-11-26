package models

import "testing"

func TestPipelineQueueRotary_Summary(t *testing.T) {
	rotary := NewPipelineQueueRotary(5)
	for i := 1; i <= 8; i++ {
		rotary.Record("linux-x64", int64(i*50), i)
	}

	summary := rotary.GetSummary()
	if summary.SampleCount != 5 {
		t.Fatalf("expected 5 samples, got %d", summary.SampleCount)
	}
	if summary.MaxQueueWaitMs != 400 {
		t.Fatalf("expected max wait 400ms, got %d", summary.MaxQueueWaitMs)
	}
	if summary.P50WaitMs <= 0 {
		t.Fatalf("expected positive P50 wait, got %d", summary.P50WaitMs)
	}
}
