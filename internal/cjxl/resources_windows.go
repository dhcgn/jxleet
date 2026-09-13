//go:build windows

package cjxl

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modPsapi                 = windows.NewLazySystemDLL("psapi.dll")
	procGetProcessMemoryInfo = modPsapi.NewProc("GetProcessMemoryInfo")
)

// processMemoryCounters mirrors PROCESS_MEMORY_COUNTERS_EX; only
// WorkingSetSize is read, but psapi needs the full struct size in cb.
type processMemoryCounters struct {
	cb                       uint32
	pageFaultCount           uint32
	peakWorkingSetSize       uintptr
	workingSetSize           uintptr
	quotaPeakPagedPoolUsage  uintptr
	quotaPagedPoolUsage      uintptr
	quotaPeakNonPagedPoolUse uintptr
	quotaNonPagedPoolUsage   uintptr
	pagefileUsage            uintptr
	peakPagefileUsage        uintptr
	privateUsage             uintptr
}

// cpuSamples holds the previous CPU reading per process, keyed by PID plus
// creation time so a reused PID starts a fresh baseline instead of diffing
// against a dead process.
var cpuSamples = struct {
	sync.Mutex
	prev map[cpuSampleKey]cpuSample
}{prev: make(map[cpuSampleKey]cpuSample)}

type cpuSampleKey struct {
	pid     int
	created uint64
}

type cpuSample struct {
	at         time.Time
	cpuSeconds float64
}

// forgetSamples drops baselines for pid, bounding the map to live processes.
func forgetSamples(pid int) {
	cpuSamples.Lock()
	defer cpuSamples.Unlock()
	for key := range cpuSamples.prev {
		if key.pid == pid {
			delete(cpuSamples.prev, key)
		}
	}
}

// GetProcessResources snapshots CPU usage (percent of total machine capacity,
// 0-100) and working-set RAM for pid. Usage is windowed between consecutive
// calls; the first sighting reports the lifetime average since process start.
// It returns an error when the process does not exist or cannot be opened,
// in which case callers keep the row placeholder.
func GetProcessResources(pid int) (ProcessResources, error) {
	if pid <= 0 {
		return ProcessResources{}, fmt.Errorf("cjxl: invalid pid %d", pid)
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		forgetSamples(pid)
		return ProcessResources{}, fmt.Errorf("cjxl: open process %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		forgetSamples(pid)
		return ProcessResources{}, fmt.Errorf("cjxl: process times %d: %w", pid, err)
	}
	filetimeToSeconds := func(ft windows.Filetime) float64 {
		return float64(uint64(ft.HighDateTime)<<32|uint64(ft.LowDateTime)) / 1e7
	}
	cpuSeconds := filetimeToSeconds(kernel) + filetimeToSeconds(user)
	now := time.Now()
	cores := float64(runtime.NumCPU())

	cpuSamples.Lock()
	key := cpuSampleKey{pid: pid, created: uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)}
	percent := 0.0
	if prev, ok := cpuSamples.prev[key]; ok && now.After(prev.at) {
		percent = 100 * (cpuSeconds - prev.cpuSeconds) / now.Sub(prev.at).Seconds() / cores
	} else if age := now.Sub(time.Unix(0, creation.Nanoseconds())); age > 0 {
		percent = 100 * cpuSeconds / age.Seconds() / cores
	}
	cpuSamples.prev[key] = cpuSample{at: now, cpuSeconds: cpuSeconds}
	cpuSamples.Unlock()
	percent = min(max(percent, 0), 100)

	var counters processMemoryCounters
	counters.cb = uint32(unsafe.Sizeof(counters))
	ret, _, callErr := procGetProcessMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.cb),
	)
	if ret == 0 {
		return ProcessResources{}, fmt.Errorf("cjxl: process memory %d: %v", pid, callErr)
	}
	return ProcessResources{
		PID:         pid,
		CPUPercent:  percent,
		MemoryBytes: uint64(counters.workingSetSize),
	}, nil
}
