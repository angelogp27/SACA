# MongoDB local SIN Docker para Windows: descarga una sola vez el zip
# oficial de MongoDB 7 (portable, no requiere instalar ni ser
# administrador) en %LOCALAPPDATA%\saca\mongodb y lo arranca en
# localhost:27017 con la caché de WiredTiger limitada a 2 GB (ver
# docker-compose.yml). Útil si Docker Desktop no funciona (falta WSL 2).
# Uso (desde la raíz del repo):
#   powershell -ExecutionPolicy Bypass -File scripts/mongo-local.ps1            # arrancar
#   powershell -ExecutionPolicy Bypass -File scripts/mongo-local.ps1 -Detener   # detener
#   powershell -ExecutionPolicy Bypass -File scripts/mongo-local.ps1 -Estado    # ¿está corriendo?
param(
    [switch]$Detener,
    [switch]$Estado,
    [string]$Version = "7.0.14",
    [int]$CacheGB = 2
)
$ErrorActionPreference = "Stop"

$base = Join-Path $env:LOCALAPPDATA "saca\mongodb"
$bin = Join-Path $base "mongodb-win32-x86_64-windows-$Version\bin\mongod.exe"
$datos = Join-Path $base "db"
$log = Join-Path $base "mongod.log"

function Corriendo { Get-Process mongod -ErrorAction SilentlyContinue }

if ($Estado) {
    if (Corriendo) { Write-Host "MongoDB está corriendo (PID $((Corriendo).Id))." } else { Write-Host "MongoDB no está corriendo." }
    exit 0
}

if ($Detener) {
    if (Corriendo) { Corriendo | Stop-Process; Write-Host "MongoDB detenido." } else { Write-Host "MongoDB no estaba corriendo." }
    exit 0
}

if (Corriendo) {
    Write-Host "MongoDB ya está corriendo (PID $((Corriendo).Id)) en localhost:27017."
    exit 0
}

if (-not (Test-Path $bin)) {
    New-Item -ItemType Directory -Force $base | Out-Null
    $zip = Join-Path $base "mongodb.zip"
    $url = "https://fastdl.mongodb.org/windows/mongodb-windows-x86_64-$Version.zip"
    Write-Host "Descargando MongoDB $Version (~600 MB, solo la primera vez) desde $url"
    $ProgressPreference = "SilentlyContinue"
    Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
    Write-Host "Descomprimiendo en $base"
    Expand-Archive -Path $zip -DestinationPath $base -Force
    Remove-Item $zip
}

New-Item -ItemType Directory -Force $datos | Out-Null
$argumentos = @("--dbpath", "`"$datos`"", "--bind_ip", "127.0.0.1", "--port", "27017",
    "--wiredTigerCacheSizeGB", "$CacheGB", "--logpath", "`"$log`"", "--logappend", "--quiet")
$proceso = Start-Process -FilePath $bin -ArgumentList $argumentos -WindowStyle Hidden -PassThru

# Esperar a que acepte conexiones.
for ($i = 0; $i -lt 30; $i++) {
    try {
        $cliente = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 27017)
        $cliente.Close()
        Write-Host "MongoDB corriendo en localhost:27017 (PID $($proceso.Id)). Datos en $datos"
        Write-Host "Para detenerlo: powershell -ExecutionPolicy Bypass -File scripts/mongo-local.ps1 -Detener"
        exit 0
    } catch { Start-Sleep -Milliseconds 500 }
    if ($proceso.HasExited) { break }
}
throw "MongoDB no arrancó; revisa el log en $log"
