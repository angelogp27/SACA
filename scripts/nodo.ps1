# Une esta PC al clúster como nodo trabajador. Es lo único que corre cada
# integrante el día de la demo (ver docs/cluster/guia_nodo_sin_nvidia.md
# o guia_nodo_con_nvidia.md):
#   powershell -ExecutionPolicy Bypass -File scripts/nodo.ps1 -Maestro 100.101.102.103:9000 -Id ana
# Comprueba la muestra y la conexión con el maestro, elige el motor (CUDA si
# hay GPU NVIDIA y bin/gradiente_cuda.exe; si no, goroutines) y lanza el
# trabajador. -Muestra permite usar otra muestra (p. ej. data/muestra_prueba.gob
# en una PC con poca RAM); debe ser la misma que la del maestro.
param(
    [Parameter(Mandatory = $true)][string]$Maestro,
    [string]$Id = $env:COMPUTERNAME.ToLower(),
    [string]$Motor = "auto",
    [int]$Hilos = 0,
    [string]$Muestra = "data/muestra.gob"
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

Write-Host "== 1/4 programa del trabajador"
if (Get-Command go -ErrorAction SilentlyContinue) {
    New-Item -ItemType Directory -Force bin | Out-Null
    go build -o bin/trabajador.exe ./entrenamiento/cmd/trabajador
    if ($LASTEXITCODE -ne 0) { throw "no se pudo compilar el trabajador" }
    Write-Host "   compilado bin/trabajador.exe (commit $(git rev-parse --short HEAD))"
} elseif (Test-Path bin/trabajador.exe) {
    Write-Host "   Go no está instalado; se usa el bin/trabajador.exe que ya existe"
} else {
    throw "no hay Go ni bin/trabajador.exe: instala Go (guía del nodo, paso 1) o pide el .exe al grupo"
}

Write-Host "== 2/4 muestra de datos"
if (-not (Test-Path $Muestra)) { throw "falta ${Muestra}: descárgala de la carpeta compartida del grupo (guía del nodo, paso 'Copiar la muestra')" }
$huella = (Get-FileHash $Muestra -Algorithm SHA256).Hash.ToLower()
Write-Host "   $Muestra, huella $($huella.Substring(0, 12)) (debe ser igual a la que muestra el maestro)"

Write-Host "== 3/4 motor de cálculo"
$nvidia = Get-Command nvidia-smi -ErrorAction SilentlyContinue
if ($nvidia) {
    $gpu = (& nvidia-smi --query-gpu=name --format=csv,noheader) | Select-Object -First 1
    Write-Host "   GPU NVIDIA: $gpu"
    if (-not (Test-Path bin/gradiente_cuda.exe)) {
        Write-Host "   AVISO: falta bin/gradiente_cuda.exe; compílalo con nvcc (nativo/cuda/README.md) o se usará la CPU" -ForegroundColor Yellow
    }
} else {
    Write-Host "   sin GPU NVIDIA: se calcula con goroutines en la CPU"
}

Write-Host "== 4/4 conexión con el maestro $Maestro"
$hostMaestro, $puerto = $Maestro.Split(":")
$prueba = Test-NetConnection -ComputerName $hostMaestro -Port ([int]$puerto) -WarningAction SilentlyContinue
if (-not $prueba.TcpTestSucceeded) {
    Write-Host "   AVISO: no se pudo conectar todavía (¿el maestro ya arrancó?, ¿Tailscale conectado?, ¿firewall?); el trabajador seguirá reintentando" -ForegroundColor Yellow
} else {
    $ping = ""
    if ($prueba.PingSucceeded) { $ping = " (ping $($prueba.PingReplyDetails.RoundtripTime) ms)" }
    Write-Host "   OK$ping"
}

Write-Host "== nodo $Id en marcha (Ctrl+C para salir)"
& ./bin/trabajador.exe -maestro $Maestro -id $Id -motor $Motor -hilos $Hilos -muestra $Muestra
