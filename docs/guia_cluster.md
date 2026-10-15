# Guía del clúster del grupo

Cómo convertir las computadoras del grupo en un **clúster heterogéneo**: **2 PCs con GPU NVIDIA** (nodos GPU) y el resto aportando su procesador (nodos CPU), conectadas por la red local. Explica qué corre en cada PC, cómo se lanza cada tecnología (Go por TCP, MPI, MPI + OpenMP + CUDA) y qué se mide.

Es la base de los Experimentos 2, 7 y 9 ([.context/proyecto9.md](../.context/proyecto9.md) §8) y de la demo final.

> **Para ejecutar el clúster ya mismo** (Forma 1, incluso con PCs en casas distintas por Tailscale y con nodos CUDA), seguir el paso a paso de [manual_cluster.md](manual_cluster.md). Para la exposición: [guia_presentacion_cluster.md](guia_presentacion_cluster.md).

## 1. Por qué un clúster y qué enseña

- El sílabo clasifica los computadores paralelos por su arquitectura (semana 2, sumilla: *"clasificación física de computadores paralelos"*). El clúster del grupo es un **multicomputador MIMD de memoria distribuida**: cada PC tiene su propia memoria y solo se comunican por la red. Dentro de cada PC hay además memoria compartida (varios núcleos) y, en dos de ellas, una GPU SIMT. Es la misma estructura jerárquica de los supercomputadores actuales, en pequeño.
- Por eso el modelo de programación natural es **híbrido**, como en HPC:

  | Nivel | Tecnología |
  |---|---|
  | Entre PCs | **MPI** (o TCP en Go) |
  | Dentro de cada PC | **OpenMP** (o goroutines) |
  | Dentro de cada GPU | **CUDA** |

  Lo habitual es **un proceso MPI por PC (o por GPU)** y paralelismo de memoria compartida o de GPU dentro de cada proceso ([UMD, MPI+X](https://www.cs.umd.edu/class/fall2024/cmsc416/slides/09-cmsc416-mpi+x.pdf); [análisis comparativo MPI/OpenMP/CUDA](https://arxiv.org/pdf/2506.15454)).
- Lo que se aprende al medir: **el costo de la comunicación**.
  - Una época de entrenamiento con la muestra actual tarda unos 12 ms en una CPU, pero solo intercambia 13 números (12 pesos + sesgo).
  - En cable la latencia de red es de ~0,2-1 ms; por WiFi, de 2-10 ms o más.
  - Con el problema actual, la comunicación pesa mucho y el speedup entre PCs será bajo. Para que el clúster gane, el trabajo por época tiene que crecer (todos los registros y no solo una muestra, más variables con los rezagos).

  Medir ese punto de cruce es un resultado del proyecto, igual que el CPU frente a GPU del Experimento 8.

## 2. Topología y roles

```
                      ┌────────────────────────────────────┐
                      │ Switch o router de la red local     │
                      │ (preferible cable; WiFi solo pruebas)│
                      └──┬──────────┬──────────┬──────────┬─┘
                         │          │          │          │
  ┌──────────────────────┴─┐ ┌──────┴───────┐ ┌┴───────────┐ ┌┴────────────┐
  │ NODO PRINCIPAL         │ │ NODO GPU 1   │ │ NODO GPU 2 │ │ NODOS CPU   │
  │ MongoDB, maestro Go,   │ │ NVIDIA       │ │ NVIDIA     │ │ (el resto)  │
  │ API, frontend,         │ │ rank MPI     │ │ rank MPI   │ │ rank MPI +  │
  │ rank 0 de MPI          │ │ + CUDA       │ │ + CUDA     │ │ OpenMP;     │
  │                        │ │              │ │            │ │ trabajador  │
  │                        │ │              │ │            │ │ Go          │
  └────────────────────────┘ └──────────────┘ └────────────┘ └─────────────┘
```

| Rol | Cuántas | Qué corre | Requisito |
|---|---|---|---|
| Principal | 1 (la de más RAM, idealmente 16 GB) | MongoDB, `entrenamiento/cmd/maestro`, API, frontend, rank 0 de MPI, recolección de resultados | Datos de la EPA, Go, Docker o MongoDB |
| Nodo GPU | 2 | Un proceso MPI que calcula su parte del gradiente con CUDA; trabajador Go | Driver NVIDIA + CUDA Toolkit |
| Nodo CPU | el resto | Un proceso MPI con hilos OpenMP; trabajador Go | gcc con OpenMP, MPI |

### Inventario (completar en el grupo)

Antes de configurar nada, cada integrante llena su fila. Sin esto no se puede balancear la carga ni interpretar los tiempos.

| PC (nombre de red) | Integrante | CPU | Núcleos / hilos | RAM | GPU | Sistema operativo | IP fija | Rol |
|---|---|---|---|---|---|---|---|---|
| pc-piero | Piero | Ryzen 5 3600 | 6 / 12 | 16 GB | AMD RX 580 (sin CUDA) | Windows 11 | 192.168.1.__ | principal o CPU |
| | | | | | NVIDIA ___ | | | GPU 1 |
| | | | | | NVIDIA ___ | | | GPU 2 |
| | | | | | — | | | CPU |

## 3. Preparación común (todas las PCs)

1. **Misma red y una IP fija por PC**, reservada en el router o configurada a mano. Anotarla en el inventario.
2. **Comprobar la red:**
   - `ping` entre todas las parejas de PCs;
   - medir latencia y ancho de banda con `iperf3` (servidor en una, cliente en otra) y anotar los valores;
   - preferir cable: el WiFi agrega latencia variable que ensucia las mediciones.
3. **Firewall:** abrir los puertos que se usen, solo en la red privada:
   - 9000/TCP para el maestro Go;
   - MS-MPI: permitir `smpd.exe`, `mpiexec.exe` y el ejecutable del núcleo.
4. **Energía:** desactivar la suspensión y usar el plan de alto rendimiento durante las mediciones.
5. **Misma versión del código:** `git pull` en todas, al mismo commit. Los resultados se guardan con el commit y el nombre de la PC (columna `maquina` del CSV).
6. **Datos:** cada nodo necesita `data/muestra.bin` (unos 15 MB). Hay dos opciones:
   - se copia a cada PC, y `herramientas/cmd/descargar -verificar` y el SHA-256 del manifiesto confirman que es el mismo;
   - o el rank 0 lo reparte por la red con `MPI_Scatterv`, que además es parte de lo que se mide.

## 4. Forma 1: Go maestro-trabajador por TCP (la más simple)

**Funciona desde el 15 de octubre de 2026 con entrenamiento distribuido completo.** No necesita instalar nada en los nodos, porque Go genera un ejecutable autocontenido. Cada nodo calcula con goroutines o, si tiene GPU NVIDIA, con el motor CUDA (`-motor auto`). Si las PCs no están en la misma red, se unen con **Tailscale** ([manual_cluster.md](manual_cluster.md), paso 2).

```powershell
# En la PC principal: compilar los trabajadores para Windows y Linux
$env:GOOS="windows"; go build -o bin/trabajador.exe ./entrenamiento/cmd/trabajador
$env:GOOS="linux";   go build -o bin/trabajador     ./entrenamiento/cmd/trabajador
Remove-Item Env:GOOS

# En la PC principal
go run ./entrenamiento/cmd/maestro -nodos 6 -escuchar :9000

# En cada nodo (copiar el ejecutable por USB, red o git)
.\trabajador.exe -maestro 192.168.1.10:9000 -id gpu1
```

- Mezclar Windows y Linux funciona sin problemas, porque el protocolo es JSON sobre TCP.
- Cada trabajador calcula su gradiente parcial sobre su partición, y el maestro verifica al final que la pérdida es la misma que en secuencial. Es el Experimento 2 en un clúster real: 1, 2, 4 y 8 nodos físicos en lugar de procesos en una sola PC (`scripts/experimento2.ps1`).

## 5. Forma 2: MPI entre PCs

Regla importante: **todas las PCs de un mismo trabajo MPI deben tener el mismo sistema operativo y la misma implementación y versión de MPI**. No se puede mezclar MS-MPI en Windows con OpenMPI en Linux en una misma ejecución.

### 5a. Windows con MS-MPI (si la mayoría usa Windows)

Según el [blog de Microsoft HPC](https://learn.microsoft.com/it-it/archive/blogs/windowshpc/how-to-compile-and-run-a-simple-ms-mpi-program), se puede ejecutar entre varias máquinas **sin HPC Pack**:

1. Instalar **la misma versión de MS-MPI** (runtime; y el SDK en la PC que compila) en todas las PCs. Para compilar con gcc de MSYS2: `pacman -S mingw-w64-ucrt-x86_64-msmpi`.
2. Usar el mismo usuario y la misma contraseña en todas las PCs, o registrar las credenciales con `mpiexec -register`.
3. Poner el ejecutable en **la misma ruta en todas las PCs** (p. ej. `C:\saca\bin\entrenamiento_mpi.exe`), o en una carpeta compartida usando `-wdir \\pc-principal\saca`.
4. En **cada** PC, iniciar el demonio: `smpd -d` (dejar la ventana abierta).
5. Agregar las reglas del firewall para `smpd.exe` y para el ejecutable.
6. Lanzar desde la PC principal, indicando cuántos procesos van en cada PC:

```powershell
# 1 proceso por PC (híbrido MPI + OpenMP); OMP_NUM_THREADS = núcleos de cada PC
mpiexec -hosts 4 192.168.1.10 1 192.168.1.11 1 192.168.1.12 1 192.168.1.13 1 `
        C:\saca\bin\entrenamiento_mpi.exe --datos C:\saca\data\muestra.bin --modo colectivo
```

- Para entrar varias PCs, no usar `mpiexec -n` solo: la referencia de MS-MPI advierte que `-n` sin lista de hosts pone todos los procesos en una sola computadora ([referencia de mpiexec](https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-compute-cluster-server-2003/cc720141(v=ws.10))). También se puede usar `-machinefile hosts.txt`.
- Diagnóstico:
  - probar primero un comando que no sea MPI en la PC remota, por ejemplo `mpiexec -hosts 1 192.168.1.11 1 hostname`;
  - si no conecta, revisar que `smpd -d` esté corriendo allí y el firewall;
  - un error de "version mismatch" significa versiones distintas de MPI ([hilo de mpich-discuss](https://lists.mcs.anl.gov/pipermail/mpich-discuss/2011-March/009296.html)).

### 5b. Linux con OpenMPI (si se usa Linux, en dual boot o USB booteable)

Según la [documentación de Open MPI](https://docs.open-mpi.org/en/main/_sources/launching-apps/ssh.rst.txt) y [esta guía para Ubuntu](https://oneuptime.com/blog/post/2026-03-02-how-to-set-up-mpi-cluster-on-ubuntu/markdown):

1. `sudo apt install openmpi-bin libopenmpi-dev` en todas las PCs, misma versión.
2. Mismo usuario en todas, y **SSH sin contraseña desde cada nodo hacia todos los demás**:
   - generar la clave con `ssh-keygen -t ed25519`;
   - copiarla con `ssh-copy-id usuario@ip`;
   - probar con `ssh ip hostname`, que no debe pedir nada.
3. El ejecutable en la misma ruta en todas, o en una carpeta NFS compartida.
4. Un `hostfile` con los procesos ("slots") por PC:

```
192.168.1.10 slots=1
192.168.1.11 slots=1   # GPU 1
192.168.1.12 slots=1   # GPU 2
192.168.1.13 slots=1
```

```sh
mpirun --hostfile hostfile -x OMP_NUM_THREADS=6 ./entrenamiento_mpi --datos /home/saca/data/muestra.bin --modo colectivo
```

### 5c. Prueba de humo obligatoria

Antes de medir cualquier cosa, correr en el clúster un **ping-pong MPI**: dos procesos en PCs distintas se envían un mensaje con `MPI_Send`/`MPI_Recv` mil veces, con tamaños de 8 B a 1 MB.

- Da la latencia y el ancho de banda **reales** del clúster, y es exactamente el tema de la semana 3 del sílabo.
- Con esos dos números se puede predecir el tiempo de comunicación por época y comprobar el modelo después.
- Será `nativo/mpi/pingpong.c`.

## 6. Forma 3: híbrido MPI + OpenMP + CUDA (la versión completa)

Un proceso MPI por PC:

```
rank 0 (principal):  lee muestra.bin → MPI_Scatterv (partición proporcional a la capacidad de cada nodo)
cada época:
    MPI_Bcast(pesos, sesgo)                               ← desde el rank 0
    si el nodo tiene GPU:  gradiente_local = kernel CUDA sobre su partición
    si no:                 gradiente_local = OpenMP reduction sobre su partición
    MPI_Allreduce(gradiente_local → gradiente_total, SUM)
    todos actualizan los pesos igual (o solo el rank 0, y vuelve a difundir)
```

- **Balanceo de carga heterogéneo** (semana 10 del sílabo): repartir los ejemplos en partes iguales deja esperando a las GPUs, o a las CPUs lentas.
  1. Antes de entrenar, cada nodo mide su rendimiento (ejemplos por segundo en una época de prueba).
  2. El rank 0 reparte **en proporción a ese rendimiento** con `MPI_Scatterv` y conteos distintos por nodo.

  El Experimento 9 compara reparto igual con reparto proporcional.
- **No hace falta MPI "CUDA-aware"** (pasar punteros de GPU directamente a MPI). Esa función existe para mover grandes volúmenes entre GPUs sin pasar por la RAM ([guía de Sigma2](https://documentation.sigma2.no/code_development/guides/gpuaware_mpi.html)), pero aquí cada época solo intercambia 13 números. Basta con copiar el gradiente de la GPU a la RAM (`cudaMemcpy`) y llamar a `MPI_Allreduce`. Además, MS-MPI en Windows no la ofrece.
- **Compilación en los nodos GPU:** la parte CUDA se compila con `nvcc` y se enlaza con MPI. Hay dos opciones:
  - en Windows, `nvcc` usa `cl.exe` de Visual Studio y se enlaza con `msmpi.lib` del SDK de MS-MPI;
  - en Linux, `nvcc -ccbin mpicxx` o `mpicxx` + `-lcudart`.

  Si complica, se separa en dos ejecutables con la misma interfaz: `entrenamiento_mpi` (CPU) y `entrenamiento_mpi_cuda` (GPU), lanzados juntos con la sintaxis MPMD de `mpiexec` (`mpiexec ... prog1 : ... prog2`).
- Riesgos conocidos: llamadas MPI bloqueantes que dejan la GPU ociosa mientras espera la red. Se mitiga superponiendo cálculo y comunicación con streams de CUDA y MPI no bloqueante ([guía práctica](https://www.cuda.it.com/openmp-mpi-gpu-programming.html)). Es una optimización opcional para la semana 13.

## 7. Docker Swarm (opcional, solo para los trabajadores Go)

Si las PCs tienen Docker (en Windows requiere WSL 2), Docker Swarm permite escalar los trabajadores Go con un comando:

```sh
docker swarm init --advertise-addr 192.168.1.10   # en la principal
docker swarm join --token ... 192.168.1.10:2377   # en cada nodo
docker service scale saca_trabajador=6
```

Hay que abrir los puertos 2377/TCP, 7946/TCP+UDP y 4789/UDP. Es cómodo para la demo, pero **no es necesario**: la Forma 1 hace lo mismo con un ejecutable por PC. Para MPI no se usa Swarm.

## 8. Experimentos en el clúster

| # | Experimento | Qué se varía | Qué se espera ver |
|---|---|---|---|
| 2 | Go TCP en PCs reales | 1, 2, 4 y 8 nodos | Speedup bajo con la muestra actual (domina la latencia); mejor con todos los registros |
| 7 | MPI entre PCs | 1 PC con N procesos frente a N PCs con 1 proceso; punto a punto frente a colectivas | Diferencia entre memoria compartida y red; `Allreduce` frente a N envíos |
| 7a | Ping-pong MPI | Tamaño del mensaje, de 8 B a 1 MB | Latencia y ancho de banda: el modelo de costo de la red |
| 9 | Clúster heterogéneo | Reparto igual frente a proporcional; con y sin los nodos GPU | Balanceo de carga: el nodo más lento marca el ritmo |
| — | Escalabilidad débil | Datos proporcionales al número de nodos | Si el tiempo por época se mantiene constante |

Todos escriben el mismo CSV de resultados ([contrato_formato_binario.md](contrato_formato_binario.md)), con la columna `maquina` indicando el clúster usado (p. ej. `"cluster 5 PCs, 2 GPU, cable"`).

## 9. Lista de verificación antes de medir

- [ ] Inventario completo y una IP fija por PC
- [ ] `ping` y `iperf3` entre todas las parejas, con los valores anotados
- [ ] Mismo commit en todas las PCs (`git log -1`)
- [ ] `muestra.bin` igual en todas (SHA-256) o repartido por el rank 0
- [ ] Prueba de humo: `mpiexec ... hostname` en todas las PCs, y después el ping-pong
- [ ] Cada núcleo valida su pérdida contra `muestra_referencia.json` antes de medir
- [ ] Suspensión desactivada; nada pesado corriendo en paralelo (antivirus, sincronización de OneDrive, juegos)
- [ ] Cable en lugar de WiFi para las mediciones finales
