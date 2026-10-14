// Entrenamiento secuencial: línea base T(1) de la Parte B. Entrena la
// regresión logística sobre una muestra de MongoDB, prediciendo si el NO2
// de una estación cruzará el umbral AQI 51 (>53 ppb) dentro de
// HorizonteHoras. Entrena con los primeros meses y evalúa con los últimos
// (partición temporal). Sirve para validar la implementación de la Parte B
// antes de construir el protocolo maestro-trabajador (semanas 9-12).
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
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	datos, err := entrenador.Preparar(ctx, cfg)
	if err != nil {
		log.Fatalf("error preparando los datos: %v", err)
	}
	X, y := datos.Entrenamiento.X, datos.Entrenamiento.Y

	modelo := logistica.NuevoModelo(len(X[0]))
	inicio := time.Now()
	perdidas := modelo.Entrenar(X, y, cfg.TasaAprendizaje, cfg.Epocas, datos.PesosClase)
	duracion := time.Since(inicio)

	err = entrenador.Reportar(cfg, entrenador.Corrida{
		Modo:       "secuencial",
		Goroutines: 1,
		Duracion:   duracion,
		Epocas:     cfg.Epocas,
		Perdidas:   perdidas,
		Evaluacion: entrenador.Evaluar(modelo, datos.Prueba),
	}, modelo)
	if err != nil {
		log.Fatalf("no se pudo guardar el resultado: %v", err)
	}
}
