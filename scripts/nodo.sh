#!/usr/bin/env bash
# Une esta PC (Linux o macOS) al clúster como nodo trabajador (ver
# docs/cluster/guia_nodo_sin_nvidia.md o guia_nodo_con_nvidia.md):
#   bash scripts/nodo.sh 100.101.102.103:9000 ana [muestra]
# El tercer argumento (opcional) permite usar otra muestra, p. ej.
# data/muestra_prueba.gob en una PC con poca RAM; debe ser la del maestro.
set -euo pipefail
cd "$(dirname "$0")/.."

MAESTRO=${1:?uso: scripts/nodo.sh IP_MAESTRO:PUERTO [id] [muestra]}
ID=${2:-$(hostname | tr '[:upper:]' '[:lower:]')}
MUESTRA=${3:-data/muestra.gob}
MOTOR=${MOTOR:-auto}

echo "== 1/4 programa del trabajador"
mkdir -p bin
go build -o bin/trabajador ./entrenamiento/cmd/trabajador
echo "   compilado bin/trabajador (commit $(git rev-parse --short HEAD))"

echo "== 2/4 muestra de datos"
[ -f "$MUESTRA" ] || { echo "falta $MUESTRA: descárgala de la carpeta compartida del grupo (ver la guía del nodo, paso 'Copiar la muestra')"; exit 1; }
HUELLA=$( (sha256sum "$MUESTRA" 2>/dev/null || shasum -a 256 "$MUESTRA") | cut -c1-12)
echo "   $MUESTRA, huella $HUELLA (debe ser igual a la que muestra el maestro)"

echo "== 3/4 motor de cálculo"
if command -v nvidia-smi >/dev/null; then
  echo "   GPU NVIDIA: $(nvidia-smi --query-gpu=name --format=csv,noheader | head -1)"
  [ -x bin/gradiente_cuda ] || echo "   AVISO: falta bin/gradiente_cuda; compílalo con nvcc (nativo/cuda/README.md) o se usará la CPU"
else
  echo "   sin GPU NVIDIA: se calcula con goroutines en la CPU"
fi

echo "== 4/4 conexión con el maestro $MAESTRO"
HOST_MAESTRO=${MAESTRO%:*}
PUERTO_MAESTRO=${MAESTRO##*:}
# macOS no trae "timeout": se prueba primero con nc, que existe en macOS y
# en casi todos los Linux.
if command -v nc >/dev/null 2>&1; then
  nc -z -w 3 "$HOST_MAESTRO" "$PUERTO_MAESTRO" >/dev/null 2>&1 && CONECTA=si || CONECTA=no
elif command -v timeout >/dev/null 2>&1; then
  timeout 3 bash -c "</dev/tcp/$HOST_MAESTRO/$PUERTO_MAESTRO" 2>/dev/null && CONECTA=si || CONECTA=no
else
  CONECTA=sin_comprobar
fi
case $CONECTA in
  si) echo "   OK" ;;
  no) echo "   AVISO: no se pudo conectar todavía (¿el maestro ya arrancó?, ¿Tailscale conectado?); el trabajador seguirá reintentando" ;;
  *)  echo "   no se pudo comprobar (falta nc); el trabajador lo intentará igual" ;;
esac

echo "== nodo $ID en marcha (Ctrl+C para salir)"
exec ./bin/trabajador -maestro "$MAESTRO" -id "$ID" -motor "$MOTOR" -muestra "$MUESTRA"
