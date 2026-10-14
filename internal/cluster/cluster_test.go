package cluster

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// Muchos nodos registrándose a la vez desde goroutines distintas: con
// -race verifica que el registro no tiene carreras, y que nunca se asigna
// dos veces la misma partición.
func TestRegistroConcurrente(t *testing.T) {
	const particiones = 8
	r := NuevoRegistro(particiones)

	var wg sync.WaitGroup
	var mu sync.Mutex
	asignadas := map[int]string{}
	llenos := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			n, err := r.Registrar(fmt.Sprintf("w%d", i), nil)
			mu.Lock()
			defer mu.Unlock()
			if errors.Is(err, ErrLleno) {
				llenos++
				return
			}
			if err != nil {
				t.Error(err)
				return
			}
			if otro, ya := asignadas[n.Particion]; ya {
				t.Errorf("partición %d asignada a %s y a %s", n.Particion, otro, n.ID)
			}
			asignadas[n.Particion] = n.ID
		}(i)
	}
	wg.Wait()

	if len(asignadas) != particiones || llenos != 20-particiones || !r.Completo() {
		t.Fatalf("asignadas=%d llenos=%d completo=%v", len(asignadas), llenos, r.Completo())
	}
}

func TestBajaLiberaLaParticion(t *testing.T) {
	r := NuevoRegistro(2)
	r.Registrar("w1", nil)
	w2, _ := r.Registrar("w2", nil)

	if _, err := r.Registrar("w1", nil); !errors.Is(err, ErrDuplicado) {
		t.Fatalf("se esperaba ErrDuplicado, llegó %v", err)
	}
	if _, ok := r.DarDeBaja("w2"); !ok {
		t.Fatal("w2 debía estar registrado")
	}
	if _, ok := r.DarDeBaja("w2"); ok {
		t.Fatal("una segunda baja de w2 no debe hacer nada")
	}
	w3, err := r.Registrar("w3", nil)
	if err != nil || w3.Particion != w2.Particion {
		t.Fatalf("w3 debía heredar la partición %d: %+v %v", w2.Particion, w3, err)
	}
	if activos := r.Activos(); len(activos) != 2 || activos[0].ID != "w1" || activos[1].ID != "w3" {
		t.Fatalf("activos = %+v", activos)
	}
}
