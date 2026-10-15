# Clúster completo en una sola PC: el maestro y N trabajadores como
# procesos separados que se hablan por TCP (localhost). Sirve para ensayar
# la demo y como plan B si la red falla durante la presentación.
# Uso (desde la raíz del repo):
#   powershell -ExecutionPolicy Bypass -File scripts/cluster-local.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/cluster-local.ps1 -Nodos 4 -Ventanas
# -Ventanas abre cada trabajador en su propia ventana (más vistoso para la
# demo); sin él, la salida de cada trabajador va a bin/logs/.
# -Web arranca el maestro en modo servidor (API en 127.0.0.1:8080) en vez
# de hacer una corrida: los trabajadores quedan en la sala y las corridas
# se lanzan desde la página (cd frontend; npm run dev). Ctrl+C para salir.
#   powershell -ExecutionPolicy Bypass -File scripts/cluster-local.ps1 -Nodos 2 -Web -Muestra data/muestra_prueba.gob
param(
    [int]$Nodos = 3,
    [int]$Epocas = 200,
    [int]$Puerto = 9000,
    [int]$Hilos = 0,
    [string]$Motor = "auto",
    [string]$Muestra = "data/muestra.gob",
    [switch]$Ventanas,
    [switch]$Web
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
. (Join-Path $PSScriptRoot "memoria.ps1")

if (-not (Test-Path $Muestra)) { throw "falta $Muestra (ver docs/cluster/, paso 'Copiar la muestra' de tu guía)" }
if ($Nodos -lt 1 -or $Nodos -gt 32) { throw "-Nodos debe estar entre 1 y 32" }
Assert-MemoriaSuficiente -Muestra $Muestra -Procesos ($Nodos + 1)
New-Item -ItemType Directory -Force bin, bin/logs | Out-Null
go build -o bin/maestro.exe ./entrenamiento/cmd/maestro
go build -o bin/trabajador.exe ./entrenamiento/cmd/trabajador
if ($LASTEXITCODE -ne 0) { throw "no se pudo compilar" }

# Si no se indica, se reparten los núcleos lógicos entre los trabajadores.
if ($Hilos -le 0) { $Hilos = [Math]::Max(1, [int]([Environment]::ProcessorCount / $Nodos)) }

$trabajadores = @()
for ($i = 1; $i -le $Nodos; $i++) {
    $argumentos = "-maestro localhost:$Puerto -id local$i -hilos $Hilos -motor $Motor -muestra $Muestra -reconectar"
    if ($Ventanas) {
        $trabajadores += Start-Process powershell -PassThru -ArgumentList "-NoExit", "-Command", "`$host.UI.RawUI.WindowTitle='trabajador local$i'; .\bin\trabajador.exe $argumentos"
    } else {
        $trabajadores += Start-Process .\bin\trabajador.exe -PassThru -NoNewWindow -ArgumentList $argumentos `
            -RedirectStandardError "bin/logs/trabajador-local$i.log" -RedirectStandardOutput "bin/logs/trabajador-local$i.out"
    }
}

if ($Web) {
    Write-Host "== maestro en modo servidor con $Nodos nodos locales; abre la página (cd frontend; npm run dev). Ctrl+C para terminar."
    & ./bin/maestro.exe -web 127.0.0.1:8080 -capacidad 16 -escuchar ":$Puerto" -epocas $Epocas -muestra $Muestra -modo tcp_local
    $codigo = $LASTEXITCODE
    foreach ($p in $trabajadores) { if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue } }
    exit $codigo
}

Write-Host "== maestro con $Nodos nodos de $Hilos goroutines cada uno"
& ./bin/maestro.exe -nodos $Nodos -escuchar ":$Puerto" -epocas $Epocas -muestra $Muestra -modo tcp_local -maquina "$env:COMPUTERNAME local"
$codigo = $LASTEXITCODE

if (-not $Ventanas) {
    foreach ($p in $trabajadores) { $p.WaitForExit(5000) | Out-Null; if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force } }
    Write-Host "`n== últimas líneas de cada trabajador (completas en bin/logs/)"
    for ($i = 1; $i -le $Nodos; $i++) {
        Write-Host "-- local$i"
        Get-Content "bin/logs/trabajador-local$i.log" -Tail 3 -Encoding UTF8
    }
}
exit $codigo
