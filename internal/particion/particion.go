// Package particion reparte estaciones de forma determinista entre n
// dueños (agregadoras del cargador, goroutines o nodos trabajadores de la
// Parte B) por hash de estacion_id. Todas las lecturas de una misma
// estación caen siempre en la misma partición, y la asignación no cambia
// entre corridas ni entre épocas: es el criterio "estacion_id_hash_mod" /
// "estacion_id_hash_resto" de docs/contrato_protocolo_tcp.md.
package particion

import "hash/fnv"

// Asignar devuelve la partición (0..n-1) de estacionID.
func Asignar(estacionID string, n int) int {
	if n <= 1 {
		return 0
	}
	h := fnv.New32a()
	h.Write([]byte(estacionID))
	return int(h.Sum32() % uint32(n))
}

// Pertenece indica si estacionID cae en la partición resto de mod.
func Pertenece(estacionID string, mod, resto int) bool {
	return Asignar(estacionID, mod) == resto
}
