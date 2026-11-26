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
    echo "== 1/4 programas en Go"
    for p in maestro:./entrenamiento/cmd/maestro trabajador:./entrenamiento/cmd/trabajador \
             cargador_concurrente:./cargador/cmd/concurrente saca:./lanzador/cmd/saca exportar:./herramientas/cmd/exportar \
             saca-top:./herramientas/cmd/saca-top; do
        go build -o "bin/${p%%:*}" "${p#*:}"
        echo "   bin/${p%%:*}"
    done

    echo "== 2/4 Pthreads y OpenMP (C)"
    # -ffp-contract=off es obligatorio: sin él la muestra.bin sale con otra huella.
    gcc -O3 -ffp-contract=off -pthread -o bin/cargador_pthreads nativo/pthreads/cargador/cargador.c -lm
    echo "   bin/cargador_pthreads"
    gcc -O3 -march=native -fopenmp -o bin/red_openmp nativo/openmp/red/red_openmp.c -lm
    echo "   bin/red_openmp"

    echo "== 3/4 MPI y CUDA (MPI obligatorio, CUDA opcional)"
    MPICC=$(command -v mpicc || ls /usr/lib64/openmpi/bin/mpicc 2>/dev/null || true)
    if [ -n "$MPICC" ] && PATH="$(dirname "$MPICC"):$PATH" "$MPICC" -O2 -o bin/puente_mpi nativo/mpi/puente/puente.c; then
        echo "   bin/puente_mpi"
    else
        echo "   FALTA MPI: sudo dnf install openmpi openmpi-devel (docs/herramientas/linux-fedora.md)"
    fi
    NVCC=$(command -v nvcc || ls /usr/local/cuda/bin/nvcc 2>/dev/null || true)
    if [ -z "$NVCC" ]; then
        echo "   sin CUDA (no hay nvcc): esta laptop calcula solo con la CPU"
    else
        for f in red_cuda:red/red gradiente_cuda:gradiente_servidor; do
            exe="bin/${f%%:*}"; fuente="nativo/cuda/${f#*:}.cu"
            if [ "$exe" -nt "$fuente" ] && [ "$exe" -nt nativo/comun/motor_red.h ]; then echo "   $exe (ya estaba)"
            elif "$NVCC" -O3 -o "$exe" "$fuente"; then echo "   $exe"
            else echo "   AVISO: no se pudo compilar $exe; se sigue sin él"; fi
        done
    fi

    echo "== 4/4 página"
    if command -v npm >/dev/null; then
        (cd frontend && { [ node_modules/.package-lock.json -nt package-lock.json ] || npm ci; } && npm run build >/dev/null)  # npm ci si cambió package-lock.json
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
