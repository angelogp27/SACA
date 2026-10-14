// Cargador concurrente de la Parte A: divide cada CSV en bloques de bytes
// ajustados al salto de línea, reparte los bloques entre un pool de
// goroutines trabajadoras (fan-out) y una o más goroutines agregadoras
// (fan-in) los van indexando a medida que llegan; al final se escriben en
// MongoDB. El número de goroutines trabajadoras es el parámetro de control
// del Experimento 1 (semana 7).
//
// Con una sola agregadora (-agregadoras 1, el diseño original) el
// indexado es la parte serial que limita el speedup según Amdahl. Con K
// agregadoras cada una es dueña de las estaciones cuyo hash cae en su
// partición (internal/particion): como dos agregadoras nunca tocan la
// misma clave, cada una tiene su propio índice y no hace falta ningún
// lock.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"proyecto9/internal/aqs"
	"proyecto9/internal/medicion"
	"proyecto9/internal/particion"
	"proyecto9/internal/persistencia"
	"proyecto9/internal/registro"
)

type bloque struct {
	ruta         string
	contaminante string
	inicio, fin  int64
}

func main() {
	dirDatos := flag.String("datos", "data/raw", "directorio con los 4 CSV crudos de la EPA")
	anio := flag.String("anio", "2025", "año de los archivos hourly_<codigo>_<anio>.csv")
	mongoURI := flag.String("mongo-uri", "mongodb://localhost:27017", "cadena de conexión a MongoDB")
	baseDatos := flag.String("db", "proyecto9", "nombre de la base de datos")
	goroutines := flag.Int("goroutines", 4, "numero de goroutines trabajadoras (parametro del Experimento 1)")
	bloquesPorGoroutine := flag.Int("bloques-por-goroutine", 4, "bloques por archivo y goroutine; más de 1 reparte mejor la carga entre archivos de distinto tamaño")
	agregadoras := flag.Int("agregadoras", 1, "goroutines agregadoras; cada una indexa las estaciones de su partición")
	sinMongo := flag.Bool("sin-mongo", false, "no escribir en MongoDB (mide solo el procesamiento)")
	rutaResultado := flag.String("resultado", "", "CSV al que se agrega una fila con los tiempos de esta corrida")
	flag.Parse()

	if *goroutines < 1 || *bloquesPorGoroutine < 1 || *agregadoras < 1 {
		log.Fatal("-goroutines, -bloques-por-goroutine y -agregadoras deben ser >= 1")
	}

	inicio := time.Now()

	bloques, err := despachar(aqs.Archivos(*dirDatos, *anio), *goroutines**bloquesPorGoroutine)
	if err != nil {
		log.Fatalf("error calculando los bloques: %v", err)
	}
	log.Printf("%d bloques repartidos entre %d goroutines trabajadoras y %d agregadoras", len(bloques), *goroutines, *agregadoras)

	entradaBloques := make(chan bloque)
	salidas := make([]chan aqs.ValorLeido, *agregadoras)
	for k := range salidas {
		salidas[k] = make(chan aqs.ValorLeido, 1000)
	}

	// total lo actualiza cada trabajadora al terminar un bloque. Es una
	// sección crítica (varias goroutines escriben el mismo struct), así que
	// se protege con un mutex; se toma una vez por bloque, no por fila, para
	// que la contención sea despreciable.
	var (
		total   aqs.Conteo
		muTotal sync.Mutex
	)

	var trabajadoras sync.WaitGroup
	for i := 0; i < *goroutines; i++ {
		trabajadoras.Add(1)
		go trabajador(entradaBloques, salidas, &total, &muTotal, &trabajadoras)
	}

	go func() {
		for _, b := range bloques {
			entradaBloques <- b
		}
		close(entradaBloques)
	}()

	go func() {
		trabajadoras.Wait()
		for _, s := range salidas {
			close(s)
		}
	}()

	// Agregadoras: cada una es la única que toca su índice, por eso Agregar
	// no necesita sincronización. Cada una escribe solo su propia casilla
	// de indices y tiemposIndexado.
	indices := make([]*aqs.Indice, *agregadoras)
	tiemposIndexado := make([]time.Duration, *agregadoras)
	var agregadorasWG sync.WaitGroup
	for k := range salidas {
		agregadorasWG.Add(1)
		go func(k int) {
			defer agregadorasWG.Done()
			indice := aqs.NuevoIndice()
			for v := range salidas[k] {
				t := time.Now()
				indice.Agregar(v)
				tiemposIndexado[k] += time.Since(t)
			}
			indices[k] = indice
		}(k)
	}
	agregadorasWG.Wait()

	// El tiempo de indexado que cuenta es el de la agregadora más cargada:
	// es el camino crítico de la fase serial.
	var tiempoIndexado time.Duration
	var registros int
	mapas := make([]map[string]*registro.RegistroProcesado, len(indices))
	for k, ix := range indices {
		tiempoIndexado = max(tiempoIndexado, tiemposIndexado[k])
		registros += len(ix.Registros)
		mapas[k] = ix.Registros
	}

	corrida := medicion.Corrida{
		Modo:          modo(*agregadoras),
		Goroutines:    *goroutines,
		Procesamiento: time.Since(inicio),
		Indexado:      tiempoIndexado,
		FilasLeidas:   total.Leidas,
		Registros:     int64(registros),
		MemoriaMB:     medicion.MemoriaMB(),
	}
	log.Printf("total: %s", &total)
	log.Printf("registros unicos (estacion, hora): %d", corrida.Registros)

	if !*sinMongo {
		inicioMongo := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		escritos, err := persistencia.Reemplazar(ctx, *mongoURI, *baseDatos, 5000, mapas...)
		if err != nil {
			log.Fatalf("error escribiendo en mongo: %v", err)
		}
		corrida.Mongo = time.Since(inicioMongo)
		log.Printf("registros escritos en mongo: %d", escritos)
	}

	corrida.Total = time.Since(inicio)
	log.Print(corrida)
	if *rutaResultado != "" {
		if err := medicion.Agregar(*rutaResultado, corrida); err != nil {
			log.Fatalf("no se pudo guardar el resultado: %v", err)
		}
	}
}

// despachar calcula los límites de bloque de cada archivo y arma la lista
// completa de bloques a repartir entre los trabajadores.
func despachar(archivos map[string]string, n int) ([]bloque, error) {
	var bloques []bloque
	for contaminante, ruta := range archivos {
		limites, err := aqs.LimitesBloques(ruta, n)
		if err != nil {
			return nil, fmt.Errorf("calculando bloques de %s: %w", ruta, err)
		}
		for i := 0; i < len(limites)-1; i++ {
			bloques = append(bloques, bloque{ruta: ruta, contaminante: contaminante, inicio: limites[i], fin: limites[i+1]})
		}
	}
	return bloques, nil
}

// trabajador consume bloques del channel de entrada hasta que se cierra y
// procesa cada uno de forma independiente, emitiendo sus valores válidos a
// medida que los encuentra. Cada valor va a la agregadora dueña de su
// estación.
func trabajador(entrada <-chan bloque, salidas []chan aqs.ValorLeido, total *aqs.Conteo, mu *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	enviar := func(v aqs.ValorLeido) { salidas[particion.Asignar(v.EstacionID, len(salidas))] <- v }
	for b := range entrada {
		c, err := aqs.ProcesarRango(b.ruta, b.contaminante, b.inicio, b.fin, enviar)
		if err != nil {
			log.Fatalf("error procesando %s[%d:%d]: %v", b.ruta, b.inicio, b.fin, err)
		}
		mu.Lock()
		total.Sumar(c)
		mu.Unlock()
	}
}

// modo distingue en los resultados del experimento el diseño original
// (una agregadora) del de agregadoras particionadas.
func modo(agregadoras int) string {
	if agregadoras == 1 {
		return "concurrente"
	}
	return fmt.Sprintf("concurrente_agreg%d", agregadoras)
}
