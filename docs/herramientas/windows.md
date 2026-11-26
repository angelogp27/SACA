# Herramientas en Windows (una vez)

Al terminar, tu laptop compila y abre SACA con un solo comando. Después sigue con la red: [switch](../switch/ips.md) o [VPN](../vpn/nodo.md).

## 1. Programas

Instala con *Next* en todo:

| Programa | Dónde |
|---|---|
| Git | https://git-scm.com/download/win |
| Go | https://go.dev/dl/ (el `.msi` que termina en `windows-amd64.msi`) |
| Node.js | https://nodejs.org (versión **LTS**) |

Abre una **PowerShell nueva** y comprueba: `git --version; go version; node --version`

## 2. gcc de 64 bits (para Pthreads y OpenMP)

1. Instala MSYS2 desde https://www.msys2.org (carpeta `C:\msys64`).
2. Abre **MSYS2 UCRT64** (ícono amarillo, **no** "MSYS2 MSYS") y pega:

   ```sh
   pacman -Syu --noconfirm
   pacman -S --needed --noconfirm mingw-w64-ucrt-x86_64-gcc
   ```

   Si se cierra la ventana, ábrela y repite las dos líneas.
3. En PowerShell, agrégalo al PATH:

   ```powershell
   [Environment]::SetEnvironmentVariable("Path", "C:\msys64\ucrt64\bin;" + [Environment]::GetEnvironmentVariable("Path", "User"), "User")
   ```

4. En una PowerShell **nueva**: `gcc -dumpmachine` debe decir **`x86_64-w64-mingw32`**. Si dice `i686-...` (32 bits), otro gcc va primero en el PATH (`where.exe gcc` muestra cuál): con él la limpieza falla con *"sin memoria para los registros"*.

## 3. El proyecto

```powershell
cd C:\
git clone https://github.com/angelogp27/SACA.git saca
cd C:\saca
```

## 4. Los 4 CSV de la EPA (~4 GB)

Van en `C:\saca\data\raw\` con estos nombres exactos:

| Archivo | Gas |
|---|---|
| `hourly_42101_2025.csv` | CO |
| `hourly_42401_2025.csv` | SO2 |
| `hourly_42602_2025.csv` | NO2 |
| `hourly_44201_2025.csv` | O3 |

Cópialos de otra laptop (USB o Drive) o descárgalos: `go run .\herramientas\cmd\descargar` (unos 10 min).

**Sin espacio para 4 GB:** copia solo `data\muestra.bin` (~360 MB) de una laptop que ya limpió. No podrás limpiar, pero sí entrenar y predecir.

## 5. Primera vez

```powershell
powershell -ExecutionPolicy Bypass -File scripts\saca.ps1
```

Compila todo y abre **http://127.0.0.1:8090**. Si Windows pregunta por el firewall, marca **redes privadas** y **Permitir**.

## 6. NVIDIA (opcional)

Solo si tienes GPU NVIDIA. Ayuda a la CPU en el entrenamiento y la predicción. Si no te sale en 30 min, sáltalo: tu laptop funciona igual.

1. `nvidia-smi` debe mostrar tu tarjeta y `CUDA Version` 12 o mayor (si no, actualiza el driver desde https://www.nvidia.com/es-la/drivers/).
2. Instala **Visual Studio Build Tools 2022** (carga *Desarrollo para el escritorio con C++*) y el **CUDA Toolkit** 12.x (https://developer.nvidia.com/cuda-downloads). Reinicia.
3. En **x64 Native Tools Command Prompt for VS 2022**, desde `C:\saca`:

   ```bat
   nvcc -O3 -o bin\gradiente_cuda.exe nativo\cuda\gradiente_servidor.cu
   nvcc -O3 -o bin\red_cuda.exe nativo\cuda\red\red.cu
   bin\gradiente_cuda.exe --info
   ```

   Debe mostrar tu tarjeta. Desde ahí tu nodo la usa solo. Más detalle: [nativo/cuda/README.md](../../nativo/cuda/README.md).

## MPI (opcional)

Como comunicación entre laptops: [OpenMP, CUDA y MPI](../desarrollo/guias/fase6.md).

## Si algo falla

- **`no se reconoce como nombre de un cmdlet`**: cierra todas las PowerShell y abre otra; si sigue, reinstala ese programa.
- **`go.mod requires go >= ...`**: tu Go es viejo; instala el de https://go.dev/dl/.
- **Sin gcc**: tu laptop limpia solo con Go; vale igual.
- **8 GB de RAM**: cierra el navegador pesado mientras limpia.
