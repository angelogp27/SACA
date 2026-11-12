package dataset

import "testing"

func TestVariablesRezago(t *testing.T) {
	// Medido en t-1 = 10, t-2 = 20, t-24 = 40; el resto falta.
	medido := map[int]float64{1: 10, 2: 20, 24: 40}
	h := func(k int) (float64, bool) { v, ok := medido[k]; return v, ok }
	x := VariablesRezago(5, h)
	quiero := []float64{10, 20, 5, 5, 5, 40, (10 + 20 + 40) / 3.0, 40}
	if len(x) != len(quiero) {
		t.Fatalf("%d variables", len(x))
	}
	for i := range quiero {
		if x[i] != quiero[i] {
			t.Fatalf("variable %d (%s) = %v, quiero %v", i, NombresFeatures[NumBase+i], x[i], quiero[i])
		}
	}
	// Sin ninguna hora medida: todo toma el valor actual.
	for i, v := range VariablesRezago(7, func(int) (float64, bool) { return 0, false }) {
		if v != 7 {
			t.Fatalf("variable %d = %v sin historia", i, v)
		}
	}
	if len(NombresFeatures) != NumBase+len(Rezagos)+2 {
		t.Fatal("NombresFeatures no coincide con las variables")
	}
}
