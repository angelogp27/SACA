package limpieza

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// En Linux el tiempo de CPU de un proceso está en /proc/<pid>/stat
// (campos 14 utime y 15 stime, en ticks de USER_HZ = 100 por segundo).
type medidorCPU struct{ ruta string }

func nuevoMedidorCPU(pid int) *medidorCPU {
	return &medidorCPU{ruta: fmt.Sprintf("/proc/%d/stat", pid)}
}

func (m *medidorCPU) segundos() (float64, error) {
	datos, err := os.ReadFile(m.ruta)
	if err != nil {
		return 0, err
	}
	// El nombre del proceso va entre paréntesis y puede tener espacios: se
	// cuenta desde el último ')'.
	s := string(datos)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0, ErrCPUNoDisponible
	}
	campos := strings.Fields(s[i+1:])
	if len(campos) < 13 {
		return 0, ErrCPUNoDisponible
	}
	// campos[0] es el estado (campo 3), así que utime (14) es campos[11].
	utime, err1 := strconv.ParseFloat(campos[11], 64)
	stime, err2 := strconv.ParseFloat(campos[12], 64)
	if err1 != nil || err2 != nil {
		return 0, ErrCPUNoDisponible
	}
	return (utime + stime) / 100, nil
}

func (m *medidorCPU) cerrar() {}
