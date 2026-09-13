package cjxl

import (
	"os"
	"runtime"
	"testing"
)

// TestGetProcessResourcesCurrentProcess snapshots the test process itself:
// it must exist and report a non-zero PID back. Windows-only; elsewhere the
// sampler reports unavailable.
func TestGetProcessResourcesCurrentProcess(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("process resources are Windows-only")
	}
	res, err := GetProcessResources(os.Getpid())
	if err != nil {
		t.Fatalf("GetProcessResources(self): %v", err)
	}
	if res.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", res.PID, os.Getpid())
	}
	if res.MemoryBytes == 0 {
		t.Error("memory = 0, want the working set of a running test process")
	}
	if res.CPUPercent < 0 || res.CPUPercent > 100 {
		t.Errorf("cpu = %v, want a 0-100 usage percent", res.CPUPercent)
	}
	// A second call windows against the first and must stay in range too.
	res2, err := GetProcessResources(os.Getpid())
	if err != nil {
		t.Fatalf("GetProcessResources(self) again: %v", err)
	}
	if res2.CPUPercent < 0 || res2.CPUPercent > 100 {
		t.Errorf("cpu = %v, want a 0-100 usage percent", res2.CPUPercent)
	}
}

// TestGetProcessResourcesInvalidPid keeps the queue placeholder contract: an
// exited or never-existing PID must error instead of returning zeros.
func TestGetProcessResourcesInvalidPid(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if _, err := GetProcessResources(pid); err == nil {
			t.Errorf("GetProcessResources(%d): want error, got nil", pid)
		}
	}
}
