// Package cluster lleva el registro de nodos trabajadores del maestro
// (Parte B): quién está conectado y qué partición de datos tiene cada uno.
// Cada conexión la atiende su propia goroutine, así que el registro es
// estado compartido entre goroutines: todas sus operaciones pasan por un
// sync.Mutex (sección crítica).
package cluster

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"proyecto9/internal/protocolo"
)

var (
	ErrLleno     = errors.New("todas las particiones ya están asignadas")
	ErrDuplicado = errors.New("ya hay un nodo registrado con ese worker_id")
)

type Nodo struct {
	ID        string
	Particion int
	Conexion  *protocolo.Conexion
}

type Registro struct {
	mu          sync.Mutex
	particiones int
	nodos       map[string]*Nodo
	// duenos[p] es el worker_id que tiene la partición p ("" si está libre).
	duenos []string
}

// NuevoRegistro prepara un registro para n particiones (una por nodo
// esperado).
func NuevoRegistro(n int) *Registro {
	return &Registro{particiones: n, nodos: map[string]*Nodo{}, duenos: make([]string, n)}
}

// Registrar da de alta un nodo y le asigna la partición libre más baja.
// Dos nodos nunca reciben la misma partición; si un nodo se dio de baja,
// su partición queda libre para el siguiente que se registre.
func (r *Registro) Registrar(id string, c *protocolo.Conexion) (*Nodo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, existe := r.nodos[id]; existe {
		return nil, fmt.Errorf("%w: %s", ErrDuplicado, id)
	}
	for p, dueno := range r.duenos {
		if dueno == "" {
			n := &Nodo{ID: id, Particion: p, Conexion: c}
			r.duenos[p] = id
			r.nodos[id] = n
			return n, nil
		}
	}
	return nil, ErrLleno
}

// DarDeBaja quita el nodo y libera su partición. Devuelve false si el
// nodo ya no estaba (p. ej. envió "baja" y luego se cortó la conexión).
func (r *Registro) DarDeBaja(id string) (*Nodo, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n, existe := r.nodos[id]
	if !existe {
		return nil, false
	}
	delete(r.nodos, id)
	r.duenos[n.Particion] = ""
	return n, true
}

// Activos devuelve una copia de los nodos registrados, ordenada por
// partición; se puede recorrer sin tener el lock.
func (r *Registro) Activos() []Nodo {
	r.mu.Lock()
	defer r.mu.Unlock()

	activos := make([]Nodo, 0, len(r.nodos))
	for _, n := range r.nodos {
		activos = append(activos, *n)
	}
	sort.Slice(activos, func(i, j int) bool { return activos[i].Particion < activos[j].Particion })
	return activos
}

func (r *Registro) Particiones() int { return r.particiones }

// Completo indica si todas las particiones tienen dueño.
func (r *Registro) Completo() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.nodos) == r.particiones
}
