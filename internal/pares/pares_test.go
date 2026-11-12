package pares

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"testing"
)

func TestAllReduce(t *testing.T) {
	for _, topo := range []string{Arbol, Anillo} {
		for n := 1; n <= 8; n++ {
			t.Run(fmt.Sprintf("%s-%d", topo, n), func(t *testing.T) {
				nodos := make([]*Pares, n)
				dirs := make([]string, n)
				for i := range nodos {
					p, err := Escuchar("127.0.0.1:0")
					if err != nil {
						t.Fatal(err)
					}
					defer p.Cerrar()
					nodos[i], dirs[i] = p, fmt.Sprintf("127.0.0.1:%d", p.Puerto())
				}
				const largo = 1003 // no divisible entre n: trozos desiguales en el anillo
				vec := make([][]float64, n)
				suma := make([]float64, largo)
				for i := range vec {
					r := rand.New(rand.NewPCG(uint64(i), 5))
					vec[i] = make([]float64, largo)
					for j := range vec[i] {
						vec[i][j] = r.NormFloat64()
						suma[j] += vec[i][j]
					}
				}
				var wg sync.WaitGroup
				errs := make([]error, n)
				for i := range nodos {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						g, err := nodos[i].Unirse(context.Background(), 7, i, dirs, topo)
						if err != nil {
							errs[i] = err
							return
						}
						defer g.Cerrar()
						for rep := 0; rep < 3; rep++ { // varias veces por las mismas conexiones
							v := append([]float64(nil), vec[i]...)
							if err := g.AllReduce(v); err != nil {
								errs[i] = err
								return
							}
							if rep == 2 {
								vec[i] = v
							}
						}
					}(i)
				}
				wg.Wait()
				for i, err := range errs {
					if err != nil {
						t.Fatalf("nodo %d: %v", i, err)
					}
				}
				for i := 1; i < n; i++ {
					for j := range vec[0] {
						if math.Float64bits(vec[i][j]) != math.Float64bits(vec[0][j]) {
							t.Fatalf("nodo %d componente %d distinto del nodo 0", i, j)
						}
					}
				}
				for j := range suma {
					if math.Abs(vec[0][j]-suma[j]) > 1e-9 {
						t.Fatalf("componente %d: %v, la suma es %v", j, vec[0][j], suma[j])
					}
				}
			})
		}
	}
}
