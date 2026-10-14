// Cargador concurrente de la Parte A: divide cada CSV en bloques de bytes
// ajustados al salto de línea, reparte los bloques entre un pool de
// goroutines trabajadoras (fan-out) y una única goroutina agregadora
// (fan-in) los va indexando a medida que llegan y los escribe en MongoDB.
// El número de goroutines es el parámetro de control del Experimento 1
// (semana 7).
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
	"proyecto9/internal/persistencia"
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
	sinMongo := flag.Bool("sin-mongo", false, "no escribir en MongoDB (mide solo el procesamiento)")
	rutaResultado := flag.String("resultado", "", "CSV al que se agrega una fila con los tiempos de esta corrida")
	flag.Parse()

	if *goroutines < 1 || *bloquesPorGoroutine < 1 {
		log.Fatal("-goroutines y -bloques-por-goroutine deben ser >= 1")
	}

	inicio := time.Now()

	bloques, err := despachar(aqs.Archivos(*dirDatos, *anio), *goroutines**bloquesPorGoroutine)
	if err != nil {
		log.Fatalf("error calculando los bloques: %v", err)
	}
	log.Printf("%d bloques repartidos entre %d goroutines trabajadoras", len(bloques), *goroutines)

	entradaBloques := make(chan bloque)
	salida := make(chan aqs.ValorLeido, 1000)

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
		go trabajador(entradaBloques, salida, &total, &muTotal, &trabajadoras)
	}

	go func() {
		for _, b := range bloques {
			entradaBloques <- b
		}
		close(entradaBloques)
	}()

	go func() {
		trabajadoras.Wait()
		close(salida)
	}()

	// Agregadora: la goroutine principal es la única que toca el índice,
	// por eso Agregar no necesita sincronización.
	indice := aqs.NuevoIndice()
	var tiempoIndexado time.Duration
	for v := range salida {
		t := time.Now()
		indice.Agregar(v)
		tiempoIndexado += time.Since(t)
	}

	corrida := medicion.Corrida{
		Modo:          "concurrente",
		Goroutines:    *goroutines,
		Procesamiento: time.Since(inicio),
		Indexado:      tiempoIndexado,
		FilasLeidas:   total.Leidas,
		Registros:     int64(len(indice.Registros)),
		MemoriaMB:     medicion.MemoriaMB(),
	}
	log.Printf("total: %s", &total)
	log.Printf("registros unicos (estacion, hora): %d", corrida.Registros)

	if !*sinMongo {
		inicioMongo := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		escritos, err := persistencia.Reemplazar(ctx, *mongoURI, *baseDatos, indice.Registros, 5000)
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
// medida que los encuentra.
func trabajador(entrada <-chan bloque, salida chan<- aqs.ValorLeido, total *aqs.Conteo, mu *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	for b := range entrada {
		c, err := aqs.ProcesarRango(b.ruta, b.contaminante, b.inicio, b.fin, salida)
		if err != nil {
			log.Fatalf("error procesando %s[%d:%d]: %v", b.ruta, b.inicio, b.fin, err)
		}
		mu.Lock()
		total.Sumar(c)
		mu.Unlock()
	}
}
