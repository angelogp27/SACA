# Compila nativo/cuda/gradiente_servidor.cu con g++ (MSYS2) emulando CUDA
# en la CPU, para probar el motor cuda del trabajador en una PC sin GPU
# NVIDIA. El resultado (bin/gradiente_cuda_emulado.exe) es lento: solo sirve
# para comprobar que el protocolo y la reducción dan el mismo gradiente que Go.
# Uso (desde la raíz del repo):
#   powershell -ExecutionPolicy Bypass -File nativo/cuda/emulacion/compilar.ps1
#   .\bin\trabajador.exe -motor cuda -cuda-exe bin/gradiente_cuda_emulado.exe -maestro localhost:9000
param([string]$Gpp = "C:\msys64\ucrt64\bin\g++.exe")
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "../../..")

New-Item -ItemType Directory -Force bin | Out-Null
$fuente = Get-Content nativo/cuda/gradiente_servidor.cu -Raw
# Las dos únicas construcciones que no son C++: la memoria compartida y el
# lanzamiento del kernel con <<< >>>.
$fuente = $fuente.Replace("extern __shared__ double compartida[];", "double *compartida = (double *)memoria_compartida_bloque.data();")
$fuente = $fuente.Replace("gradiente_bloques<<<bloques, hilos, bytesCompartida>>>(", "lanzar(bloques, hilos, bytesCompartida, gradiente_bloques, ")
$tmp = "bin/gradiente_cuda_emulado.cpp"
[IO.File]::WriteAllText((Join-Path (Get-Location) $tmp), $fuente)

& $Gpp -std=c++20 -O2 -Wall -Wextra -I nativo/cuda/emulacion -o bin/gradiente_cuda_emulado.exe $tmp
if ($LASTEXITCODE -ne 0) { throw "falló la compilación" }
& ./bin/gradiente_cuda_emulado.exe --info
Write-Host "listo: bin/gradiente_cuda_emulado.exe"
