package convert

import (
	"testing"
	"time"
)

func TestThroughputRateAndETA(t *testing.T) {
	tp := newThroughput(15)
	base := time.Now()
	// Inject controlled samples: 1000 bytes over 2 seconds -> 500 B/s.
	tp.samples = []sample{
		{at: base, totalBytes: 0},
		{at: base.Add(2 * time.Second), totalBytes: 1000},
	}
	if r := tp.rate(); r < 499 || r > 501 {
		t.Errorf("rate = %v, want ~500", r)
	}
	// 1000 remaining bytes at 500 B/s -> ~2s.
	eta := tp.eta(1000)
	if eta < 1900*time.Millisecond || eta > 2100*time.Millisecond {
		t.Errorf("eta = %v, want ~2s", eta)
	}
}

func TestThroughputInsufficientData(t *testing.T) {
	tp := newThroughput(15)
	if tp.rate() != 0 {
		t.Error("rate with no samples should be 0")
	}
	if tp.eta(1000) != 0 {
		t.Error("eta with unknown rate should be 0")
	}
	tp.record(100)
	if tp.rate() != 0 {
		t.Error("rate with one sample should be 0")
	}
}

func TestThroughputWindowTrims(t *testing.T) {
	tp := newThroughput(3)
	for i := 0; i < 10; i++ {
		tp.record(int64(i * 100))
	}
	if len(tp.samples) != 3 {
		t.Errorf("window not trimmed: %d samples", len(tp.samples))
	}
}
