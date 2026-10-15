# Motor CUDA del entrenamiento distribuido

`gradiente_servidor.cu` calcula en una GPU NVIDIA el gradiente parcial de la regresión logística sobre la partición de un nodo del clúster.

No usa la red. Lo lanza el trabajador Go (`entrenamiento/cmd/trabajador -motor cuda`) y se comunican por stdin/stdout con un protocolo binario simple (ver el comentario al inicio del `.cu` y `internal/distribuido/externo.go`). Go se encarga de la red y del protocolo con el maestro, y CUDA del cálculo: es el modelo híbrido de [docs/guia_cluster.md](../../docs/guia_cluster.md) §6, con TCP en lugar de MPI.

```
maestro (Go) ──TCP──► trabajador (Go) ──stdin/stdout──► gradiente_cuda.exe ──► GPU
```

## Qué hace el kernel

- Los datos de la partición se copian **una sola vez** a la memoria de la GPU. Por época solo viajan 13 números de ida (pesos y sesgo) y 14 de vuelta.
- Cada hilo acumula en memoria compartida el aporte de sus ejemplos con un *grid-stride loop*.
- Cada bloque reduce con un **árbol en memoria compartida** (`__shared__` + `__syncthreads`, semana 12 del sílabo) y deja su suma en un arreglo global.
- La CPU suma las sumas de los bloques en orden fijo. El resultado es **determinista** y no requiere `atomicAdd` sobre `double`, que solo existe en sm_60 o superior. Las variantes con `atomicAdd` y *warp shuffles* son las del Experimento 8.

**Corrección.** Antes de la primera época, el trabajador compara el gradiente de la GPU con el de Go en dos puntos. Si no coinciden (tolerancia relativa 1e-8), avisa y calcula con goroutines. La demo nunca se cae por el motor CUDA.

> Para integrantes que no conocen el código: la guía paso a paso, con la salida que se debe ver en cada paso, es [docs/cluster/guia_nodo_con_nvidia.md](../../docs/cluster/guia_nodo_con_nvidia.md).

## Compilar en una PC con GPU NVIDIA

### Windows

1. Driver de NVIDIA actualizado. `nvidia-smi` debe mostrar la GPU.
2. **Visual Studio Build Tools 2022** con la carga de trabajo "Desarrollo para el escritorio con C++". `nvcc` usa `cl.exe` como compilador del lado de la CPU.
3. **CUDA Toolkit** 12.x desde https://developer.nvidia.com/cuda-downloads. Comprobar con `nvcc --version`.
4. Desde la raíz del repo, en una **"x64 Native Tools Command Prompt for VS 2022"** (así `cl.exe` está en el PATH):

   ```bat
   mkdir bin
   nvcc -O3 -o bin\gradiente_cuda.exe nativo\cuda\gradiente_servidor.cu
   bin\gradiente_cuda.exe --info
   ```

   La última línea debe mostrar algo como `GPU: NVIDIA GeForce RTX 3060, sm_86, 28 SM, 12.0 GB`.

### Linux

```sh
sudo apt install nvidia-cuda-toolkit    # o el instalador oficial de NVIDIA
mkdir -p bin
nvcc -O3 -o bin/gradiente_cuda nativo/cuda/gradiente_servidor.cu
./bin/gradiente_cuda --info
```

## Usarlo

```powershell
.\bin\trabajador.exe -maestro 100.x.y.z:9000 -id gpu-juan -motor cuda
```

Con `-motor auto` (el valor por defecto, que usa `scripts/nodo.ps1`) el trabajador usa CUDA si existe `bin/gradiente_cuda.exe` y arranca, y si no, goroutines.

En el log del trabajador deben aparecer, en orden:

```
motor cuda iniciado: NVIDIA GeForce RTX 3060, sm_86, 28 SM, 12.0 GB
registrado como gpu-juan: partición 1 de 3 → 48... ejemplos
gradiente_cuda: 48... ejemplos x 12 variables en la GPU (224 bloques x 256 hilos)
motor cuda validado contra el de Go: mismo gradiente (tolerancia relativa 1e-8)
```

## Probar sin GPU NVIDIA (emulación)

`emulacion/` compila el mismo `.cu` con g++ de MSYS2. Emula cada hilo CUDA con un hilo del sistema operativo y `__syncthreads` con una barrera. Es lenta, pero sirve para comprobar el protocolo y la reducción en cualquier PC:

```powershell
powershell -ExecutionPolicy Bypass -File nativo/cuda/emulacion/compilar.ps1
.\bin\trabajador.exe -maestro localhost:9000 -id gpu-emulada -motor cuda -cuda-exe bin/gradiente_cuda_emulado.exe
```

Con esa emulación se validó, en la PC de desarrollo (AMD, sin CUDA), que el kernel da el mismo gradiente que Go y que el entrenamiento distribuido llega a la misma pérdida que el secuencial.

## Problemas frecuentes

| Síntoma | Causa y solución |
|---|---|
| `nvcc fatal : Cannot find compiler 'cl.exe' in PATH` | Compilar desde la "x64 Native Tools Command Prompt for VS 2022", o instalar las Build Tools con C++ |
| `no hay GPU CUDA disponible` | Driver viejo o ausente: revisar `nvidia-smi`. En laptops, forzar la GPU NVIDIA en el panel de control |
| `CUDA driver version is insufficient` | El Toolkit es más nuevo que el driver: actualizar el driver |
| `motor cuda no da el mismo gradiente que el de Go` | No debería pasar. Anotar el mensaje y la GPU y avisar al grupo; mientras tanto el nodo calcula con goroutines |
| La GPU tarda más que la CPU | Es esperable con particiones pequeñas: por época hay que lanzar el kernel y copiar el resultado, lo que cuesta unas decenas de µs. Es el punto de cruce que mide el Experimento 8 |
