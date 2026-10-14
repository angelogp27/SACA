package particion

import (
	"fmt"
	"testing"
)

func TestAsignarEsDeterministaYBalanceada(t *testing.T) {
	const n = 4
	conteo := make([]int, n)
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("%02d-%03d-%04d", i%56, i%97, i)
		p := Asignar(id, n)
		if p != Asignar(id, n) {
			t.Fatalf("%s cambió de partición", id)
		}
		if !Pertenece(id, n, p) {
			t.Fatalf("%s no pertenece a su propia partición", id)
		}
		conteo[p]++
	}
	for p, c := range conteo {
		// Con 1000 estaciones, cada partición debería tener ~250.
		if c < 180 || c > 320 {
			t.Fatalf("partición %d desbalanceada: %v", p, conteo)
		}
	}
}

func TestAsignarConUnaParticion(t *testing.T) {
	if Asignar("06-037-0002", 1) != 0 || Asignar("06-037-0002", 0) != 0 {
		t.Fatal("con n<=1 todo cae en la partición 0")
	}
}
