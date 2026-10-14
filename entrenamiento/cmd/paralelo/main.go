// Entrenamiento paralelo en memoria compartida: el mismo modelo y los
// mismos datos que entrenamiento/cmd/secuencial, pero el gradiente de cada
// época lo calculan N goroutines sobre bloques disjuntos de ejemplos. Es
// el paso intermedio antes del entrenamiento distribuido por TCP (Parte
// B): misma descomposición de datos, pero con goroutines en vez de nodos.
//
// -estrategia elige cómo se combinan los gradientes parciales
// (reduccion, mutex o atomico; ver internal/logistica/paralelo.go). Es la
// base del Experimento 1b (speedup del entrenamiento) y un adelanto del
// Experimento 3 (mutex frente a atómicos).
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"proyecto9/internal/entrenador"
	"proyecto9/internal/logistica"
)

func main() {
	cfg := entrenador.RegistrarFlags()
	goroutines := flag.Int("goroutines", 4, "goroutines que calculan el gradiente en cada época")
	nombreEstrategia := flag.String("estrategia", "reduccion", "cómo se combinan los gradientes: reduccion, mutex o atomico")
	flag.Parse()

	estrategia, err := logistica.ParsearEstrategia(*nombreEstrategia)
	if err != nil {
		log.Fatal(err)
	}
	if *goroutines < 1 {
		log.Fatal("-goroutines debe ser >= 1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	datos, err := entrenador.Preparar(ctx, cfg)
	if err != nil {
		log.Fatalf("error preparando los datos: %v", err)
	}
	X, y := datos.Entrenamiento.X, datos.Entrenamiento.Y

	modelo := logistica.NuevoModelo(len(X[0]))
	inicio := time.Now()
	perdidas := modelo.EntrenarParalelo(X, y, cfg.TasaAprendizaje, cfg.Epocas, datos.PesosClase, *goroutines, estrategia)
	duracion := time.Since(inicio)

	err = entrenador.Reportar(cfg, entrenador.Corrida{
		Modo:       "paralelo_" + string(estrategia),
		Goroutines: *goroutines,
		Duracion:   duracion,
		Epocas:     cfg.Epocas,
		Perdidas:   perdidas,
		Evaluacion: entrenador.Evaluar(modelo, datos.Prueba),
	}, modelo, datos.Normalizador)
	if err != nil {
		log.Fatalf("no se pudo guardar el resultado: %v", err)
	}
}
