# Genera la tabla del Experimento 2 a partir de las corridas guardadas por
# el maestro (resultados\experimento2.csv): una serie por combinación de
# nodos, contra el T(1) secuencial (al menos una corrida desde la página con
# "Verificar contra el secuencial"). Escribe resultados\experimento2.md y el
# JSON que lee la página.
# Uso (desde la raíz del repo):
#   powershell -ExecutionPolicy Bypass -File scripts/experimento2-tabla.ps1
param([string]$Nombre = "experimento2")
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
go run ./experimentos/cmd/analisis -entrada "resultados/$Nombre.csv" -json "frontend/public/datos/$Nombre.json" -md "resultados/$Nombre.md" `
    -nombre "Experimento 2" -descripcion "Entrenamiento distribuido por TCP entre las PCs del grupo, por combinación de nodos" `
    -paralelo "*" "-serial="
if ($LASTEXITCODE -ne 0) { throw "no se pudo generar la tabla" }
