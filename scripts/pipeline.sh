#!/usr/bin/env sh
# Pipeline completo de SACA, reproducible de punta a punta:
#   1. descarga y verifica los CSV de la EPA (data/manifiesto.json)
#   2. levanta MongoDB con Docker Compose
#   3. carga los datos con el cargador concurrente
#   4. entrena el modelo (crea la caché de muestra y data/modelo.json)
#   5. (opcional, EXPERIMENTOS=1) corre los Experimentos 1b y 1
#   6. exporta los JSON reales para el frontend
# Uso: sh scripts/pipeline.sh
#      EXPERIMENTOS=1 sh scripts/pipeline.sh
#      SIN_DOCKER=1 sh scripts/pipeline.sh     # Mongo ya corre en localhost:27017
set -eu
cd "$(dirname "$0")/.."
GOROUTINES=${GOROUTINES:-8}

paso() { printf '\n=== %s ===\n' "$1"; }

paso "1/6 Datos de la EPA"
go run ./herramientas/cmd/descargar

if [ -z "${SIN_DOCKER:-}" ]; then
  paso "2/6 MongoDB (docker compose)"
  docker compose up -d mongo
  i=0
  until docker compose exec -T mongo mongosh --quiet --eval "db.runCommand({ ping: 1 }).ok" >/dev/null 2>&1; do
    i=$((i + 1))
    [ "$i" -ge 30 ] && { echo "MongoDB no respondió" >&2; exit 1; }
    sleep 2
  done
else
  paso "2/6 MongoDB: se usa el que ya corre en localhost:27017"
fi

paso "3/6 Carga concurrente ($GOROUTINES goroutines y agregadoras)"
go run ./cargador/cmd/concurrente -goroutines "$GOROUTINES" -agregadoras 0

paso "4/6 Entrenamiento y modelo"
# Se reutiliza data/muestra.gob si existe (los CSV son siempre los mismos,
# verificados por SHA-256): el modelo es reproducible. Bórrala a mano para
# sortear una muestra nueva.
go run ./entrenamiento/cmd/secuencial -modelo data/modelo.json

if [ -n "${EXPERIMENTOS:-}" ]; then
  paso "5/6 Experimento 1b (entrenamiento)"
  sh scripts/experimento1b.sh
  paso "5/6 Experimento 1 (cargador, ~1 hora)"
  sh scripts/experimento1.sh
else
  paso "5/6 Experimentos omitidos (usa EXPERIMENTOS=1 para correrlos)"
fi

paso "6/6 Exportar datos para el frontend"
go run ./herramientas/cmd/exportar

printf '\nListo. Para ver el frontend: cd frontend && npm install && npm run dev\n'
