// Package medicion registra los tiempos por fase de cada corrida de los
// cargadores, para el Experimento 1 y el análisis de Amdahl. Cada corrida
// agrega una fila a un CSV; experimentos/cmd/analisis lo resume después.
package medicion

import (
	"encoding/csv"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"
)

// Corrida es una ejecución completa de un cargador.
//
//   - Procesamiento: lectura + limpieza + indexado, hasta tener el índice
//     completo. En el concurrente, la lectura/limpieza corre en paralelo y
//     el indexado se solapa con ella en la goroutina agregadora.
//   - Indexado: tiempo acumulado dentro de Indice.Agregar. Es trabajo
//     serial en ambos cargadores (una sola goroutina toca el índice).
//   - Mongo: escritura por lotes, también serial.
type Corrida struct {
	Modo          string
	Goroutines    int
	Procesamiento time.Duration
	Indexado      time.Duration
	Mongo         time.Duration
	Total         time.Duration
	FilasLeidas   int64
	Registros     int64
	MemoriaMB     float64
}

var encabezado = []string{"modo", "goroutines", "t_procesamiento_s", "t_indexado_s", "t_mongo_s", "t_total_s", "filas_leidas", "registros", "memoria_mb"}

// Agregar añade c como una fila de ruta, escribiendo el encabezado si el
// archivo es nuevo.
func Agregar(ruta string, c Corrida) error {
	_, errStat := os.Stat(ruta)
	nuevo := os.IsNotExist(errStat)

	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if nuevo {
		if err := w.Write(encabezado); err != nil {
			return err
		}
	}
	segundos := func(d time.Duration) string { return strconv.FormatFloat(d.Seconds(), 'f', 3, 64) }
	err = w.Write([]string{
		c.Modo,
		strconv.Itoa(c.Goroutines),
		segundos(c.Procesamiento),
		segundos(c.Indexado),
		segundos(c.Mongo),
		segundos(c.Total),
		strconv.FormatInt(c.FilasLeidas, 10),
		strconv.FormatInt(c.Registros, 10),
		strconv.FormatFloat(c.MemoriaMB, 'f', 0, 64),
	})
	if err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

// MemoriaMB devuelve la memoria que el runtime de Go ha pedido al sistema
// operativo. Nunca baja durante la ejecución, así que leída justo después
// de construir el índice aproxima el pico de memoria del cargador.
func MemoriaMB() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.Sys) / (1 << 20)
}

func (c Corrida) String() string {
	return fmt.Sprintf("T(%d) %s: total=%s procesamiento=%s indexado=%s mongo=%s memoria=%.0f MB",
		c.Goroutines, c.Modo, c.Total.Round(time.Millisecond), c.Procesamiento.Round(time.Millisecond),
		c.Indexado.Round(time.Millisecond), c.Mongo.Round(time.Millisecond), c.MemoriaMB)
}
