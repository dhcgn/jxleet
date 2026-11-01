package app

import (
	"runtime"
	"testing"

	"github.com/dhcgn/jxleet/internal/convert"
)

func TestAutoProcessesPassthrough(t *testing.T) {
	for _, n := range []int{1, 2, 8, 64} {
		if got := AutoProcesses(n); got != n {
			t.Errorf("AutoProcesses(%d) = %d, want passthrough", n, got)
		}
	}
}

func TestAutoProcessesAutomatic(t *testing.T) {
	want := runtime.NumCPU() - 1
	if want < 1 {
		want = 1
	}
	if want > MaxAutoProcesses {
		want = MaxAutoProcesses
	}
	if got := AutoProcesses(0); got != want {
		t.Errorf("AutoProcesses(0) = %d, want %d", got, want)
	}
}

func TestGetCPUCount(t *testing.T) {
	if got := testService(t).GetCPUCount(); got != runtime.NumCPU() {
		t.Errorf("GetCPUCount() = %d, want %d", got, runtime.NumCPU())
	}
}

func TestStartQueueRunRejectsEmpty(t *testing.T) {
	if err := testService(t).StartQueueRun(nil); err == nil {
		t.Error("StartQueueRun(nil): want error, got nil")
	}
}

func TestStartQueueRunRejectsBusyEngine(t *testing.T) {
	service := testService(t)
	service.engine = convert.New(convert.Deps{}, convert.Settings{})
	if err := service.StartQueueRun([]QueueItemInput{{Path: "a.png"}}); err == nil {
		t.Error("StartQueueRun with live engine: want error, got nil")
	}
}
