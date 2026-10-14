# Experimento 1 (semana 7): tiempo de ingesta del cargador con 1, 2, 4, 8,
# 12 y 16 goroutines contra la línea base secuencial T(1).
# Uso (desde la raíz del repo):
#   powershell -ExecutionPolicy Bypass -File scripts/experimento1.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/experimento1.ps1 -Repeticiones 5 -SinMongo
param(
    [int]$Repeticiones = 3,
    [int[]]$Goroutines = @(1, 2, 4, 8, 12, 16),
    [switch]$SinMongo
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

New-Item -ItemType Directory -Force bin, resultados | Out-Null
go build -o bin/cargador-secuencial.exe ./cargador/cmd/secuencial
go build -o bin/cargador-concurrente.exe ./cargador/cmd/concurrente
go build -o bin/analisis.exe ./experimentos/cmd/analisis

$csv = "resultados/experimento1.csv"
if (Test-Path $csv) { Remove-Item $csv }
$extra = @("-resultado", $csv)
if ($SinMongo) { $extra += "-sin-mongo" }

for ($r = 1; $r -le $Repeticiones; $r++) {
    Write-Host "== repeticion $r/$Repeticiones: secuencial"
    & ./bin/cargador-secuencial.exe @extra
    if ($LASTEXITCODE -ne 0) { throw "fallo el cargador secuencial" }
    foreach ($n in $Goroutines) {
        Write-Host "== repeticion $r/$Repeticiones: concurrente con $n goroutines"
        & ./bin/cargador-concurrente.exe -goroutines $n @extra
        if ($LASTEXITCODE -ne 0) { throw "fallo el cargador concurrente con $n goroutines" }
        Write-Host "== repeticion $r/$Repeticiones: concurrente particionado con $n goroutines y $n agregadoras"
        & ./bin/cargador-concurrente.exe -goroutines $n -agregadoras 0 @extra
        if ($LASTEXITCODE -ne 0) { throw "fallo el cargador particionado con $n goroutines" }
    }
}

$serial = if ($SinMongo) { "t_indexado_s" } else { "t_indexado_s,t_mongo_s" }
& ./bin/analisis.exe -entrada $csv -serial $serial -paralelo "concurrente,concurrente_particionado"
