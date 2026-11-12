// Package red es un perceptrón multicapa pequeño para la misma tarea que la
// regresión logística (¿el NO2 superará el umbral en la hora siguiente?),
// pensado para el entrenamiento distribuido por mini-lotes (Fase 2 de
// docs/plan_avance.md): tiene miles de parámetros en vez de 13, así que en
// cada paso viajan decenas de KB por nodo y la red pasa a importar.
//
// Los parámetros van en un solo vector, capa por capa: primero la matriz W
// (salidas × entradas, por filas) y luego el sesgo b. Las capas ocultas usan
// ReLU y la salida, sigmoide. La pérdida es la misma log-loss ponderada por
// clase que usa la regresión logística.
package red

import (
	"encoding/binary"
	"errors"
	"math"
	"math/rand/v2"
	"sync"

	"proyecto9/internal/logistica"
)

// CapasPorDefecto: 20 variables → 64 → 32 → 1 (3 457 parámetros).
var CapasPorDefecto = []int{64, 32}

// Arquitectura completa a partir del número de variables y las ocultas.
func Arquitectura(d int, ocultas []int) []int {
	return append(append([]int{d}, ocultas...), 1)
}

// NumParametros de una arquitectura.
func NumParametros(capas []int) int {
	n := 0
	for l := 1; l < len(capas); l++ {
		n += capas[l]*capas[l-1] + capas[l]
	}
	return n
}

// Inicializar devuelve parámetros iniciales deterministas (Glorot uniforme
// con semilla fija): todos los nodos y la verificación parten de lo mismo.
func Inicializar(capas []int, semilla uint64) []float64 {
	r := rand.New(rand.NewPCG(semilla, 2026))
	p := make([]float64, NumParametros(capas))
	k := 0
	for l := 1; l < len(capas); l++ {
		in, out := capas[l-1], capas[l]
		limite := math.Sqrt(6 / float64(in+out))
		for i := 0; i < in*out; i++ {
			p[k] = (2*r.Float64() - 1) * limite
			k++
		}
		k += out // sesgos en 0
	}
	return p
}

// trabajo guarda las activaciones de un ejemplo; una por goroutine.
type trabajo struct {
	a     [][]float64 // activaciones por capa (a[0] = entrada)
	delta [][]float64
}

func nuevoTrabajo(capas []int) *trabajo {
	t := &trabajo{a: make([][]float64, len(capas)), delta: make([][]float64, len(capas))}
	for l, n := range capas {
		t.a[l] = make([]float64, n)
		t.delta[l] = make([]float64, n)
	}
	return t
}

// adelante calcula las activaciones y devuelve la probabilidad.
func adelante(capas []int, p []float64, x []float64, t *trabajo) float64 {
	copy(t.a[0], x)
	k := 0
	for l := 1; l < len(capas); l++ {
		in, out := capas[l-1], capas[l]
		w, b := p[k:k+in*out], p[k+in*out:k+in*out+out]
		prev, act := t.a[l-1], t.a[l]
		for o := 0; o < out; o++ {
			z := b[o]
			fila := w[o*in : (o+1)*in]
			for i, v := range prev {
				z += float64(fila[i] * v)
			}
			if l == len(capas)-1 {
				act[o] = logistica.Sigmoide(z)
			} else if z > 0 {
				act[o] = z
			} else {
				act[o] = 0
			}
		}
		k += in*out + out
	}
	return t.a[len(capas)-1][0]
}

// Predecir la probabilidad de un ejemplo.
func Predecir(capas []int, p, x []float64) float64 {
	return adelante(capas, p, x, nuevoTrabajo(capas))
}

// Gradiente devuelve la suma (no el promedio) del gradiente de la pérdida
// ponderada sobre X, y la suma de la pérdida. Reparte los ejemplos entre
// hilos goroutines, cada una con su propio acumulador (reducción con
// casillas privadas, sin locks); al final se suman en orden fijo, así el
// resultado no depende de cuál goroutine termina primero.
func Gradiente(capas []int, p []float64, X [][]float64, y []float64, pc logistica.PesosClase, hilos int) ([]float64, float64) {
	hilos = max(1, min(hilos, len(X)))
	grads := make([][]float64, hilos)
	perdidas := make([]float64, hilos)
	var wg sync.WaitGroup
	for h := 0; h < hilos; h++ {
		wg.Add(1)
		go func(h int) {
			defer wg.Done()
			ini, fin := h*len(X)/hilos, (h+1)*len(X)/hilos
			grads[h], perdidas[h] = gradienteRango(capas, p, X[ini:fin], y[ini:fin], pc)
		}(h)
	}
	wg.Wait()
	total, perdida := grads[0], perdidas[0]
	if total == nil {
		total = make([]float64, len(p))
	}
	for h := 1; h < hilos; h++ {
		for j, g := range grads[h] {
			total[j] += g
		}
		perdida += perdidas[h]
	}
	return total, perdida
}

func gradienteRango(capas []int, p []float64, X [][]float64, y []float64, pc logistica.PesosClase) ([]float64, float64) {
	g := make([]float64, len(p))
	t := nuevoTrabajo(capas)
	L := len(capas) - 1
	perdida := 0.0
	// Inicio de cada capa en el vector de parámetros.
	inicio := make([]int, len(capas))
	for l := 1; l < len(capas); l++ {
		inicio[l] = inicio[l-1]
		if l > 1 {
			inicio[l] += capas[l-2]*capas[l-1] + capas[l-1]
		}
	}
	for i, x := range X {
		prob := adelante(capas, p, x, t)
		peso := pc.Cero
		if y[i] == 1 {
			peso = pc.Uno
		}
		pr := min(max(prob, 1e-12), 1-1e-12)
		perdida += -peso * (y[i]*math.Log(pr) + (1-y[i])*math.Log(1-pr))
		t.delta[L][0] = peso * (prob - y[i])
		for l := L; l >= 1; l-- {
			in, out := capas[l-1], capas[l]
			k := inicio[l]
			w := p[k : k+in*out]
			gw, gb := g[k:k+in*out], g[k+in*out:k+in*out+out]
			prev := t.a[l-1]
			for o := 0; o < out; o++ {
				d := t.delta[l][o]
				gb[o] += d
				if d == 0 {
					continue
				}
				fila := gw[o*in : (o+1)*in]
				for j, v := range prev {
					fila[j] += float64(d * v)
				}
			}
			if l > 1 {
				dPrev := t.delta[l-1]
				for j := range dPrev {
					dPrev[j] = 0
				}
				for o := 0; o < out; o++ {
					d := t.delta[l][o]
					if d == 0 {
						continue
					}
					fila := w[o*in : (o+1)*in]
					for j := range dPrev {
						dPrev[j] += float64(d * fila[j])
					}
				}
				for j, a := range prev { // derivada de ReLU
					if a <= 0 {
						dPrev[j] = 0
					}
				}
			}
		}
	}
	return g, perdida
}

// Adam actualiza los parámetros con el promedio del gradiente de un paso.
type Adam struct {
	Tasa       float64
	m, v       []float64
	t          int
	b1, b2, ep float64
}

func NuevoAdam(n int, tasa float64) *Adam {
	return &Adam{Tasa: tasa, m: make([]float64, n), v: make([]float64, n), b1: 0.9, b2: 0.999, ep: 1e-8}
}

// Paso aplica grad/n (suma de gradientes de n ejemplos).
func (a *Adam) Paso(p, gradSuma []float64, n int) {
	a.t++
	c1 := 1 - math.Pow(a.b1, float64(a.t))
	c2 := 1 - math.Pow(a.b2, float64(a.t))
	for j := range p {
		g := gradSuma[j] / float64(n)
		a.m[j] = a.b1*a.m[j] + (1-a.b1)*g
		a.v[j] = a.b2*a.v[j] + (1-a.b2)*g*g
		p[j] -= a.Tasa * (a.m[j] / c1) / (math.Sqrt(a.v[j]/c2) + a.ep)
	}
}

// Lote devuelve el tramo de los n ejemplos que toca en el paso s de pasos:
// los ejemplos de cada nodo se recorren en orden, sin barajar, para que la
// verificación secuencial pueda repetir exactamente los mismos lotes.
func Lote(n, s, pasos int) (ini, fin int) {
	return s * n / pasos, (s + 1) * n / pasos
}

// Codificar y Decodificar pasan un vector a bytes (float64 little-endian)
// para mandarlo en binario dentro del mensaje JSON (base64): un tercio del
// tamaño que como números en texto, y sin perder ni un bit.
func Codificar(v []float64) []byte {
	b := make([]byte, 8*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint64(b[8*i:], math.Float64bits(x))
	}
	return b
}

func Decodificar(b []byte) ([]float64, error) {
	if len(b)%8 != 0 {
		return nil, errors.New("vector binario de largo inválido")
	}
	v := make([]float64, len(b)/8)
	for i := range v {
		v[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[8*i:]))
	}
	return v, nil
}
