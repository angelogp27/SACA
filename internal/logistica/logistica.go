// Package logistica implementa regresión logística por descenso de
// gradiente desde cero, sin librerías de ML. GradienteParcial es la pieza
// que se reutiliza sin cambios en la Parte B distribuida: cada nodo
// trabajador la corre sobre su propia partición y el maestro suma los
// resultados (ver docs/contrato_protocolo_tcp.md, mensaje
// "gradiente_parcial").
package logistica

import "math"

func Sigmoide(z float64) float64 {
	return 1 / (1 + math.Exp(-z))
}

// PesosClase multiplica el error y la pérdida de cada ejemplo según su
// etiqueta, para compensar un dataset desbalanceado. Cero=Uno=1 equivale a
// no ponderar. Se calculan una sola vez sobre el conjunto de entrenamiento
// completo (ver CalcularPesosClase) y se pasan igual a todas las
// particiones — no tendría sentido que cada nodo trabajador de la Parte B
// los recalculara sobre su propio subconjunto, porque el desbalance real
// es una propiedad del dataset entero, no de una partición.
type PesosClase struct {
	Cero float64
	Uno  float64
}

// PesosClaseNeutros no pondera nada; es el valor por defecto si no hay
// desbalance que compensar.
var PesosClaseNeutros = PesosClase{Cero: 1, Uno: 1}

// CalcularPesosClase pondera cada clase en proporción inversa a su
// frecuencia (n / (2 * n_clase)), la fórmula estándar de "balanced class
// weight": la clase minoritaria pesa más en el gradiente y en la pérdida.
func CalcularPesosClase(y []float64) PesosClase {
	var positivos float64
	for _, v := range y {
		if v == 1 {
			positivos++
		}
	}
	n := float64(len(y))
	negativos := n - positivos

	pesos := PesosClaseNeutros
	if positivos > 0 {
		pesos.Uno = n / (2 * positivos)
	}
	if negativos > 0 {
		pesos.Cero = n / (2 * negativos)
	}
	return pesos
}

// GradienteParcial calcula, sobre un conjunto de ejemplos, la suma (no el
// promedio) del gradiente de la log-verosimilitud negativa ponderada
// respecto a pesos y sesgo, más la suma de la pérdida ponderada. Sumar los
// gradientes parciales de varias particiones disjuntas da el mismo
// resultado que calcularlo sobre el conjunto completo de una sola vez,
// porque el gradiente de la regresión logística es una suma sobre
// ejemplos independientes.
func GradienteParcial(pesos []float64, sesgo float64, X [][]float64, y []float64, pesosClase PesosClase) (gradPesos []float64, gradSesgo, perdida float64) {
	gradPesos = make([]float64, len(pesos))
	for i, xi := range X {
		z := sesgo
		for j, xij := range xi {
			z += pesos[j] * xij
		}
		pred := Sigmoide(z)

		pesoClase := pesosClase.Cero
		if y[i] == 1 {
			pesoClase = pesosClase.Uno
		}

		error := pesoClase * (pred - y[i])
		for j, xij := range xi {
			gradPesos[j] += error * xij
		}
		gradSesgo += error
		perdida += pesoClase * logLoss(y[i], pred)
	}
	return gradPesos, gradSesgo, perdida
}

func logLoss(yReal, yPred float64) float64 {
	const eps = 1e-15
	yPred = math.Min(math.Max(yPred, eps), 1-eps)
	return -(yReal*math.Log(yPred) + (1-yReal)*math.Log(1-yPred))
}

type Modelo struct {
	Pesos []float64
	Sesgo float64
}

func NuevoModelo(numFeatures int) *Modelo {
	return &Modelo{Pesos: make([]float64, numFeatures)}
}

func (m *Modelo) Predecir(x []float64) float64 {
	z := m.Sesgo
	for j, xj := range x {
		z += m.Pesos[j] * xj
	}
	return Sigmoide(z)
}

// Entrenar corre descenso de gradiente por lote completo: en cada época
// calcula el gradiente sobre todo X (vía GradienteParcial) y actualiza los
// parámetros con su promedio. Devuelve la pérdida promedio ponderada de
// cada época.
func (m *Modelo) Entrenar(X [][]float64, y []float64, tasaAprendizaje float64, epocas int, pesosClase PesosClase) []float64 {
	perdidaPorEpoca := make([]float64, 0, epocas)
	n := float64(len(X))

	for e := 0; e < epocas; e++ {
		gradPesos, gradSesgo, perdida := GradienteParcial(m.Pesos, m.Sesgo, X, y, pesosClase)
		for j := range m.Pesos {
			m.Pesos[j] -= tasaAprendizaje * gradPesos[j] / n
		}
		m.Sesgo -= tasaAprendizaje * gradSesgo / n
		perdidaPorEpoca = append(perdidaPorEpoca, perdida/n)
	}

	return perdidaPorEpoca
}
