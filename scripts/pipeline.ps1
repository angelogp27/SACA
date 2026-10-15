# Pipeline completo de SACA, reproducible de punta a punta:
#   1. descarga y verifica los CSV de la EPA (data/manifiesto.json)
#   2. levanta MongoDB con Docker Compose
#   3. carga los datos con el cargador concurrente
#   4. entrena el modelo (crea la caché de muestra y data/modelo.json)
#   5. (opcional, -Experimentos) corre los Experimentos 1b y 1
#   6. exporta los JSON reales para el frontend
# Uso (desde la raíz del repo):
#   powershell -ExecutionPolicy Bypass -File scripts/pipeline.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/pipeline.ps1 -Experimentos
#   powershell -ExecutionPolicy Bypass -File scripts/pipeline.ps1 -SinDocker   # Mongo ya corre en localhost:27017
#   powershell -ExecutionPolicy Bypass -File scripts/pipeline.ps1 -MongoLocal  # Windows sin Docker: levanta MongoDB portable
param(
    [switch]$Experimentos,
    [switch]$SinDocker,
    [switch]$MongoLocal,
    [int]$Goroutines = 8
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

function Paso($texto) { Write-Host "`n=== $texto ===" -ForegroundColor Cyan }
function Revisar($que) { if ($LASTEXITCODE -ne 0) { throw "fallo: $que" } }

Paso "1/6 Datos de la EPA"
go run ./herramientas/cmd/descargar
Revisar "descarga de datos"

if ($MongoLocal) {
    Paso "2/6 MongoDB portable (scripts/mongo-local.ps1)"
    & powershell -ExecutionPolicy Bypass -File scripts/mongo-local.ps1
    Revisar "mongo-local"
} elseif (-not $SinDocker) {
    Paso "2/6 MongoDB (docker compose)"
    docker compose up -d mongo
    Revisar "docker compose"
    $listo = $false
    for ($i = 0; $i -lt 30 -and -not $listo; $i++) {
        docker compose exec -T mongo mongosh --quiet --eval "db.runCommand({ ping: 1 }).ok" 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) { $listo = $true } else { Start-Sleep -Seconds 2 }
    }
    if (-not $listo) { throw "MongoDB no respondió" }
} else {
    Paso "2/6 MongoDB: se usa el que ya corre en localhost:27017"
}

New-Item -ItemType Directory -Force bin | Out-Null

Paso "3/6 Carga concurrente ($Goroutines goroutines y agregadoras)"
go run ./cargador/cmd/concurrente -goroutines $Goroutines -agregadoras 0
Revisar "cargador concurrente"

Paso "4/6 Entrenamiento y modelo"
# Se reutiliza data/muestra_prueba.gob si existe: los CSV son siempre los mismos
# (verificados por SHA-256), así el modelo es reproducible. Bórrala a mano
# para sortear una muestra nueva.
go run ./entrenamiento/cmd/secuencial -modelo data/modelo.json
Revisar "entrenamiento"

if ($Experimentos) {
    Paso "5/6 Experimento 1b (entrenamiento)"
    & powershell -ExecutionPolicy Bypass -File scripts/experimento1b.ps1
    Revisar "experimento 1b"
    Paso "5/6 Experimento 1 (cargador, ~1 hora)"
    & powershell -ExecutionPolicy Bypass -File scripts/experimento1.ps1
    Revisar "experimento 1"
} else {
    Paso "5/6 Experimentos omitidos (usa -Experimentos para correrlos)"
}

Paso "6/6 Exportar datos para el frontend"
go run ./herramientas/cmd/exportar
Revisar "exportar"

Write-Host "`nListo. Para ver el frontend: cd frontend; npm install; npm run dev" -ForegroundColor Green
