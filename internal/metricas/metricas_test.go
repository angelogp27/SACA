package metricas

import (
	"math"
	"testing"
)

func TestEvaluar(t *testing.T) {
	y := []float64{1, 1, 0, 0, 0, 1}
	p := []float64{0.9, 0.4, 0.6, 0.1, 0.2, 0.8}
	c := Evaluar(y, p, 0.5)
	if c.VP != 2 || c.FN != 1 || c.FP != 1 || c.VN != 2 {
		t.Fatalf("matriz de confusion = %+v", c)
	}
	if math.Abs(c.Precision-2.0/3) > 1e-12 || math.Abs(c.Exhaustividad-2.0/3) > 1e-12 {
		t.Fatalf("precision/exhaustividad = %v/%v", c.Precision, c.Exhaustividad)
	}
}

func TestAUC(t *testing.T) {
	if a := AUC([]float64{0, 0, 1, 1}, []float64{0.1, 0.2, 0.8, 0.9}); a != 1 {
		t.Fatalf("AUC perfecta = %v", a)
	}
	if a := AUC([]float64{0, 1}, []float64{0.5, 0.5}); a != 0.5 {
		t.Fatalf("AUC con empate = %v", a)
	}
	// 3 de 4 pares positivo-negativo bien ordenados.
	if a := AUC([]float64{0, 1, 0, 1}, []float64{0.1, 0.3, 0.4, 0.9}); math.Abs(a-0.75) > 1e-12 {
		t.Fatalf("AUC = %v", a)
	}
}
