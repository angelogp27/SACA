#!/usr/bin/env bash
# Compila nativo/cuda/gradiente/gradiente.cu con g++ emulando CUDA en la CPU
# (Linux/macOS), para comprobar la lógica de los tres kernels (atomic,
# shared y shuffle) en una PC sin GPU NVIDIA. Un hilo del sistema por hilo
# CUDA: es lentísimo, así que se usa con pocos ejemplos y épocas y los
# tiempos NO significan nada. El resultado es bin/gradiente_cuda_emulado.
# Uso (desde la raíz del repo):
#   nativo/cuda/emulacion/compilar.sh
#   bin/gradiente_cuda_emulado --kernel shuffle --ejemplos 20000 --epocas 3
set -euo pipefail
cd "$(dirname "$0")/../../.."
mkdir -p bin
fuente=nativo/cuda/gradiente/gradiente.cu
tmp=bin/gradiente_cuda_emulado.cpp
# Las dos construcciones que no son C++: la memoria compartida dinámica y el
# lanzamiento con <<< >>> (3 o 4 argumentos de configuración).
perl -0pe '
  s/extern __shared__ double compartida\[\];/double *compartida = (double *)memoria_compartida_bloque.data();/;
  s/(\w+)<<<\s*([^,<>]+?)\s*,\s*([^,<>]+?)\s*(?:,\s*([^<>]+?)\s*)?>>>\(/"lanzar($2, $3, " . (defined $4 ? $4 : "0") . ", $1, "/ge;
' "$fuente" > "$tmp"
g++ -std=c++20 -O2 -Wall -Wextra -Wno-unknown-pragmas -D__CUDA_ARCH__=700 -I nativo/cuda/emulacion -pthread -o bin/gradiente_cuda_emulado "$tmp" -lm
echo "listo: bin/gradiente_cuda_emulado"
