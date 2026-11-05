package particion

import (
	"fmt"
	"testing"
)

func TestRepartirProporcionalCubreTodasLasCubetasUnaVez(t *testing.T) {
	for _, pesos := range [][]float64{{1, 1}, {1, 3}, {1, 1, 6}, {0.3, 0.3, 0.3}, {1, 1000}} {
		restos, err := RepartirProporcional(pesos, BucketsReparto)
		if err != nil {
			t.Fatal(err)
		}
		visto := make([]int, BucketsReparto)
		for _, rs := range restos {
			if len(rs) == 0 {
				t.Fatalf("pesos %v: un nodo se quedó sin cubetas", pesos)
			}
			for _, r := range rs {
				visto[r]++
			}
		}
		for c, v := range visto {
			if v != 1 {
				t.Fatalf("pesos %v: la cubeta %d tiene %d dueños", pesos, c, v)
			}
		}
	}
}

func TestRepartirProporcionalRespetaLosPesos(t *testing.T) {
	restos, _ := RepartirProporcional([]float64{1, 3}, 1000)
	if len(restos[0]) != 250 || len(restos[1]) != 750 {
		t.Fatalf("1:3 de 1000 cubetas dio %d y %d", len(restos[0]), len(restos[1]))
	}
}

func TestRepartirProporcionalRechazaPesosInvalidos(t *testing.T) {
	if _, err := RepartirProporcional([]float64{1, 0}, 16); err == nil {
		t.Fatal("un peso cero debía fallar")
	}
	if _, err := RepartirProporcional(nil, 16); err == nil {
		t.Fatal("sin nodos debía fallar")
	}
	if _, err := RepartirProporcional([]float64{1, 1, 1}, 2); err == nil {
		t.Fatal("menos cubetas que nodos debía fallar")
	}
}

func TestPerteneceAEquivaleAPertenece(t *testing.T) {
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("%02d-%03d-%04d", i%56, i%97, i)
		if PerteneceA(id, 8, []int{3}) != Pertenece(id, 8, 3) {
			t.Fatalf("%s: PerteneceA y Pertenece difieren", id)
		}
	}
}
