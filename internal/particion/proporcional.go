package particion

import (
	"fmt"
	"slices"
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

// ParticipacionMinima: nadie recibe menos de esta fracción de las filas,
// para que todo nodo de la corrida tenga ejemplos que calcular.
const ParticipacionMinima = 0.02

// RepartirPorFilas asigna las cubetas 0..len(filas)-1 a len(pesos) dueños
// de modo que las FILAS (no las cubetas) queden en proporción a los pesos:
// filas[b] es cuántas filas de la muestra caen en la cubeta b. Las cubetas
// se recorren en orden y cada una va al dueño en cuyo tramo cae su punto
// medio, así el resultado es determinista. Las cubetas vacías no importan;
// cada dueño recibe al menos ParticipacionMinima de las filas.
func RepartirPorFilas(pesos []float64, filas []int) ([][]int, error) {
	n := len(pesos)
	if n == 0 || len(filas) < n {
		return nil, fmt.Errorf("no se pueden repartir %d cubetas entre %d nodos", len(filas), n)
	}
	total := 0
	for _, f := range filas {
		total += f
	}
	var suma float64
	for i, p := range pesos {
		if !(p > 0) {
			return nil, fmt.Errorf("el peso del nodo %d debe ser positivo (es %g)", i, p)
		}
		suma += p
	}
	// Fracciones con el mínimo, renormalizadas.
	frac := make([]float64, n)
	var sumaFrac float64
	for i, p := range pesos {
		frac[i] = max(p/suma, ParticipacionMinima)
		sumaFrac += frac[i]
	}
	limites := make([]float64, n) // fin del tramo de cada dueño, en filas
	acum := 0.0
	for i := range frac {
		acum += frac[i] / sumaFrac * float64(total)
		limites[i] = acum
	}
	restos := make([][]int, n)
	dueno, cum := 0, 0
	for b, f := range filas {
		medio := float64(cum) + float64(f)/2
		for dueno < n-1 && medio >= limites[dueno] {
			dueno++
		}
		restos[dueno] = append(restos[dueno], b)
		cum += f
	}
	// Con cubetas gruesas, un dueño de peso muy chico puede quedarse sin
	// filas: toma la cubeta con filas más cercana del vecino que más tiene.
	filasDe := func(rs []int) (s int) {
		for _, r := range rs {
			s += filas[r]
		}
		return
	}
	for i := range restos {
		if filasDe(restos[i]) > 0 {
			continue
		}
		v := -1
		for _, j := range []int{i - 1, i + 1} {
			if j >= 0 && j < n && filasDe(restos[j]) > 0 && len(restos[j]) > 1 && (v < 0 || filasDe(restos[j]) > filasDe(restos[v])) {
				v = j
			}
		}
		if v < 0 {
			continue
		}
		// La cubeta con filas del lado que da hacia i.
		k := len(restos[v]) - 1
		paso := -1
		if v > i {
			k, paso = 0, 1
		}
		for ; k >= 0 && k < len(restos[v]) && filas[restos[v][k]] == 0; k += paso {
		}
		if k < 0 || k >= len(restos[v]) {
			continue
		}
		b := restos[v][k]
		restos[v] = append(restos[v][:k:k], restos[v][k+1:]...)
		restos[i] = append(restos[i], b)
		slices.Sort(restos[i])
	}
	return restos, nil
}
