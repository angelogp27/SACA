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

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"proyecto9/internal/aqs"
	"proyecto9/internal/persistencia"
	"proyecto9/internal/registro"
)

func main() {
	dirDatos := flag.String("datos", "data/raw", "directorio con los 4 CSV crudos de la EPA")
	anio := flag.String("anio", "2025", "año de los archivos hourly_<codigo>_<anio>.csv")
	mongoURI := flag.String("mongo-uri", "mongodb://localhost:27017", "cadena de conexión a MongoDB")
	baseDatos := flag.String("db", "proyecto9", "nombre de la base de datos")
	flag.Parse()

	inicio := time.Now()

	archivos := map[string]string{
		"o3":  fmt.Sprintf("%s/hourly_44201_%s.csv", *dirDatos, *anio),
		"co":  fmt.Sprintf("%s/hourly_42101_%s.csv", *dirDatos, *anio),
		"so2": fmt.Sprintf("%s/hourly_42401_%s.csv", *dirDatos, *anio),
		"no2": fmt.Sprintf("%s/hourly_42602_%s.csv", *dirDatos, *anio),
	}

	indice := make(map[string]*registro.RegistroProcesado)
	var leidas, descartadas int64

	for contaminante, ruta := range archivos {
		if err := procesarArchivo(ruta, contaminante, indice, &leidas, &descartadas); err != nil {
			log.Fatalf("error procesando %s: %v", ruta, err)
		}
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
	log.Printf("filas leidas: %d | descartadas: %d | registros escritos: %d", leidas, descartadas, escritos)
	log.Printf("T(1) cargador secuencial: %s", duracion)
}

func procesarArchivo(ruta, contaminante string, indice map[string]*registro.RegistroProcesado, leidas, descartadas *int64) error {
	f, err := os.Open(ruta)
	if err != nil {
		return fmt.Errorf("abriendo archivo: %w", err)
	}
	defer f.Close()

	l, d, err := aqs.Procesar(f, contaminante, indice)
	if err != nil {
		return err
	}
	*leidas += l
	*descartadas += d
	log.Printf("%s: %d filas leidas, %d descartadas", ruta, l, d)
	return nil
}
