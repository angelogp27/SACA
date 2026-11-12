//go:build !linux && !windows

package limpieza

// En otros sistemas (macOS) no se mide la CPU: la gráfica muestra 0 %.
type medidorCPU struct{}

func nuevoMedidorCPU(int) *medidorCPU            { return &medidorCPU{} }
func (m *medidorCPU) segundos() (float64, error) { return 0, ErrCPUNoDisponible }
func (m *medidorCPU) cerrar()                    {}
