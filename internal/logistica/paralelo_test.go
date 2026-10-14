package logistica

import (
	"math"
	"testing"
)

// Las tres estrategias, con cualquier número de goroutines, deben dar el
// mismo gradiente que el cálculo secuencial (salvo redondeo por el orden
// de las sumas). Correr con -race comprueba además que no hay carreras.
func TestGradienteParaleloIgualAlSecuencial(t *testing.T) {
	X, y := datosAleatorios(3001, 5, 3)
	pesos := []float64{0.2, -0.1, 0.4, 0.05, -0.3}
	pc := CalcularPesosClase(y)
	gEsperado, sEsperado, pEsperado := GradienteParcial(pesos, 0.1, X, y, pc)

	for _, e := range []Estrategia{Reduccion, Mutex, Atomico} {
		for _, n := range []int{1, 2, 3, 8} {
			g, s, p := GradienteParalelo(pesos, 0.1, X, y, pc, n, e)
			for j := range g {
				if math.Abs(g[j]-gEsperado[j]) > 1e-8 {
					t.Fatalf("%s n=%d: gradiente[%d]=%v, se esperaba %v", e, n, j, g[j], gEsperado[j])
				}
			}
			if math.Abs(s-sEsperado) > 1e-8 || math.Abs(p-pEsperado) > 1e-8 {
				t.Fatalf("%s n=%d: sesgo/perdida distintos", e, n)
			}
		}
	}
}

func TestEntrenarParaleloConUnaGoroutineEsIdentico(t *testing.T) {
	X, y := datosAleatorios(500, 4, 4)
	pc := CalcularPesosClase(y)
	a, b := NuevoModelo(4), NuevoModelo(4)
	a.Entrenar(X, y, 0.3, 50, pc)
	b.EntrenarParalelo(X, y, 0.3, 50, pc, 1, Reduccion)
	for j := range a.Pesos {
		if a.Pesos[j] != b.Pesos[j] {
			t.Fatalf("peso %d: %v != %v", j, a.Pesos[j], b.Pesos[j])
		}
	}
}

func TestBloquesCubrenTodo(t *testing.T) {
	for _, m := range []int{0, 1, 7, 100} {
		for n := 1; n <= 9; n++ {
			siguiente := 0
			for k := 0; k < n; k++ {
				ini, fin := bloque(k, n, m)
				if ini != siguiente || fin < ini {
					t.Fatalf("m=%d n=%d k=%d: [%d,%d)", m, n, k, ini, fin)
				}
				siguiente = fin
			}
			if siguiente != m {
				t.Fatalf("m=%d n=%d: termina en %d", m, n, siguiente)
			}
		}
	}
}
