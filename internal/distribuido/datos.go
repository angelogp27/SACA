// Package distribuido reúne lo que comparten el maestro y los trabajadores
// del entrenamiento distribuido por TCP (Parte B, docs/contrato_protocolo_tcp.md):
// la preparación local de la muestra, el filtrado por partición, la
// agregación de los gradientes parciales y los motores de cálculo de cada
// nodo (goroutines en Go o un proceso externo en CUDA).
package distribuido

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"proyecto9/internal/dataset"
	"proyecto9/internal/entrenador"
	"proyecto9/internal/particion"
)

// PrepararLocal carga la caché de la muestra (data/muestra.gob) y la
// prepara igual que entrenador.Preparar, sin MongoDB. Devuelve también la
// huella del archivo: si dos nodos tienen la misma huella, tienen los
// mismos datos y, como la preparación es determinista, la misma
// normalización y los mismos pesos de clase.
func PrepararLocal(rutaMuestra string, mesCorte int) (*entrenador.Datos, string, error) {
	huella, err := Huella(rutaMuestra)
	if err != nil {
		return nil, "", err
	}
	muestra, err := dataset.Cargar(rutaMuestra)
	if err != nil {
		return nil, "", err
	}
	datos, err := entrenador.PrepararDesde(muestra, mesCorte)
	if err != nil {
		return nil, "", err
	}
	return datos, huella, nil
}

// Huella es el SHA-256 del archivo, en hexadecimal.
func Huella(ruta string) (string, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Corta abrevia una huella para mostrarla en el log.
func Corta(huella string) string {
	if len(huella) > 12 {
		return huella[:12]
	}
	return huella
}

// Filtrar devuelve los ejemplos cuyas estaciones caen en la partición
// resto de mod (el filtro que el maestro envía en particion_asignada). Las
// filas de X se comparten con la muestra original, no se copian.
func Filtrar(m *dataset.Muestra, mod, resto int) (X [][]float64, y []float64) {
	for i, estacion := range m.EstacionID {
		if particion.Pertenece(estacion, mod, resto) {
			X = append(X, m.X[i])
			y = append(y, m.Y[i])
		}
	}
	return X, y
}
