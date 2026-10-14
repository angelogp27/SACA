// Package metricas evalúa un clasificador binario. Como el problema está
// desbalanceado (pocas horas cruzan el umbral), la exactitud sola engaña:
// se reportan precisión, exhaustividad, F1 y AUC-ROC (plano 3 de la
// evaluación, .context/proyecto9.md §8).
package metricas

import (
	"fmt"
	"sort"
)

type Clasificacion struct {
	VP, FP, VN, FN int
	Exactitud      float64
	Precision      float64
	Exhaustividad  float64
	F1             float64
	AUC            float64
}

// Evaluar compara las probabilidades predichas con las etiquetas reales
// usando umbral para decidir la clase.
func Evaluar(y, probabilidades []float64, umbral float64) Clasificacion {
	var c Clasificacion
	for i, p := range probabilidades {
		predicha := p >= umbral
		real := y[i] == 1
		switch {
		case predicha && real:
			c.VP++
		case predicha && !real:
			c.FP++
		case !predicha && !real:
			c.VN++
		default:
			c.FN++
		}
	}
	div := func(a, b int) float64 {
		if b == 0 {
			return 0
		}
		return float64(a) / float64(b)
	}
	c.Exactitud = div(c.VP+c.VN, len(y))
	c.Precision = div(c.VP, c.VP+c.FP)
	c.Exhaustividad = div(c.VP, c.VP+c.FN)
	if c.Precision+c.Exhaustividad > 0 {
		c.F1 = 2 * c.Precision * c.Exhaustividad / (c.Precision + c.Exhaustividad)
	}
	c.AUC = AUC(y, probabilidades)
	return c
}

// AUC calcula el área bajo la curva ROC con la fórmula de Mann-Whitney:
// la probabilidad de que un positivo al azar reciba una probabilidad mayor
// que un negativo al azar. Los empates se resuelven con rangos promedio.
func AUC(y, probabilidades []float64) float64 {
	idx := make([]int, len(y))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return probabilidades[idx[a]] < probabilidades[idx[b]] })

	var sumaRangosPos float64
	var positivos, negativos int
	for i := 0; i < len(idx); {
		j := i
		for j < len(idx) && probabilidades[idx[j]] == probabilidades[idx[i]] {
			j++
		}
		rango := float64(i+j+1) / 2 // rango promedio (1-indexado) del grupo empatado
		for k := i; k < j; k++ {
			if y[idx[k]] == 1 {
				sumaRangosPos += rango
				positivos++
			} else {
				negativos++
			}
		}
		i = j
	}
	if positivos == 0 || negativos == 0 {
		return 0
	}
	return (sumaRangosPos - float64(positivos*(positivos+1))/2) / float64(positivos*negativos)
}

func (c Clasificacion) String() string {
	return fmt.Sprintf("VP=%d FP=%d VN=%d FN=%d | exactitud=%.3f precision=%.3f exhaustividad=%.3f F1=%.3f AUC=%.3f",
		c.VP, c.FP, c.VN, c.FN, c.Exactitud, c.Precision, c.Exhaustividad, c.F1, c.AUC)
}
