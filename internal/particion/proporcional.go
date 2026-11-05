package particion

import (
	"fmt"
	"sort"
)

// BucketsReparto es el número de cubetas (restos del hash) en que se divide
// el espacio de estaciones para un reparto proporcional: cuanto más cubetas,
// más fino puede ser el reparto entre nodos de distinta velocidad.
const BucketsReparto = 1024

// RepartirProporcional asigna las mod cubetas 0..mod-1 a len(pesos) dueños,
// cada uno con una cantidad proporcional a su peso (método del mayor
// resto: la suma es exactamente mod y nadie con peso positivo se queda sin
// ninguna). Devuelve, por dueño, la lista de cubetas que le tocan. Una
// estación pertenece al dueño que tiene la cubeta Asignar(estacion, mod).
func RepartirProporcional(pesos []float64, mod int) ([][]int, error) {
	n := len(pesos)
	if n == 0 || mod < n {
		return nil, fmt.Errorf("no se pueden repartir %d cubetas entre %d nodos", mod, n)
	}
	var suma float64
	for i, p := range pesos {
		if !(p > 0) {
			return nil, fmt.Errorf("el peso del nodo %d debe ser positivo (es %g)", i, p)
		}
		suma += p
	}
	cuota := make([]int, n)
	resto := make([]float64, n)
	asignadas := 0
	for i, p := range pesos {
		exacto := p / suma * float64(mod)
		cuota[i] = max(int(exacto), 1)
		resto[i] = exacto - float64(int(exacto))
		asignadas += cuota[i]
	}
	orden := make([]int, n)
	for i := range orden {
		orden[i] = i
	}
	sort.SliceStable(orden, func(a, b int) bool { return resto[orden[a]] > resto[orden[b]] })
	// Faltan cubetas: se dan a quien tiene mayor resto. Sobran (por el
	// mínimo de 1): se quitan a quien tiene más cubetas y menos resto.
	for k := 0; asignadas < mod; k++ {
		cuota[orden[k%n]]++
		asignadas++
	}
	for k := n - 1; asignadas > mod; k-- {
		i := orden[((k%n)+n)%n]
		if cuota[i] > 1 {
			cuota[i]--
			asignadas--
		}
	}
	restos := make([][]int, n)
	cubeta := 0
	for i := range restos {
		for j := 0; j < cuota[i]; j++ {
			restos[i] = append(restos[i], cubeta)
			cubeta++
		}
	}
	return restos, nil
}

// PerteneceA indica si estacionID cae en alguna de las cubetas restos de mod.
func PerteneceA(estacionID string, mod int, restos []int) bool {
	c := Asignar(estacionID, mod)
	for _, r := range restos {
		if r == c {
			return true
		}
	}
	return false
}
