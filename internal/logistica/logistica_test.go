package logistica

import (
	"math"
	"math/rand"
	"testing"
)

func datosAleatorios(n, d int, semilla int64) ([][]float64, []float64) {
	r := rand.New(rand.NewSource(semilla))
	X := make([][]float64, n)
	y := make([]float64, n)
	for i := range X {
		X[i] = make([]float64, d)
		for j := range X[i] {
			X[i][j] = r.NormFloat64()
		}
		if X[i][0]+0.5*X[i][1] > 0.8 {
			y[i] = 1
		}
	}
	return X, y
}

// TestGradienteParcialEsSumable es la propiedad en la que se apoya toda la
// Parte B: la suma de los gradientes parciales de particiones disjuntas es
// igual al gradiente calculado sobre el conjunto completo.
func TestGradienteParcialEsSumable(t *testing.T) {
	X, y := datosAleatorios(1000, 6, 1)
	pesos := []float64{0.3, -0.2, 0.1, 0, 0.5, -0.4}
	sesgo := 0.05
	pc := CalcularPesosClase(y)

	gTotal, sTotal, pTotal := GradienteParcial(pesos, sesgo, X, y, pc)

	for _, partes := range []int{2, 3, 7, 16} {
		gSuma := make([]float64, len(pesos))
		var sSuma, pSuma float64
		for k := 0; k < partes; k++ {
			ini, fin := k*len(X)/partes, (k+1)*len(X)/partes
			g, s, p := GradienteParcial(pesos, sesgo, X[ini:fin], y[ini:fin], pc)
			for j := range g {
				gSuma[j] += g[j]
			}
			sSuma += s
			pSuma += p
		}
		for j := range gTotal {
			if math.Abs(gTotal[j]-gSuma[j]) > 1e-9 {
				t.Fatalf("%d partes: gradiente[%d] = %v, se esperaba %v", partes, j, gSuma[j], gTotal[j])
			}
		}
		if math.Abs(sTotal-sSuma) > 1e-9 || math.Abs(pTotal-pSuma) > 1e-9 {
			t.Fatalf("%d partes: sesgo/perdida no coinciden", partes)
		}
	}
}

func TestEntrenarReducePerdida(t *testing.T) {
	X, y := datosAleatorios(2000, 4, 2)
	m := NuevoModelo(4)
	perdidas := m.Entrenar(X, y, 0.5, 100, CalcularPesosClase(y))
	if perdidas[len(perdidas)-1] >= perdidas[0] {
		t.Fatalf("la pérdida no bajó: %v -> %v", perdidas[0], perdidas[len(perdidas)-1])
	}
}

func TestCalcularPesosClase(t *testing.T) {
	pc := CalcularPesosClase([]float64{1, 0, 0, 0})
	if pc.Uno != 2 || math.Abs(pc.Cero-4.0/6) > 1e-12 {
		t.Fatalf("pesos = %+v", pc)
	}
}
