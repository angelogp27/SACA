# Une esta PC al clúster como nodo trabajador. Es lo único que corre cada
# integrante el día de la demo (ver docs/manual_cluster.md, paso 6):
#   powershell -ExecutionPolicy Bypass -File scripts/nodo.ps1 -Maestro 100.101.102.103:9000 -Id ana
# Comprueba la muestra y la conexión con el maestro, elige el motor (CUDA si
# hay GPU NVIDIA y bin/gradiente_cuda.exe; si no, goroutines) y lanza el
# trabajador.
param(
    [Parameter(Mandatory = $true)][string]$Maestro,
    [string]$Id = $env:COMPUTERNAME.ToLower(),
    [string]$Motor = "auto",
    [int]$Hilos = 0
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
    throw "no hay Go ni bin/trabajador.exe: instala Go (manual, paso 2) o pide el .exe al grupo"
}

Write-Host "== 2/4 muestra de datos"
if (-not (Test-Path data/muestra.gob)) { throw "falta data/muestra.gob: descárgala de la carpeta compartida del grupo (manual, paso 3)" }
$huella = (Get-FileHash data/muestra.gob -Algorithm SHA256).Hash.ToLower()
Write-Host "   huella $($huella.Substring(0, 12)) (debe ser igual a la que muestra el maestro)"

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
& ./bin/trabajador.exe -maestro $Maestro -id $Id -motor $Motor -hilos $Hilos
