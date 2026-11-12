package limpieza

import "syscall"

// En Windows se abre el proceso hijo y se le piden sus tiempos de kernel y
// de usuario (GetProcessTimes, en unidades de 100 ns).
type medidorCPU struct {
	h   syscall.Handle
	err error
}

const processQueryLimitedInformation = 0x1000

func nuevoMedidorCPU(pid int) *medidorCPU {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	return &medidorCPU{h: h, err: err}
}

func (m *medidorCPU) segundos() (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	var creacion, salida, kernel, usuario syscall.Filetime
	if err := syscall.GetProcessTimes(m.h, &creacion, &salida, &kernel, &usuario); err != nil {
		return 0, err
	}
	cien := func(f syscall.Filetime) float64 { return float64(uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime)) }
	return (cien(kernel) + cien(usuario)) / 1e7, nil
}

func (m *medidorCPU) cerrar() {
	if m.err == nil {
		syscall.CloseHandle(m.h)
	}
}
