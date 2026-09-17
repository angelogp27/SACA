// Package persistencia escribe el índice de registros procesados en
// MongoDB. La usan tanto el cargador secuencial como el concurrente, que
// solo difieren en cómo construyen el índice, no en cómo lo persisten.
package persistencia

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"proyecto9/internal/registro"
)

// EscribirLotes inserta el índice en coleccion en lotes de tamanoLote
// documentos y devuelve cuántos escribió. Borra cada entrada del índice
// apenas la agrega a un lote, para que el recolector de basura pueda
// liberarla antes de terminar de recorrer el mapa entero — sin esto, el
// pico de memoria duplica el tamaño del índice completo justo cuando más
// se necesita esa memoria libre (para los buffers de red del driver).
func EscribirLotes(ctx context.Context, coleccion *mongo.Collection, indice map[string]*registro.RegistroProcesado, tamanoLote int) (int64, error) {
	lote := make([]any, 0, tamanoLote)
	var escritos int64

	for clave, reg := range indice {
		lote = append(lote, reg)
		delete(indice, clave)
		if len(lote) == tamanoLote {
			if _, err := coleccion.InsertMany(ctx, lote); err != nil {
				return escritos, err
			}
			escritos += int64(len(lote))
			lote = lote[:0]
		}
	}
	if len(lote) > 0 {
		if _, err := coleccion.InsertMany(ctx, lote); err != nil {
			return escritos, err
		}
		escritos += int64(len(lote))
	}
	return escritos, nil
}
