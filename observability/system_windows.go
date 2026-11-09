package observability

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

var kernel = windows.NewLazySystemDLL("kernel32.dll")
var getTimes = kernel.NewProc("GetSystemTimes")
var getMemory = kernel.NewProc("GlobalMemoryStatusEx")
var getDisk = kernel.NewProc("GetDiskFreeSpaceExW")

type memoryStatus struct {
	Length            uint32
	Load              uint32
	TotalPhys         uint64
	AvailablePhys     uint64
	TotalPage         uint64
	AvailablePage     uint64
	TotalVirtual      uint64
	AvailableVirtual  uint64
	AvailableExtended uint64
}

func systemCounters() (uint64, uint64, *float64, *float64) {
	var idle, kernelTime, user windows.Filetime
	var total, idleCount uint64
	if ok, _, _ := getTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernelTime)), uintptr(unsafe.Pointer(&user))); ok != 0 {
		asInt := func(t windows.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }
		total, idleCount = asInt(kernelTime)+asInt(user), asInt(idle)
	}
	var mem, disk *float64
	memory := memoryStatus{}
	memory.Length = uint32(unsafe.Sizeof(memory))
	if ok, _, _ := getMemory.Call(uintptr(unsafe.Pointer(&memory))); ok != 0 && memory.TotalPhys > 0 {
		value := 100 * (1 - float64(memory.AvailablePhys)/float64(memory.TotalPhys))
		mem = &value
	}
	path, _ := windows.UTF16PtrFromString(".")
	var available, capacity, free uint64
	if ok, _, _ := getDisk.Call(uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&available)), uintptr(unsafe.Pointer(&capacity)), uintptr(unsafe.Pointer(&free))); ok != 0 && capacity > 0 {
		value := 100 * (1 - float64(free)/float64(capacity))
		disk = &value
	}
	return total, idleCount, mem, disk
}
