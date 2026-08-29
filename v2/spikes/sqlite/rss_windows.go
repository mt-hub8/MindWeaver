//go:build windows

package sqliteprobe

import (
	"fmt"
	"syscall"
	"unsafe"
)

type rssSample struct {
	supported   bool
	measurement string
	current     uint64
	peak        uint64
	err         string
}

type processMemoryCountersEx struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

var (
	procGetCurrentProcess    = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentProcess")
	procGetProcessMemoryInfo = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
)

func readRSS() rssSample {
	var counters processMemoryCountersEx
	counters.CB = uint32(unsafe.Sizeof(counters))
	process, _, _ := procGetCurrentProcess.Call()
	ok, _, callErr := procGetProcessMemoryInfo.Call(
		process,
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.CB),
	)
	if ok == 0 {
		return rssSample{measurement: "Windows GetProcessMemoryInfo working set", err: fmt.Sprintf("GetProcessMemoryInfo: %v", callErr)}
	}
	return rssSample{
		supported:   true,
		measurement: "Windows GetProcessMemoryInfo working set",
		current:     uint64(counters.WorkingSetSize),
		peak:        uint64(counters.PeakWorkingSetSize),
	}
}
