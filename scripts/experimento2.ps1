# Experimento 2: entrenamiento distribuido maestro-trabajador por TCP con
# 1, 2, 4 y 8 nodos en esta PC (procesos separados, una goroutine por
# nodo), contra el entrenamiento secuencial T(1) que el maestro corre en
# cada repetición (-verificar). Mide el costo del protocolo sin la red
# real; las corridas en el clúster por Tailscale se agregan al mismo CSV
# con -modo tcp_tailscale (ver docs/cluster/guia_maestro.md).
# Uso (desde la raíz del repo):
#   powershell -ExecutionPolicy Bypass -File scripts/experimento2.ps1
param(
    [int]$Repeticiones = 3,
    [int[]]$Nodos = @(1, 2, 4, 8),
    [int]$Epocas = 200,
    [int]$Puerto = 9000
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

if (-not (Test-Path data/muestra.gob)) { throw "falta data/muestra.gob (se crea con scripts/pipeline.ps1)" }
New-Item -ItemType Directory -Force bin, bin/logs, resultados | Out-Null
go build -o bin/maestro.exe ./entrenamiento/cmd/maestro
go build -o bin/trabajador.exe ./entrenamiento/cmd/trabajador
go build -o bin/analisis.exe ./experimentos/cmd/analisis
if ($LASTEXITCODE -ne 0) { throw "no se pudo compilar" }

$csv = "resultados/experimento2.csv"
if (Test-Path $csv) { Remove-Item $csv }

for ($r = 1; $r -le $Repeticiones; $r++) {
    foreach ($n in $Nodos) {
        Write-Host "== repeticion ${r}/${Repeticiones}: $n nodos"
        $trabajadores = @()
        for ($i = 1; $i -le $n; $i++) {
            $trabajadores += Start-Process .\bin\trabajador.exe -PassThru -NoNewWindow `
                -ArgumentList "-maestro localhost:$Puerto -id local$i -hilos 1 -motor go" `
                -RedirectStandardError "bin/logs/e2-trabajador$i.log" -RedirectStandardOutput "bin/logs/e2-trabajador$i.out"
        }
        # Start-Process y no "&": en PowerShell 5.1 redirigir el stderr de un
        # ejecutable con 2> lo convierte en error y detiene el script.
        $maestro = Start-Process .\bin\maestro.exe -PassThru -NoNewWindow -Wait `
            -ArgumentList "-nodos $n -escuchar :$Puerto -epocas $Epocas -resultado $csv -modo tcp_local -maquina `"$env:COMPUTERNAME local`"" `
            -RedirectStandardError "bin/logs/e2-maestro.log" -RedirectStandardOutput "bin/logs/e2-maestro.out"
        if ($maestro.ExitCode -ne 0) { Get-Content bin/logs/e2-maestro.log -Tail 5 -Encoding UTF8; throw "falló la corrida con $n nodos" }
        Get-Content bin/logs/e2-maestro.log -Encoding UTF8 | Select-String -Pattern "T\(|verificación" | ForEach-Object { "   " + $_.Line }
        foreach ($p in $trabajadores) { $p.WaitForExit(5000) | Out-Null; if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force } }
    }
}

& ./bin/analisis.exe -entrada $csv -json frontend/public/datos/experimento2.json -md resultados/experimento2.md `
    -nombre "Experimento 2" -descripcion "Entrenamiento distribuido maestro-trabajador por TCP (procesos en una PC)" `
    -paralelo "tcp_local" "-serial="
