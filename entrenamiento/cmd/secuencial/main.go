// Entrenamiento secuencial: línea base T(1) de la Parte B. Entrena la
// regresión logística sobre una muestra de MongoDB, prediciendo si el NO2
// de una estación cruzará el umbral AQI 51 (>53 ppb) dentro de
// horizonteHoras. Sirve para validar la implementación de la Parte B antes
// de construir el protocolo maestro-trabajador (semanas 9-12).
package main

import (
	"context"
	"flag"
	"log"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"proyecto9/internal/logistica"
	"proyecto9/internal/registro"
)

// horizonteHoras fijo en 1 por ahora; el enunciado permite un horizonte
// configurable entre 1 y 24 horas (ver docs/contrato_registro_procesado.md
// y .context/proyecto9.md §4).
const horizonteHoras = 1

// Vector de features por registro. No incluye los rezagos (t-1..t-24) ni
// las medias/máximos móviles de 24h del enunciado completo: eso exige
// recorrer la serie ordenada por estación, y queda pendiente para cuando
// se integre con el pipeline real de entrenamiento. Tampoco incluye tipo
// de zona urbana (requiere el archivo annual_conc_by_monitor, no cargado
// todavía). Aquí solo entra lo que ya vive en el registro procesado:
// concentración actual del contaminante objetivo y de los otros 3 gases,
// posición geográfica, y estacionalidad cíclica de hora/día/mes.
func vectorFeatures(r *registro.RegistroProcesado) []float64 {
	valor := func(p *float64) float64 {
		if p == nil {
			return 0
		}
		return *p
	}

	t, _ := time.Parse(time.RFC3339, r.TimestampUTC)
	hora := float64(t.Hour())
	diaSemana := float64(t.Weekday())
	mes := float64(t.Month())

	return []float64{
		valor(r.NO2Ppb),
		valor(r.O3Ppm),
		valor(r.SO2Ppb),
		valor(r.COPpm),
		r.Latitud,
		r.Longitud,
		math.Sin(2 * math.Pi * hora / 24),
		math.Cos(2 * math.Pi * hora / 24),
		math.Sin(2 * math.Pi * diaSemana / 7),
		math.Cos(2 * math.Pi * diaSemana / 7),
		math.Sin(2 * math.Pi * mes / 12),
		math.Cos(2 * math.Pi * mes / 12),
	}
}

func main() {
	mongoURI := flag.String("mongo-uri", "mongodb://localhost:27017", "cadena de conexión a MongoDB")
	baseDatos := flag.String("db", "proyecto9", "nombre de la base de datos")
	tamanoMuestra := flag.Int("muestra", 200000, "cantidad de registros sorteados de MongoDB antes de descartar los que no tienen etiqueta")
	tasaAprendizaje := flag.Float64("tasa", 0.1, "tasa de aprendizaje")
	epocas := flag.Int("epocas", 200, "numero de epocas")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	cliente, err := mongo.Connect(options.Client().ApplyURI(*mongoURI))
	if err != nil {
		log.Fatalf("no se pudo conectar a mongo: %v", err)
	}
	defer cliente.Disconnect(ctx)

	coleccion := cliente.Database(*baseDatos).Collection("registros_procesados")
	if _, err := coleccion.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "estacion_id", Value: 1}, {Key: "timestamp_utc", Value: 1}},
	}); err != nil {
		log.Fatalf("no se pudo crear el indice: %v", err)
	}

	inicioCarga := time.Now()
	X, y, err := construirMuestra(ctx, coleccion, *tamanoMuestra)
	if err != nil {
		log.Fatalf("error construyendo la muestra: %v", err)
	}
	log.Printf("muestra construida: %d ejemplos etiquetados de %d sorteados (%s)", len(X), *tamanoMuestra, time.Since(inicioCarga))

	if len(X) == 0 {
		log.Fatal("no se encontraron ejemplos etiquetables, revisa la coleccion")
	}
	normalizar(X)

	positivos := 0
	for _, v := range y {
		if v == 1 {
			positivos++
		}
	}
	log.Printf("balance de clases: %d positivos de %d (%.2f%%)", positivos, len(y), 100*float64(positivos)/float64(len(y)))

	pesosClase := logistica.CalcularPesosClase(y)
	log.Printf("pesos de clase: clase 0=%.4f, clase 1=%.2f", pesosClase.Cero, pesosClase.Uno)

	modelo := logistica.NuevoModelo(len(X[0]))

	inicioEntrenamiento := time.Now()
	perdidas := modelo.Entrenar(X, y, *tasaAprendizaje, *epocas, pesosClase)
	duracionEntrenamiento := time.Since(inicioEntrenamiento)

	paso := len(perdidas) / 10
	if paso == 0 {
		paso = 1
	}
	for e := 0; e < len(perdidas); e += paso {
		log.Printf("epoca %d: perdida=%.4f", e, perdidas[e])
	}
	log.Printf("perdida final: %.4f", perdidas[len(perdidas)-1])
	log.Printf("pesos: %v", modelo.Pesos)
	log.Printf("sesgo: %.4f", modelo.Sesgo)
	log.Printf("T(1) entrenamiento secuencial (%d epocas sobre %d ejemplos): %s", *epocas, len(X), duracionEntrenamiento)
}

// construirMuestra sortea tamanoMuestra registros con $sample y, para cada
// uno, busca por (estacion_id, timestamp_utc) la lectura de esa misma
// estación horizonteHoras más tarde. Si no existe, el ejemplo se descarta
// porque no se puede etiquetar.
func construirMuestra(ctx context.Context, coleccion *mongo.Collection, n int) ([][]float64, []float64, error) {
	cursor, err := coleccion.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$sample", Value: bson.D{{Key: "size", Value: n}}}},
	})
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var X [][]float64
	var y []float64

	for cursor.Next(ctx) {
		var actual registro.RegistroProcesado
		if err := cursor.Decode(&actual); err != nil {
			return nil, nil, err
		}
		if actual.NO2Ppb == nil {
			continue
		}

		tActual, err := time.Parse(time.RFC3339, actual.TimestampUTC)
		if err != nil {
			continue
		}
		tFutura := tActual.Add(horizonteHoras * time.Hour)

		var futuro registro.RegistroProcesado
		err = coleccion.FindOne(ctx, bson.D{
			{Key: "estacion_id", Value: actual.EstacionID},
			{Key: "timestamp_utc", Value: tFutura.Format(time.RFC3339)},
		}).Decode(&futuro)
		if err == mongo.ErrNoDocuments {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if futuro.NO2Ppb == nil {
			continue
		}

		etiqueta := 0.0
		if *futuro.NO2Ppb > 53 {
			etiqueta = 1.0
		}

		X = append(X, vectorFeatures(&actual))
		y = append(y, etiqueta)
	}

	return X, y, cursor.Err()
}

func normalizar(X [][]float64) {
	numFeatures := len(X[0])
	medias := make([]float64, numFeatures)
	for _, x := range X {
		for j, v := range x {
			medias[j] += v
		}
	}
	for j := range medias {
		medias[j] /= float64(len(X))
	}

	desviaciones := make([]float64, numFeatures)
	for _, x := range X {
		for j, v := range x {
			d := v - medias[j]
			desviaciones[j] += d * d
		}
	}
	for j := range desviaciones {
		desviaciones[j] = math.Sqrt(desviaciones[j] / float64(len(X)))
		if desviaciones[j] == 0 {
			desviaciones[j] = 1
		}
	}

	for _, x := range X {
		for j := range x {
			x[j] = (x[j] - medias[j]) / desviaciones[j]
		}
	}
}
