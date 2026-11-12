#!/usr/bin/env bash
# Compila todo lo que necesita SACA en esta laptop (Linux) y abre la página.
# Es lo único que corre cada integrante: en la página elige "Iniciar
# clúster" (una sola laptop) o "Unirse al clúster" (las demás).
#   bash scripts/saca.sh            # compila y abre
#   bash scripts/saca.sh --sin-compilar
set -euo pipefail
cd "$(dirname "$0")/.."

if [ "${1:-}" != "--sin-compilar" ]; then
    mkdir -p bin
    echo "== 1/3 programas en Go"
    for p in maestro:./entrenamiento/cmd/maestro trabajador:./entrenamiento/cmd/trabajador \
             cargador_concurrente:./cargador/cmd/concurrente saca:./lanzador/cmd/saca exportar:./herramientas/cmd/exportar; do
        go build -o "bin/${p%%:*}" "${p#*:}"
        echo "   bin/${p%%:*}"
    done

    echo "== 2/3 cargador de Pthreads (C)"
    # -ffp-contract=off es obligatorio: sin él la muestra.bin sale con otra huella.
    gcc -O3 -ffp-contract=off -pthread -o bin/cargador_pthreads nativo/pthreads/cargador/cargador.c -lm
    echo "   bin/cargador_pthreads"

    echo "== 3/3 página"
    if command -v npm >/dev/null; then
        (cd frontend && { [ -d node_modules ] || npm ci; } && npm run build >/dev/null)
        echo "   frontend/dist"
    elif [ -f frontend/dist/index.html ]; then
        echo "   npm no está instalado; se usa el frontend/dist que ya existe"
    else
        echo "   FALTA: instala Node.js (npm) para compilar la página"; exit 1
    fi
fi

for f in data/raw/hourly_44201_2025.csv data/raw/hourly_42602_2025.csv data/raw/hourly_42401_2025.csv data/raw/hourly_42101_2025.csv; do
    [ -f "$f" ] || echo "AVISO: falta $f (sin los 4 CSV esta laptop no puede limpiar)"
done
exec ./bin/saca
