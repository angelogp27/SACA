// Cargador secuencial: línea base T(1) de la Parte A. Lee los 4 CSV
// horarios de la EPA con una sola goroutine, limpia y une por
// (estacion_id, timestamp_utc), y escribe el resultado en MongoDB.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"proyecto9/internal/aqs"
	"proyecto9/internal/medicion"
	"proyecto9/internal/persistencia"
)

func main() {
	dirDatos := flag.String("datos", "data/raw", "directorio con los 4 CSV crudos de la EPA")
	anio := flag.String("anio", "2025", "año de los archivos hourly_<codigo>_<anio>.csv")
	mongoURI := flag.String("mongo-uri", "mongodb://localhost:27017", "cadena de conexión a MongoDB")
	baseDatos := flag.String("db", "proyecto9", "nombre de la base de datos")
	sinMongo := flag.Bool("sin-mongo", false, "no escribir en MongoDB (mide solo el procesamiento)")
	rutaResultado := flag.String("resultado", "", "CSV al que se agrega una fila con los tiempos de esta corrida")
	flag.Parse()

	inicio := time.Now()

	indice := aqs.NuevoIndice()
	var total aqs.Conteo
	var tiempoIndexado time.Duration

	for contaminante, ruta := range aqs.Archivos(*dirDatos, *anio) {
		c, t, err := procesarArchivo(ruta, contaminante, indice)
		if err != nil {
			log.Fatalf("error procesando %s: %v", ruta, err)
		}
		total.Sumar(c)
		tiempoIndexado += t
	}
	corrida := medicion.Corrida{
		Modo:          "secuencial",
		Goroutines:    1,
		Procesamiento: time.Since(inicio),
		Indexado:      tiempoIndexado,
		FilasLeidas:   total.Leidas,
		Registros:     int64(len(indice.Registros)),
		MemoriaMB:     medicion.MemoriaMB(),
	}
	log.Printf("total: %s", &total)
	log.Printf("registros unicos (estacion, hora): %d (%d valores de monitores repetidos resueltos por POC)", corrida.Registros, indice.Duplicados)

	if !*sinMongo {
		inicioMongo := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		escritos, err := persistencia.Reemplazar(ctx, *mongoURI, *baseDatos, 5000, indice.Registros)
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

func procesarArchivo(ruta, contaminante string, indice *aqs.Indice) (aqs.Conteo, time.Duration, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return aqs.Conteo{}, 0, fmt.Errorf("abriendo archivo: %w", err)
	}
	defer f.Close()

	c, t, err := aqs.Procesar(f, contaminante, indice)
	if err != nil {
		return c, t, err
	}
	log.Printf("%s: %s", ruta, &c)
	return c, t, nil
}
