package limpieza

import "time"

// MedidorUso mide la CPU que gastan varios procesos entre dos lecturas, en
// % de todos los núcleos lógicos (el entrenamiento: el trabajador y sus
// motores externos).
type MedidorUso struct {
	medidores []*medidorCPU
	nucleos   float64
	antes     []float64
	t         time.Time
}

func NuevoMedidorUso(nucleos int, pids ...int) *MedidorUso {
	m := &MedidorUso{nucleos: float64(max(nucleos, 1)), antes: make([]float64, len(pids))}
	for _, p := range pids {
		m.medidores = append(m.medidores, nuevoMedidorCPU(p))
	}
	m.Leer()
	return m
}

// Leer devuelve el uso desde la lectura anterior.
func (m *MedidorUso) Leer() float64 {
	ahora, gasto := time.Now(), 0.0
	for i, md := range m.medidores {
		s, err := md.segundos()
		if err != nil {
			continue
		}
		gasto += max(s-m.antes[i], 0)
		m.antes[i] = s
	}
	dt := ahora.Sub(m.t).Seconds()
	primera := m.t.IsZero()
	m.t = ahora
	if primera || dt <= 0 {
		return 0
	}
	return min(100, 100*gasto/(dt*m.nucleos))
}

func (m *MedidorUso) Cerrar() {
	for _, md := range m.medidores {
		md.cerrar()
	}
}
