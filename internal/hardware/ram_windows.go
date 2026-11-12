package hardware

import (
	"syscall"
	"unsafe"
)

// memoryStatusEx es MEMORYSTATUSEX de la API de Windows.
type memoryStatusEx struct {
	longitud, carga                       uint32
	total, libre, totalPag, librePag      uint64
	totalVirtual, libreVirtual, extendida uint64
}

// ramWindows devuelve la RAM total en GB (GlobalMemoryStatusEx), o 0.
func ramWindows() float64 {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	var m memoryStatusEx
	m.longitud = uint32(unsafe.Sizeof(m))
	if r, _, _ := proc.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return 0
	}
	return float64(m.total) / (1 << 30)
}
