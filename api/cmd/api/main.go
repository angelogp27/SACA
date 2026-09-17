// Esqueleto de la Parte C: por ahora solo expone /health, según
// docs/contrato_api_rest.md. El resto de rutas (predicciones,
// entrenamientos, WebSocket de telemetría) se agregan desde la semana 9.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
)

func main() {
	puerto := flag.String("puerto", "8080", "puerto en el que escucha la API")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", manejarHealth)

	log.Printf("API escuchando en :%s", *puerto)
	if err := http.ListenAndServe(":"+*puerto, mux); err != nil {
		log.Fatalf("error del servidor: %v", err)
	}
}

func manejarHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
