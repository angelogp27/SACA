package logistica

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
)

// Estrategia es la forma en que las goroutines combinan sus gradientes
// parciales en cada época. Las tres dan el mismo resultado (salvo el orden
// de las sumas en punto flotante); lo que cambia es cuánto se sincronizan.
type Estrategia string

const (
	// Reduccion: cada goroutine acumula en su propia casilla, sin
	// compartir nada; al final la goroutine principal suma las casillas.
	// Es el equivalente de reduction(+:...) de OpenMP o de MPI_Reduce, y el
	// diseño correcto: la sincronización se reduce a una barrera por época.
	Reduccion Estrategia = "reduccion"
	// Mutex: un único acumulador compartido protegido por sync.Mutex, que
	// cada goroutine toma por cada ejemplo. Existe para medir el costo de
	// la contención en una sección crítica de grano fino.
	Mutex Estrategia = "mutex"
	// Atomico: el mismo acumulador compartido, pero actualizado sin locks
	// con comparar-e-intercambiar (CAS) sobre los bits del float64.
	Atomico Estrategia = "atomico"
)

func ParsearEstrategia(s string) (Estrategia, error) {
	switch e := Estrategia(s); e {
	case Reduccion, Mutex, Atomico:
		return e, nil
	}
	return "", fmt.Errorf("estrategia desconocida %q (usa reduccion, mutex o atomico)", s)
}

// bloque devuelve el rango [ini, fin) de la goroutine k de n sobre m
// ejemplos; los bloques difieren en tamaño a lo sumo en 1 (balanceo
// estático de carga).
func bloque(k, n, m int) (int, int) {
	return k * m / n, (k + 1) * m / n
}

// GradienteParalelo calcula lo mismo que GradienteParcial sobre todo X,
// repartiendo los ejemplos entre n goroutines.
func GradienteParalelo(pesos []float64, sesgo float64, X [][]float64, y []float64, pesosClase PesosClase, n int, e Estrategia) ([]float64, float64, float64) {
	if n <= 1 {
		return GradienteParcial(pesos, sesgo, X, y, pesosClase)
	}
	switch e {
	case Mutex:
		return gradienteMutex(pesos, sesgo, X, y, pesosClase, n)
	case Atomico:
		return gradienteAtomico(pesos, sesgo, X, y, pesosClase, n)
	default:
		return gradienteReduccion(pesos, sesgo, X, y, pesosClase, n)
	}
}

func gradienteReduccion(pesos []float64, sesgo float64, X [][]float64, y []float64, pc PesosClase, n int) ([]float64, float64, float64) {
	type parcial struct {
		grad           []float64
		sesgo, perdida float64
	}
	parciales := make([]parcial, n)

	var wg sync.WaitGroup
	for k := 0; k < n; k++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			ini, fin := bloque(k, n, len(X))
			g, s, p := GradienteParcial(pesos, sesgo, X[ini:fin], y[ini:fin], pc)
			parciales[k] = parcial{g, s, p} // cada goroutine escribe solo su casilla
		}(k)
	}
	wg.Wait() // barrera: nadie lee parciales hasta que todas terminaron

	grad := make([]float64, len(pesos))
	var gSesgo, perdida float64
	for _, p := range parciales {
		for j := range grad {
			grad[j] += p.grad[j]
		}
		gSesgo += p.sesgo
		perdida += p.perdida
	}
	return grad, gSesgo, perdida
}

// contribucion calcula el aporte de un ejemplo al gradiente y la pérdida.
func contribucion(pesos []float64, sesgo float64, xi []float64, yi float64, pc PesosClase) (errorPonderado, perdida float64) {
	z := sesgo
	for j, xij := range xi {
		z += pesos[j] * xij
	}
	pred := Sigmoide(z)
	pesoClase := pc.Cero
	if yi == 1 {
		pesoClase = pc.Uno
	}
	return pesoClase * (pred - yi), pesoClase * logLoss(yi, pred)
}

func gradienteMutex(pesos []float64, sesgo float64, X [][]float64, y []float64, pc PesosClase, n int) ([]float64, float64, float64) {
	grad := make([]float64, len(pesos))
	var gSesgo, perdida float64
	var mu sync.Mutex

	var wg sync.WaitGroup
	for k := 0; k < n; k++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			ini, fin := bloque(k, n, len(X))
			for i := ini; i < fin; i++ {
				err, p := contribucion(pesos, sesgo, X[i], y[i], pc)
				mu.Lock() // sección crítica: el acumulador es compartido
				for j, xij := range X[i] {
					grad[j] += err * xij
				}
				gSesgo += err
				perdida += p
				mu.Unlock()
			}
		}(k)
	}
	wg.Wait()
	return grad, gSesgo, perdida
}

// sumarAtomico suma delta a un float64 guardado como bits en un uint64,
// reintentando con CAS si otra goroutine lo modificó en medio.
func sumarAtomico(destino *atomic.Uint64, delta float64) {
	for {
		viejo := destino.Load()
		nuevo := math.Float64bits(math.Float64frombits(viejo) + delta)
		if destino.CompareAndSwap(viejo, nuevo) {
			return
		}
	}
}

func gradienteAtomico(pesos []float64, sesgo float64, X [][]float64, y []float64, pc PesosClase, n int) ([]float64, float64, float64) {
	grad := make([]atomic.Uint64, len(pesos))
	var gSesgo, perdida atomic.Uint64

	var wg sync.WaitGroup
	for k := 0; k < n; k++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			ini, fin := bloque(k, n, len(X))
			for i := ini; i < fin; i++ {
				err, p := contribucion(pesos, sesgo, X[i], y[i], pc)
				for j, xij := range X[i] {
					sumarAtomico(&grad[j], err*xij)
				}
				sumarAtomico(&gSesgo, err)
				sumarAtomico(&perdida, p)
			}
		}(k)
	}
	wg.Wait()

	res := make([]float64, len(pesos))
	for j := range res {
		res[j] = math.Float64frombits(grad[j].Load())
	}
	return res, math.Float64frombits(gSesgo.Load()), math.Float64frombits(perdida.Load())
}

// EntrenarParalelo es Entrenar con el gradiente de cada época calculado
// por n goroutines. La actualización de los parámetros la hace solo la
// goroutine principal, después de la barrera, así que no compite con
// nadie.
func (m *Modelo) EntrenarParalelo(X [][]float64, y []float64, tasaAprendizaje float64, epocas int, pesosClase PesosClase, n int, e Estrategia) []float64 {
	perdidaPorEpoca := make([]float64, 0, epocas)
	total := float64(len(X))

	for ep := 0; ep < epocas; ep++ {
		gradPesos, gradSesgo, perdida := GradienteParalelo(m.Pesos, m.Sesgo, X, y, pesosClase, n, e)
		for j := range m.Pesos {
			m.Pesos[j] -= tasaAprendizaje * gradPesos[j] / total
		}
		m.Sesgo -= tasaAprendizaje * gradSesgo / total
		perdidaPorEpoca = append(perdidaPorEpoca, perdida/total)
	}

	return perdidaPorEpoca
}
