#!/usr/bin/env bash
# Clúster completo en una sola PC: el maestro y N trabajadores como
# procesos separados que se hablan por TCP (localhost). Sirve para ensayar
# la demo y como plan B si la red falla durante la presentación.
# Uso (desde la raíz del repo): scripts/cluster-local.sh [nodos] [epocas]
set -euo pipefail
cd "$(dirname "$0")/.."

NODOS=${1:-3}
EPOCAS=${2:-200}
PUERTO=${PUERTO:-9000}
MOTOR=${MOTOR:-auto}

[ -f data/muestra.gob ] || { echo "falta data/muestra.gob (ver docs/manual_cluster.md, paso 3)"; exit 1; }
mkdir -p bin/logs
go build -o bin/maestro ./entrenamiento/cmd/maestro
go build -o bin/trabajador ./entrenamiento/cmd/trabajador

CPUS=$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)
HILOS=${HILOS:-$(( CPUS / NODOS > 0 ? CPUS / NODOS : 1 ))}

pids=()
for i in $(seq 1 "$NODOS"); do
  ./bin/trabajador -maestro "localhost:$PUERTO" -id "local$i" -hilos "$HILOS" -motor "$MOTOR" \
    > "bin/logs/trabajador-local$i.log" 2>&1 &
  pids+=($!)
done
trap 'kill "${pids[@]}" 2>/dev/null || true' EXIT

echo "== maestro con $NODOS nodos de $HILOS goroutines cada uno"
./bin/maestro -nodos "$NODOS" -escuchar ":$PUERTO" -epocas "$EPOCAS" -modo tcp_local -maquina "$(hostname) local"

sleep 0.5
echo
echo "== últimas líneas de cada trabajador (completas en bin/logs/)"
for i in $(seq 1 "$NODOS"); do
  echo "-- local$i"
  tail -n 3 "bin/logs/trabajador-local$i.log"
done
