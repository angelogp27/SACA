package hardware

import (
	"syscall"
	"unsafe"
)

// procesador es SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION (clase 8 de
// NtQuerySystemInformation), en unidades de 100 ns. KernelTime incluye
// el tiempo ocioso.
type procesador struct {
	ocioso, kernel, usuario, dpc, interrupcion int64
	interrupciones                             uint32
	_                                          uint32
}

var ntQuery = syscall.NewLazyDLL("ntdll.dll").NewProc("NtQuerySystemInformation")

func ticsNucleos() []tics {
	buf := make([]procesador, 256)
	var largo uint32
	r, _, _ := ntQuery.Call(8, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))*unsafe.Sizeof(buf[0]), uintptr(unsafe.Pointer(&largo)))
	if r != 0 {
		return nil
	}
	n := int(uintptr(largo) / unsafe.Sizeof(buf[0]))
	lista := make([]tics, n)
	for i, p := range buf[:n] {
		total := uint64(p.kernel + p.usuario)
		lista[i] = tics{ocupado: total - uint64(p.ocioso), total: total}
	}
	return lista
}

func ramUso() (usada, total float64) {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	var m memoryStatusEx
	m.longitud = uint32(unsafe.Sizeof(m))
	if r, _, _ := proc.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return 0, 0
	}
	return float64(m.total-m.libre) / (1 << 30), float64(m.total) / (1 << 30)
}
