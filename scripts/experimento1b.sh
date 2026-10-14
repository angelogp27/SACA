#!/usr/bin/env sh
# Experimento 1b: tiempo de entrenamiento de la regresión logística con
# 1, 2, 4, 8 y 12 goroutines y las tres estrategias de sincronización
# (reduccion, mutex, atomico), contra el entrenamiento secuencial T(1).
# Usa la caché data/muestra.gob (se crea desde MongoDB la primera vez).
# Uso: sh scripts/experimento1b.sh   (REPETICIONES, GOROUTINES, EPOCAS opcionales)
set -eu
cd "$(dirname "$0")/.."

REPETICIONES=${REPETICIONES:-3}
GOROUTINES=${GOROUTINES:-"1 2 4 8 12"}
EPOCAS=${EPOCAS:-200}

mkdir -p bin resultados
go build -o bin/entrenamiento-secuencial ./entrenamiento/cmd/secuencial
go build -o bin/entrenamiento-paralelo ./entrenamiento/cmd/paralelo
go build -o bin/analisis ./experimentos/cmd/analisis

CSV=resultados/experimento1b.csv
rm -f "$CSV"

r=1
while [ "$r" -le "$REPETICIONES" ]; do
  echo "== repeticion $r/$REPETICIONES: secuencial"
  ./bin/entrenamiento-secuencial -epocas "$EPOCAS" -resultado "$CSV" 2>/dev/null
  for e in reduccion mutex atomico; do
    for n in $GOROUTINES; do
      echo "== repeticion $r/$REPETICIONES: $e con $n goroutines"
      ./bin/entrenamiento-paralelo -goroutines "$n" -estrategia "$e" -epocas "$EPOCAS" -resultado "$CSV" 2>/dev/null
    done
  done
  r=$((r + 1))
done

./bin/analisis -entrada "$CSV" -json frontend/public/datos/experimento1b.json -md resultados/experimento1b.md \
  -nombre "Experimento 1b" -descripcion "Entrenamiento: gradiente de la regresión logística en paralelo" \
  -paralelo "paralelo_reduccion,paralelo_mutex,paralelo_atomico" -serial ""
