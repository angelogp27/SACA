#!/usr/bin/env sh
# Experimento 1 (semana 7): tiempo de ingesta del cargador con 1, 2, 4, 8,
# 12 y 16 goroutines contra la línea base secuencial T(1).
# Uso (desde cualquier carpeta):
#   sh scripts/experimento1.sh            # 3 repeticiones, con MongoDB
#   REPETICIONES=5 SIN_MONGO=1 sh scripts/experimento1.sh
set -eu
cd "$(dirname "$0")/.."

REPETICIONES=${REPETICIONES:-3}
GOROUTINES=${GOROUTINES:-"1 2 4 8 12 16"}

mkdir -p bin resultados
go build -o bin/cargador-secuencial ./cargador/cmd/secuencial
go build -o bin/cargador-concurrente ./cargador/cmd/concurrente
go build -o bin/analisis ./experimentos/cmd/analisis

CSV=resultados/experimento1.csv
rm -f "$CSV"
EXTRA="-resultado $CSV"
SERIAL="t_indexado_s,t_mongo_s"
if [ -n "${SIN_MONGO:-}" ]; then
  EXTRA="$EXTRA -sin-mongo"
  SERIAL="t_indexado_s"
fi

r=1
while [ "$r" -le "$REPETICIONES" ]; do
  echo "== repeticion $r/$REPETICIONES: secuencial"
  ./bin/cargador-secuencial $EXTRA
  for n in $GOROUTINES; do
    echo "== repeticion $r/$REPETICIONES: concurrente con $n goroutines"
    ./bin/cargador-concurrente -goroutines "$n" $EXTRA
    echo "== repeticion $r/$REPETICIONES: concurrente particionado con $n goroutines y $n agregadoras"
    ./bin/cargador-concurrente -goroutines "$n" -agregadoras 0 $EXTRA
  done
  r=$((r + 1))
done

./bin/analisis -entrada "$CSV" -serial "$SERIAL" -paralelo "concurrente,concurrente_particionado"
