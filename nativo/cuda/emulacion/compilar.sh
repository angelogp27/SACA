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
# Sin argumentos, el Experimento 8; con "red", el motor de la red neuronal
# (bin/red_cuda_emulado, con 4 hilos por bloque).
if [ "${1:-}" = red ]; then
    fuente=nativo/cuda/red/red.cu
    salida=bin/red_cuda_emulado
    extra="-DHILOS=4 -I nativo/cuda/red -w"
else
    fuente=nativo/cuda/gradiente/gradiente.cu
    salida=bin/gradiente_cuda_emulado
    extra=""
fi
tmp=$salida.cpp
# Las dos construcciones que no son C++: la memoria compartida dinámica y el
# lanzamiento con <<< >>> (3 o 4 argumentos de configuración).
perl -0pe '
  s/extern __shared__ double compartida\[\];/double *compartida = (double *)memoria_compartida_bloque.data();/;
  s/(\w+)<<<\s*([^,<>]+?)\s*,\s*([^,<>]+?)\s*(?:,\s*([^<>]+?)\s*)?>>>\(/"lanzar($2, $3, " . (defined $4 ? $4 : "0") . ", $1, "/ge;
  s#"../../comun/#"../nativo/comun/#g;
' "$fuente" > "$tmp"
g++ -std=c++20 -O2 -Wall -Wextra -Wno-unknown-pragmas -D__CUDA_ARCH__=700 -I nativo/cuda/emulacion -pthread $extra -o "$salida" "$tmp" -lm
echo "listo: $salida"
