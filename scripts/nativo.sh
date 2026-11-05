#!/usr/bin/env sh
# Núcleos nativos en C (nativo/): compilarlos y correr sus experimentos.
#
#   sh scripts/nativo.sh compilar   gcc (OpenMP, Pthreads), mpicc y nvcc si están instalados
#   sh scripts/nativo.sh muestra    genera data/muestra.bin y la referencia de Go
#   sh scripts/nativo.sh exp5       Pthreads frente al cargador de Go (sin MongoDB)
#   sh scripts/nativo.sh exp6       OpenMP: static, dynamic y guided con 1..16 hilos
#   sh scripts/nativo.sh exp7       MPI: Allreduce frente a Send/Recv con 1..8 procesos
#   sh scripts/nativo.sh exp8       CUDA (atomic, shared, shuffle) frente a OpenMP, de 10^3 a 10^7 ejemplos
#
# Variables: MAQUINA (hostname; nombre del equipo en exp8), EPOCAS (200), REPETICIONES (3), HILOS ("1 2 4 8 12 16"), PROCESOS ("1 2 4 8"),
# EJEMPLOS ("1000 10000 100000 1000000 10000000"), MPIEXEC (mpiexec),
# MPIEXEC_OPCIONES (p. ej. --oversubscribe si hay más procesos que núcleos).
# Cada corrida agrega una fila a resultados/experimentoN.csv con la columna
# "maquina" (CPU/GPU), para no mezclar mediciones de PCs distintas.
set -eu
cd "$(dirname "$0")/.."

REPETICIONES=${REPETICIONES:-3}
EPOCAS=${EPOCAS:-200}
HILOS=${HILOS:-"1 2 4 8 12 16"}
PROCESOS=${PROCESOS:-"1 2 4 8"}
EJEMPLOS=${EJEMPLOS:-"1000 10000 100000 1000000 10000000"}
MPIEXEC=${MPIEXEC:-mpiexec}
MPIEXEC_OPCIONES=${MPIEXEC_OPCIONES:-}
mkdir -p bin resultados

compilar() {
  gcc -O3 -Wall -Wextra -fopenmp -o bin/entrenamiento_openmp nativo/openmp/entrenamiento/entrenamiento.c -lm
  echo "ok: bin/entrenamiento_openmp"
  gcc -O3 -Wall -Wextra -pthread -o bin/cargador_pthreads nativo/pthreads/cargador/cargador.c -lm
  echo "ok: bin/cargador_pthreads"
  if command -v mpicc >/dev/null 2>&1; then
    mpicc -O3 -Wall -Wextra -o bin/entrenamiento_mpi nativo/mpi/entrenamiento/entrenamiento.c -lm
    echo "ok: bin/entrenamiento_mpi"
  else
    echo "aviso: no hay mpicc (instala openmpi-devel o libopenmpi-dev); se omite MPI"
  fi
  if command -v nvcc >/dev/null 2>&1; then
    nvcc -O3 -o bin/gradiente_cuda_exp nativo/cuda/gradiente/gradiente.cu
    nvcc -O3 -o bin/gradiente_cuda nativo/cuda/gradiente_servidor.cu
    echo "ok: bin/gradiente_cuda_exp y bin/gradiente_cuda"
  else
    echo "aviso: no hay nvcc; se omite CUDA (compílalo en una PC con GPU NVIDIA, ver nativo/cuda/README.md)"
  fi
}

muestra() {
  go run ./herramientas/cmd/exportar-muestra
}

necesita() {
  [ -x "$1" ] || { echo "falta $1: corre primero 'sh scripts/nativo.sh compilar'" >&2; exit 1; }
}

repetir() { # repetir "comando con {r}" para r = 1..REPETICIONES
  r=1
  while [ "$r" -le "$REPETICIONES" ]; do
    echo "== repetición $r/$REPETICIONES"
    "$@"
    r=$((r + 1))
  done
}

exp5() {
  necesita bin/cargador_pthreads
  go build -o bin/cargador-secuencial ./cargador/cmd/secuencial
  go build -o bin/cargador-concurrente ./cargador/cmd/concurrente
  go build -o bin/analisis ./experimentos/cmd/analisis
  CSV=resultados/experimento5.csv
  rm -f "$CSV"
  una() {
    ./bin/cargador-secuencial -sin-mongo -resultado "$CSV"
    for n in $HILOS; do
      ./bin/cargador-concurrente -sin-mongo -goroutines "$n" -agregadoras 0 -resultado "$CSV"
      ./bin/cargador_pthreads --hilos "$n" --agregadoras 0 --resultado "$CSV"
    done
  }
  repetir una
  ./bin/analisis -entrada "$CSV" -json frontend/public/datos/experimento5.json -md resultados/experimento5.md \
    -nombre "Experimento 5" -descripcion "Cargador: Pthreads (C) frente a goroutines (Go), sin MongoDB" \
    -base secuencial -paralelo "pthreads_particionado,concurrente_particionado" -serial "t_indexado_s"
}

exp6() {
  necesita bin/entrenamiento_openmp
  go build -o bin/analisis ./experimentos/cmd/analisis
  CSV=resultados/experimento6.csv
  rm -f "$CSV"
  una() {
    ./bin/entrenamiento_openmp --secuencial --epocas "$EPOCAS" --resultado "$CSV"
    for n in $HILOS; do
      for s in static dynamic guided; do
        ./bin/entrenamiento_openmp --hilos "$n" --schedule "$s" --epocas "$EPOCAS" --resultado "$CSV"
      done
    done
  }
  repetir una
  ./bin/analisis -entrada "$CSV" -json frontend/public/datos/experimento6.json -md resultados/experimento6.md \
    -nombre "Experimento 6" -descripcion "Entrenamiento con OpenMP: reducción del gradiente con schedule static, dynamic y guided" \
    -base secuencial_c -paralelo "openmp_static,openmp_dynamic,openmp_guided" -serial ""
}

exp7() {
  necesita bin/entrenamiento_mpi
  necesita bin/entrenamiento_openmp
  go build -o bin/analisis ./experimentos/cmd/analisis
  CSV=resultados/experimento7.csv
  rm -f "$CSV"
  una() {
    ./bin/entrenamiento_openmp --secuencial --epocas "$EPOCAS" --resultado "$CSV"
    for p in $PROCESOS; do
      for modo in colectivo p2p; do
        # shellcheck disable=SC2086
        $MPIEXEC $MPIEXEC_OPCIONES -n "$p" ./bin/entrenamiento_mpi --modo "$modo" --epocas "$EPOCAS" --resultado "$CSV"
      done
    done
  }
  repetir una
  ./bin/analisis -entrada "$CSV" -json frontend/public/datos/experimento7.json -md resultados/experimento7.md \
    -nombre "Experimento 7" -descripcion "Entrenamiento con MPI: MPI_Allreduce frente a Send/Recv + Bcast" \
    -base secuencial_c -paralelo "mpi_colectivo,mpi_p2p" -serial ""
}

exp8() {
  necesita bin/entrenamiento_openmp
  necesita bin/gradiente_cuda_exp
  go build -o bin/resumen-gpu ./experimentos/cmd/gpu
  CSV=resultados/experimento8.csv
  # El nombre del equipo agrupa las filas de OpenMP (CPU) y de CUDA (GPU) de la
  # misma PC: usa uno corto y distinto en cada laptop (MAQUINA=angelo).
  MAQUINA=${MAQUINA:-$(hostname)}
  rm -f "$CSV"
  una() {
    for n in $EJEMPLOS; do
      ./bin/entrenamiento_openmp --secuencial --ejemplos "$n" --epocas "$EPOCAS" --maquina "$MAQUINA" --resultado "$CSV"
      ./bin/entrenamiento_openmp --schedule static --ejemplos "$n" --epocas "$EPOCAS" --maquina "$MAQUINA" --resultado "$CSV"
      for k in atomic shared shuffle; do
        ./bin/gradiente_cuda_exp --kernel "$k" --hilos 256 --ejemplos "$n" --epocas "$EPOCAS" --maquina "$MAQUINA" --resultado "$CSV"
      done
    done
  }
  repetir una
  ./bin/resumen-gpu -entrada "$CSV" -json frontend/public/datos/experimento8.json
}

caso=${1:-}
case "$caso" in
  compilar) compilar ;;
  muestra) muestra ;;
  exp5) exp5 ;;
  exp6) exp6 ;;
  exp7) exp7 ;;
  exp8) exp8 ;;
  *) sed -n '2,13p' "$0"; exit 1 ;;
esac
