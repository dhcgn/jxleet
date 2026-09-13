//go:build windows

package cjxl

import (
	"fmt"
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

// GetProcessResources snapshots CPU time and working-set RAM for pid.
// It returns an error when the process does not exist or cannot be opened,
// in which case callers keep the row placeholder.
func GetProcessResources(pid int) (ProcessResources, error) {
	if pid <= 0 {
		return ProcessResources{}, fmt.Errorf("cjxl: invalid pid %d", pid)
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ProcessResources{}, fmt.Errorf("cjxl: open process %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return ProcessResources{}, fmt.Errorf("cjxl: process times %d: %w", pid, err)
	}
	// ponytail: cumulative CPU seconds, not a % — a % needs two samples and the
	// frontend already samples on its fixed cadence; diff there if % is wanted.
	filetimeToSeconds := func(ft windows.Filetime) float64 {
		return float64(uint64(ft.HighDateTime)<<32|uint64(ft.LowDateTime)) / 1e7
	}
	cpuSeconds := filetimeToSeconds(kernel) + filetimeToSeconds(user)

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
		PID:            pid,
		CPUTimeSeconds: cpuSeconds,
		MemoryBytes:    uint64(counters.workingSetSize),
	}, nil
}
