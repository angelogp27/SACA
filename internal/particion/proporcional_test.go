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

func TestRepartirPorFilas(t *testing.T) {
	// 10 cubetas de 100 filas, algunas vacías.
	filas := []int{100, 0, 100, 100, 0, 100, 100, 100, 100, 100}
	restos, err := RepartirPorFilas([]float64{3, 1}, filas)
	if err != nil {
		t.Fatal(err)
	}
	cuenta := func(rs []int) (s int) {
		for _, r := range rs {
			s += filas[r]
		}
		return
	}
	if a, b := cuenta(restos[0]), cuenta(restos[1]); a != 600 || b != 200 {
		t.Fatalf("filas: %d y %d (se esperaban 600 y 200): %v", a, b, restos)
	}
	vistas := map[int]bool{}
	for _, rs := range restos {
		for _, r := range rs {
			if vistas[r] {
				t.Fatalf("cubeta %d repetida", r)
			}
			vistas[r] = true
		}
	}
	if len(vistas) != len(filas) {
		t.Fatalf("faltan cubetas: %v", restos)
	}
	// Un peso ínfimo igual recibe filas.
	filas = append(filas, filas...)
	restos, _ = RepartirPorFilas([]float64{1000, 1}, filas)
	if cuenta(restos[1]) == 0 || cuenta(restos[0])+cuenta(restos[1]) != 1600 {
		t.Fatalf("el nodo de peso mínimo quedó sin filas: %v", restos)
	}
}
