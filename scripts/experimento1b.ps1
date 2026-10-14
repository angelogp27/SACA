# Experimento 1b: tiempo de entrenamiento de la regresión logística con
# 1, 2, 4, 8 y 12 goroutines y las tres estrategias de sincronización
# (reduccion, mutex, atomico), contra el entrenamiento secuencial T(1).
# Usa la caché data/muestra.gob (se crea desde MongoDB la primera vez).
# Uso (desde la raíz del repo):
#   powershell -ExecutionPolicy Bypass -File scripts/experimento1b.ps1
param(
    [int]$Repeticiones = 3,
    [int[]]$Goroutines = @(1, 2, 4, 8, 12),
    [int]$Epocas = 200
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

New-Item -ItemType Directory -Force bin, resultados | Out-Null
go build -o bin/entrenamiento-secuencial.exe ./entrenamiento/cmd/secuencial
go build -o bin/entrenamiento-paralelo.exe ./entrenamiento/cmd/paralelo
go build -o bin/analisis.exe ./experimentos/cmd/analisis

$csv = "resultados/experimento1b.csv"
if (Test-Path $csv) { Remove-Item $csv }

for ($r = 1; $r -le $Repeticiones; $r++) {
    Write-Host "== repeticion $r/$Repeticiones: secuencial"
    & ./bin/entrenamiento-secuencial.exe -epocas $Epocas -resultado $csv 2>$null
    if ($LASTEXITCODE -ne 0) { throw "fallo el entrenamiento secuencial" }
    foreach ($e in @("reduccion", "mutex", "atomico")) {
        foreach ($n in $Goroutines) {
            Write-Host "== repeticion $r/$Repeticiones: $e con $n goroutines"
            & ./bin/entrenamiento-paralelo.exe -goroutines $n -estrategia $e -epocas $Epocas -resultado $csv 2>$null
            if ($LASTEXITCODE -ne 0) { throw "fallo el entrenamiento $e con $n goroutines" }
        }
    }
}

& ./bin/analisis.exe -entrada $csv -json frontend/public/datos/experimento1b.json -md resultados/experimento1b.md `
    -nombre "Experimento 1b" -descripcion "Entrenamiento: gradiente de la regresión logística en paralelo" `
    -paralelo "paralelo_reduccion,paralelo_mutex,paralelo_atomico" -serial ""
