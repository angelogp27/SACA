# Compila todo lo que necesita SACA en esta laptop (Windows) y abre la página.
# Es lo único que corre cada integrante: en la página elige "Iniciar
# clúster" (una sola laptop) o "Unirse al clúster" (las demás).
#   powershell -ExecutionPolicy Bypass -File scripts/saca.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/saca.ps1 -SinCompilar
# Para el cargador de Pthreads hace falta gcc de MinGW-w64 (MSYS2 UCRT64:
# pacman -S mingw-w64-ucrt-x86_64-gcc, y agregar C:\msys64\ucrt64\bin al
# PATH). Sin gcc, la limpieza de esta laptop corre solo con Go.
param([switch]$SinCompilar)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

if (-not $SinCompilar) {
    New-Item -ItemType Directory -Force bin | Out-Null
    Write-Host "== 1/3 programas en Go"
    $programas = @{ maestro = "./entrenamiento/cmd/maestro"; trabajador = "./entrenamiento/cmd/trabajador";
                    cargador_concurrente = "./cargador/cmd/concurrente"; saca = "./lanzador/cmd/saca"; exportar = "./herramientas/cmd/exportar" }
    foreach ($p in $programas.GetEnumerator()) {
        go build -o "bin/$($p.Key).exe" $p.Value
        if ($LASTEXITCODE -ne 0) { throw "no se pudo compilar $($p.Key)" }
        Write-Host "   bin/$($p.Key).exe"
    }

    Write-Host "== 2/3 cargador de Pthreads (C)"
    $gccMaquina = if (Get-Command gcc -ErrorAction SilentlyContinue) { (gcc -dumpmachine) } else { "" }
    if ($gccMaquina -and $gccMaquina -notmatch '^x86_64') {
        # Un gcc de 32 bits genera un cargador que solo puede usar ~2 GB:
        # la limpieza termina en "sin memoria para los registros".
        Write-Host "   AVISO: tu gcc es de 32 bits ($gccMaquina). Instala el de 64 bits (MSYS2 UCRT64:"
        Write-Host "   pacman -S mingw-w64-ucrt-x86_64-gcc) y pon C:\msys64\ucrt64\bin primero en el PATH."
        if (Test-Path bin/cargador_pthreads.exe) { Remove-Item bin/cargador_pthreads.exe }
        Write-Host "   Mientras tanto esta laptop limpia solo con Go."
    } elseif ($gccMaquina) {
        # -ffp-contract=off es obligatorio: sin él la muestra.bin sale con otra huella.
        gcc -O3 -ffp-contract=off -o bin/cargador_pthreads.exe nativo/pthreads/cargador/cargador.c -static -lpthread -lm
        if ($LASTEXITCODE -ne 0) { throw "no se pudo compilar el cargador de Pthreads" }
        Write-Host "   bin/cargador_pthreads.exe"
    } elseif (Test-Path bin/cargador_pthreads.exe) {
        Write-Host "   gcc no está instalado; se usa el bin/cargador_pthreads.exe que ya existe"
    } else {
        Write-Host "   AVISO: no hay gcc (MinGW-w64) ni bin/cargador_pthreads.exe: esta laptop limpiará solo con Go"
    }

    Write-Host "== 3/3 página"
    if (Get-Command npm -ErrorAction SilentlyContinue) {
        Push-Location frontend
        if (-not (Test-Path node_modules)) { npm ci; if ($LASTEXITCODE -ne 0) { throw "npm ci falló" } }
        npm run build | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "no se pudo compilar la página" }
        Pop-Location
        Write-Host "   frontend/dist"
    } elseif (Test-Path frontend/dist/index.html) {
        Write-Host "   npm no está instalado; se usa el frontend/dist que ya existe"
    } else {
        throw "instala Node.js (npm) para compilar la página, o copia frontend/dist de otra laptop"
    }
}

foreach ($c in "44201", "42602", "42401", "42101") {
    if (-not (Test-Path "data/raw/hourly_${c}_2025.csv")) { Write-Host "AVISO: falta data/raw/hourly_${c}_2025.csv (sin los 4 CSV esta laptop no puede limpiar)" }
}
& ./bin/saca.exe
