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
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"proyecto9/internal/aqs"
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
	flag.Parse()

	if *goroutines < 1 {
		log.Fatal("-goroutines debe ser >= 1")
	}

	inicio := time.Now()

	archivos := map[string]string{
		"o3":  fmt.Sprintf("%s/hourly_44201_%s.csv", *dirDatos, *anio),
		"co":  fmt.Sprintf("%s/hourly_42101_%s.csv", *dirDatos, *anio),
		"so2": fmt.Sprintf("%s/hourly_42401_%s.csv", *dirDatos, *anio),
		"no2": fmt.Sprintf("%s/hourly_42602_%s.csv", *dirDatos, *anio),
	}

	bloques, err := despachar(archivos, *goroutines)
	if err != nil {
		log.Fatalf("error calculando los bloques: %v", err)
	}
	log.Printf("%d bloques repartidos entre %d goroutines trabajadoras", len(bloques), *goroutines)

	entradaBloques := make(chan bloque)
	salida := make(chan aqs.ValorLeido, 1000)
	var leidas, descartadas atomic.Int64

	var trabajadoras sync.WaitGroup
	for i := 0; i < *goroutines; i++ {
		trabajadoras.Add(1)
		go trabajador(entradaBloques, salida, &leidas, &descartadas, &trabajadoras)
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

	indice := make(map[string]*registro.RegistroProcesado)
	for v := range salida {
		aqs.Indexar(indice, v)
	}
	log.Printf("registros unicos (estacion, hora): %d", len(indice))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cliente, err := mongo.Connect(options.Client().ApplyURI(*mongoURI))
	if err != nil {
		log.Fatalf("no se pudo conectar a mongo: %v", err)
	}
	defer cliente.Disconnect(ctx)

	coleccion := cliente.Database(*baseDatos).Collection("registros_procesados")
	if err := coleccion.Drop(ctx); err != nil {
		log.Fatalf("no se pudo limpiar la coleccion: %v", err)
	}

	escritos, err := persistencia.EscribirLotes(ctx, coleccion, indice, 5000)
	if err != nil {
		log.Fatalf("error escribiendo en mongo: %v", err)
	}

	duracion := time.Since(inicio)
	log.Printf("filas leidas: %d | descartadas: %d | registros escritos: %d", leidas.Load(), descartadas.Load(), escritos)
	log.Printf("T(%d) cargador concurrente: %s", *goroutines, duracion)
}

// despachador: calcula los límites de bloque de cada archivo y arma la
// lista completa de bloques a repartir entre los trabajadores.
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
// medida que los encuentra. leidas y descartadas son atómicos porque los
// suman varias goroutines a la vez.
func trabajador(entrada <-chan bloque, salida chan<- aqs.ValorLeido, leidas, descartadas *atomic.Int64, wg *sync.WaitGroup) {
	defer wg.Done()
	for b := range entrada {
		l, d, err := aqs.ProcesarRango(b.ruta, b.contaminante, b.inicio, b.fin, salida)
		if err != nil {
			log.Fatalf("error procesando %s[%d:%d]: %v", b.ruta, b.inicio, b.fin, err)
		}
		leidas.Add(l)
		descartadas.Add(d)
	}
}
