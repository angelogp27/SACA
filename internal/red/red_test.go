package red

import (
	"math"
	"math/rand/v2"
	"testing"

	"proyecto9/internal/logistica"
)

func datos(n, d int) ([][]float64, []float64) {
	r := rand.New(rand.NewPCG(1, 2))
	X := make([][]float64, n)
	y := make([]float64, n)
	for i := range X {
		X[i] = make([]float64, d)
		for j := range X[i] {
			X[i][j] = r.NormFloat64()
		}
		// Frontera no lineal: una regresión logística no la separa.
		if X[i][0]*X[i][1] > 0.2 {
			y[i] = 1
		}
	}
	return X, y
}

func TestGradienteNumerico(t *testing.T) {
	capas := []int{4, 5, 3, 1}
	X, y := datos(30, 4)
	pc := logistica.PesosClase{Cero: 0.7, Uno: 1.9}
	p := Inicializar(capas, 3)
	// Sesgos en 0 dejan algunas neuronas justo en el quiebre de ReLU (z = 0),
	// donde la derivada numérica no vale: se mueven un poco.
	r := rand.New(rand.NewPCG(9, 9))
	for j := range p {
		p[j] += 0.05 * r.NormFloat64()
	}
	g, _ := Gradiente(capas, p, X, y, pc, 3)
	perdida := func(q []float64) float64 { _, l := Gradiente(capas, q, X, y, pc, 1); return l }
	const h = 1e-6
	for j := range p {
		q := append([]float64(nil), p...)
		q[j] += h
		mas := perdida(q)
		q[j] -= 2 * h
		menos := perdida(q)
		num := (mas - menos) / (2 * h)
		if math.Abs(num-g[j]) > 1e-4*math.Max(1, math.Abs(num)) {
			t.Fatalf("parámetro %d: analítico %g, numérico %g", j, g[j], num)
		}
	}
}

func TestHilosNoCambianElResultado(t *testing.T) {
	capas := Arquitectura(6, []int{8, 4})
	X, y := datos(500, 6)
	p := Inicializar(capas, 1)
	g1, l1 := Gradiente(capas, p, X, y, logistica.PesosClaseNeutros, 1)
	g8, l8 := Gradiente(capas, p, X, y, logistica.PesosClaseNeutros, 8)
	for j := range g1 {
		if math.Abs(g1[j]-g8[j]) > 1e-9*math.Max(1, math.Abs(g1[j])) {
			t.Fatalf("componente %d: %g vs %g", j, g1[j], g8[j])
		}
	}
	if math.Abs(l1-l8) > 1e-9*l1 {
		t.Fatal("pérdida distinta")
	}
}

func TestAprende(t *testing.T) {
	capas := Arquitectura(4, []int{16, 8})
	X, y := datos(2000, 4)
	p := Inicializar(capas, 7)
	adam := NuevoAdam(len(p), 0.01)
	_, inicial := Gradiente(capas, p, X, y, logistica.PesosClaseNeutros, 4)
	for epoca := 0; epoca < 30; epoca++ {
		for s := 0; s < 10; s++ {
			ini, fin := Lote(len(X), s, 10)
			g, _ := Gradiente(capas, p, X[ini:fin], y[ini:fin], logistica.PesosClaseNeutros, 4)
			adam.Paso(p, g, fin-ini)
		}
	}
	_, final := Gradiente(capas, p, X, y, logistica.PesosClaseNeutros, 4)
	if final > inicial*0.5 {
		t.Fatalf("no aprendió: pérdida %g → %g", inicial/2000, final/2000)
	}
	v, err := Decodificar(Codificar(p))
	if err != nil || len(v) != len(p) || v[3] != p[3] {
		t.Fatal("codificación binaria")
	}
	if NumParametros(Arquitectura(20, CapasPorDefecto)) != 3457 {
		t.Fatal("parámetros de la arquitectura por defecto")
	}
}
