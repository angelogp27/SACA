package distribuido

import (
	"fmt"

	"proyecto9/internal/logistica"
)

// Parcial es lo que un nodo devuelve en gradiente_parcial: sumas (no
// promedios) sobre sus N ejemplos.
type Parcial struct {
	Grad      []float64
	GradSesgo float64
	Perdida   float64
	N         int
}

// Actualizar combina los gradientes parciales de una época y aplica el
// paso de descenso de gradiente al modelo, con la misma fórmula que
// logistica.Modelo.Entrenar: como el gradiente es una suma sobre ejemplos
// independientes, sumar las particiones da el gradiente de todo el
// conjunto. Las sumas se hacen en el orden de las particiones para que el
// resultado no dependa del orden de llegada por la red. Devuelve la
// pérdida promedio de la época.
//
// nTotal es el número de ejemplos de entrenamiento: si las particiones no
// suman exactamente eso, algún nodo tiene otros datos y se devuelve error.
func Actualizar(m *logistica.Modelo, tasa float64, parciales []Parcial, nTotal int) (float64, error) {
	grad := make([]float64, len(m.Pesos))
	var gradSesgo, perdida float64
	n := 0
	for i, p := range parciales {
		if len(p.Grad) != len(grad) {
			return 0, fmt.Errorf("partición %d: gradiente de %d componentes, se esperaban %d", i, len(p.Grad), len(grad))
		}
		for j, g := range p.Grad {
			grad[j] += g
		}
		gradSesgo += p.GradSesgo
		perdida += p.Perdida
		n += p.N
	}
	if n != nTotal {
		return 0, fmt.Errorf("las particiones suman %d ejemplos y el entrenamiento tiene %d", n, nTotal)
	}
	for j := range m.Pesos {
		m.Pesos[j] -= tasa * grad[j] / float64(n)
	}
	m.Sesgo -= tasa * gradSesgo / float64(n)
	return perdida / float64(n), nil
}
