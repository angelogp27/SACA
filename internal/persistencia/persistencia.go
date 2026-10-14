// Package persistencia escribe el índice de registros procesados en
// MongoDB. La usan tanto el cargador secuencial como el concurrente, que
// solo difieren en cómo construyen el índice, no en cómo lo persisten.
package persistencia

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"proyecto9/internal/registro"
)

// Coleccion es donde viven los registros procesados (contrato
// docs/contrato_registro_procesado.md).
const Coleccion = "registros_procesados"

// Reemplazar se conecta a MongoDB, vacía la colección de registros
// procesados y la vuelve a llenar con los índices dados (uno por
// agregadora en el cargador concurrente; sus claves son disjuntas), un
// escritor concurrente por índice. Es el paso final común de los dos
// cargadores.
func Reemplazar(ctx context.Context, uri, baseDatos string, tamanoLote int, indices ...map[string]*registro.RegistroProcesado) (int64, error) {
	cliente, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return 0, fmt.Errorf("conectando a mongo: %w", err)
	}
	defer cliente.Disconnect(ctx)

	coleccion := cliente.Database(baseDatos).Collection(Coleccion)
	if err := coleccion.Drop(ctx); err != nil {
		return 0, fmt.Errorf("limpiando la coleccion: %w", err)
	}

	// Un escritor por índice: los índices tienen claves disjuntas, así que
	// cada goroutine inserta lo suyo sin coordinarse con las demás (el
	// cliente de Mongo es seguro para uso concurrente). Con un solo índice
	// esto se reduce a la escritura serial original.
	escritos := make([]int64, len(indices))
	errores := make([]error, len(indices))
	var wg sync.WaitGroup
	for k, indice := range indices {
		wg.Add(1)
		go func(k int, indice map[string]*registro.RegistroProcesado) {
			defer wg.Done()
			escritos[k], errores[k] = EscribirLotes(ctx, coleccion, indice, tamanoLote)
		}(k, indice)
	}
	wg.Wait()

	var total int64
	for _, n := range escritos {
		total += n
	}
	return total, errors.Join(errores...)
}

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
