package distribuido

import (
	"fmt"
	"math"
	"os"
	"runtime"

	"proyecto9/internal/logistica"
)

// Motor calcula el gradiente parcial de un nodo sobre su partición. Así
// el clúster es heterogéneo: la red y el protocolo son siempre Go, y cada
// nodo calcula con lo que tiene (goroutines en la CPU o CUDA en la GPU).
type Motor interface {
	Nombre() string      // "go" o "cuda"
	Dispositivo() string // texto libre para el log del maestro
	// Cargar recibe la partición una sola vez, antes de la primera época.
	// d va aparte porque la partición puede quedar vacía.
	Cargar(d int, X [][]float64, y []float64, pc logistica.PesosClase) error
	Gradiente(pesos []float64, sesgo float64) (grad []float64, gradSesgo, perdida float64, err error)
	Cerrar() error
}

// MotorGo calcula con goroutines (reducción con casillas privadas,
// Experimento 1b): dentro de cada PC hay memoria compartida y entre PCs
// paso de mensajes, el modelo híbrido de docs/guia_cluster.md.
type MotorGo struct {
	Hilos int
	X     [][]float64
	Y     []float64
	PC    logistica.PesosClase
}

func NuevoMotorGo(hilos int) *MotorGo {
	if hilos < 1 {
		hilos = runtime.NumCPU()
	}
	return &MotorGo{Hilos: hilos}
}

func (m *MotorGo) Nombre() string { return "go" }

func (m *MotorGo) Dispositivo() string {
	nombre, _ := os.Hostname()
	return fmt.Sprintf("CPU %s, %d goroutines", nombre, m.Hilos)
}

func (m *MotorGo) Cargar(_ int, X [][]float64, y []float64, pc logistica.PesosClase) error {
	m.X, m.Y, m.PC = X, y, pc
	return nil
}

func (m *MotorGo) Gradiente(pesos []float64, sesgo float64) ([]float64, float64, float64, error) {
	if len(m.X) == 0 {
		return make([]float64, len(pesos)), 0, 0, nil
	}
	g, gs, p := logistica.GradienteParalelo(pesos, sesgo, m.X, m.Y, m.PC, m.Hilos, logistica.Reduccion)
	return g, gs, p, nil
}

func (m *MotorGo) Cerrar() error { return nil }

// Comparar verifica que dos motores den el mismo gradiente con los mismos
// parámetros, salvo el redondeo por el distinto orden de las sumas
// (tolerancia relativa tol). Es la prueba de corrección de un motor nuevo
// antes de usarlo: un motor rápido pero con otra respuesta no sirve.
func Comparar(a, b Motor, pesos []float64, sesgo, tol float64) error {
	ga, gsa, pa, err := a.Gradiente(pesos, sesgo)
	if err != nil {
		return fmt.Errorf("%s: %w", a.Nombre(), err)
	}
	gb, gsb, pb, err := b.Gradiente(pesos, sesgo)
	if err != nil {
		return fmt.Errorf("%s: %w", b.Nombre(), err)
	}
	if len(ga) != len(gb) {
		return fmt.Errorf("%s devolvió %d componentes y %s %d", a.Nombre(), len(ga), b.Nombre(), len(gb))
	}
	cerca := func(x, y float64) bool { return math.Abs(x-y) <= tol*math.Max(1, math.Abs(y)) }
	for j := range ga {
		if !cerca(ga[j], gb[j]) {
			return fmt.Errorf("componente %d del gradiente: %s=%g, %s=%g", j, a.Nombre(), ga[j], b.Nombre(), gb[j])
		}
	}
	if !cerca(gsa, gsb) || !cerca(pa, pb) {
		return fmt.Errorf("sesgo o pérdida distintos: %s=(%g, %g), %s=(%g, %g)", a.Nombre(), gsa, pa, b.Nombre(), gsb, pb)
	}
	return nil
}
