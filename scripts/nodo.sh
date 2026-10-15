#!/usr/bin/env bash
# Une esta PC (Linux o macOS) al clúster como nodo trabajador (ver
# docs/manual_cluster.md, paso 6):
#   scripts/nodo.sh 100.101.102.103:9000 ana
set -euo pipefail
cd "$(dirname "$0")/.."

MAESTRO=${1:?uso: scripts/nodo.sh IP_MAESTRO:PUERTO [id]}
ID=${2:-$(hostname | tr '[:upper:]' '[:lower:]')}
MOTOR=${MOTOR:-auto}

echo "== 1/4 programa del trabajador"
mkdir -p bin
go build -o bin/trabajador ./entrenamiento/cmd/trabajador
echo "   compilado bin/trabajador (commit $(git rev-parse --short HEAD))"

echo "== 2/4 muestra de datos"
[ -f data/muestra.gob ] || { echo "falta data/muestra.gob: descárgala de la carpeta compartida del grupo (manual, paso 3)"; exit 1; }
HUELLA=$( (sha256sum data/muestra.gob 2>/dev/null || shasum -a 256 data/muestra.gob) | cut -c1-12)
echo "   huella $HUELLA (debe ser igual a la que muestra el maestro)"

echo "== 3/4 motor de cálculo"
if command -v nvidia-smi >/dev/null; then
  echo "   GPU NVIDIA: $(nvidia-smi --query-gpu=name --format=csv,noheader | head -1)"
  [ -x bin/gradiente_cuda ] || echo "   AVISO: falta bin/gradiente_cuda; compílalo con nvcc (nativo/cuda/README.md) o se usará la CPU"
else
  echo "   sin GPU NVIDIA: se calcula con goroutines en la CPU"
fi

echo "== 4/4 conexión con el maestro $MAESTRO"
if timeout 3 bash -c "</dev/tcp/${MAESTRO%:*}/${MAESTRO##*:}" 2>/dev/null; then
  echo "   OK"
else
  echo "   AVISO: no se pudo conectar todavía; el trabajador seguirá reintentando"
fi

echo "== nodo $ID en marcha (Ctrl+C para salir)"
exec ./bin/trabajador -maestro "$MAESTRO" -id "$ID" -motor "$MOTOR"
