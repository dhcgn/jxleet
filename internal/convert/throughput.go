package convert

import (
	"sync"
	"time"
)

// throughput estimates the recent processing rate from a sliding window of
// cumulative-bytes samples, so the ETA adapts when file sizes or effort change
// mid-run (see README "Progress ... from measured throughput").
type throughput struct {
	mu      sync.Mutex
	samples []sample
	window  int
}

type sample struct {
	at         time.Time
	totalBytes int64
}

func newThroughput(window int) *throughput {
	if window < 2 {
		window = 2
	}
	return &throughput{window: window}
}

// record adds a cumulative-bytes-done sample at the current time.
func (t *throughput) record(totalBytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.samples = append(t.samples, sample{at: time.Now(), totalBytes: totalBytes})
	if len(t.samples) > t.window {
		t.samples = t.samples[len(t.samples)-t.window:]
	}
}

// rate returns the recent throughput in bytes per second, or 0 when there is not
// enough data yet.
func (t *throughput) rate() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.samples) < 2 {
		return 0
	}
	first := t.samples[0]
	last := t.samples[len(t.samples)-1]
	elapsed := last.at.Sub(first.at).Seconds()
	if elapsed <= 0 {
		return 0
	}
	delta := last.totalBytes - first.totalBytes
	if delta <= 0 {
		return 0
	}
	return float64(delta) / elapsed
}

// eta estimates the time to process remainingBytes at the recent rate. It
// returns 0 when the rate is unknown.
func (t *throughput) eta(remainingBytes int64) time.Duration {
	r := t.rate()
	if r <= 0 || remainingBytes <= 0 {
		return 0
	}
	return time.Duration(float64(remainingBytes) / r * float64(time.Second))
}
