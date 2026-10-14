# Plan de aprendizaje — Proyecto 9 (SACA)

Orden sugerido según cuándo lo pide el cronograma (ver [proyecto9.md](proyecto9.md) §10). Desde el 13 de octubre de 2026 el proyecto implementa todas las tecnologías del sílabo, así que el plan incluye C con Pthreads, OpenMP, MPI y CUDA además de Go.

Cada integrante necesita los temas 1-3, la base común. Después basta con profundizar en la tecnología de la que es responsable.

## Base común

1. **Go básico.** Sintaxis, structs, interfaces, slices y maps, manejo de errores, módulos (`go.mod`). Es el lenguaje del sistema completo.
2. **Concurrencia en Go.**
   - Goroutines, channels, `select`, `sync.WaitGroup`, `sync.Mutex`, `sync/atomic`.
   - Patrones: worker pool, fan-out, fan-in.
   - Correr siempre con `go test -race`.

   Es el corazón de la Parte A y de la sección crítica de la Parte B. Ejemplos reales en `cargador/cmd/concurrente` e `internal/logistica/paralelo.go`.
3. **Métricas de rendimiento.**
   - Speedup S(p) = T(1)/T(p) y eficiencia E(p) = S(p)/p.
   - Ley de Amdahl y métrica de Karp-Flatt.
   - Por qué se usa la mediana de varias repeticiones.

   Ver `experimentos/cmd/analisis` y [docs/experimento1.md](../docs/experimento1.md).
4. **Regresión logística y descenso de gradiente.** Solo lo esencial: sigmoide, log loss, y que el gradiente es una suma sobre ejemplos independientes. Por eso se puede repartir entre hilos, procesos o hilos de GPU, y por eso todas las implementaciones deben dar la misma pérdida.
5. **Procesar archivos grandes sin cargarlos en memoria.** `bufio`, partir un archivo en bloques de bytes ajustados al salto de línea. Es el diseño de la Parte A en Go y en Pthreads.

## Tecnologías del sílabo (núcleos nativos en C/C++)

6. **C para estos núcleos.**
   - Punteros y arreglos, `malloc`/`free`.
   - Leer un archivo binario con `fread` (formato en [docs/contrato_formato_binario.md](../docs/contrato_formato_binario.md)).
   - Medir tiempo con `clock_gettime` u `omp_get_wtime`.
   - Compilar con gcc de MSYS2 en Windows (`C:\msys64\ucrt64\bin\gcc.exe`) o con gcc en Linux.
7. **Pthreads** (sílabo semanas 5-6; Experimento 5).
   - `pthread_create`/`pthread_join`, `pthread_mutex_t`, `pthread_cond_t` (siempre dentro de un `while`), `pthread_barrier_t`.
   - Productor-consumidor con buffer circular acotado.

   Referencias: programas "Productor Consumidor" y "Semáforos" del repositorio del curso; Pacheco y Malensek, cap. 4.
8. **OpenMP** (sílabo semanas 7, 9 y 10; Experimento 6).
   - `#pragma omp parallel for`, `private`/`shared`, `reduction(+:arr[:n])`.
   - `schedule(static|dynamic|guided, bloque)`, `#pragma omp task` / `taskwait`, `omp_get_wtime`.

   Referencias: programas "Suma de Vectores" y "Problema de la mochila"; Pacheco y Malensek, cap. 5.
9. **MPI** (sílabo semanas 3-4; Experimento 7).
   - `MPI_Init`, `MPI_Comm_rank/size`, `MPI_Send/Recv`.
   - Colectivas: `MPI_Bcast`, `MPI_Scatterv`, `MPI_Reduce`/`MPI_Allreduce`; `MPI_Wtime`.
   - Ejecutar con `mpiexec -n 4`.

   Instalación: MS-MPI en Windows (necesita administrador) u OpenMPI en Linux/Docker. Referencias: programas "Cálculo de PI" y "Producto Matriz Vector"; Pacheco y Malensek, cap. 3.
10. **CUDA** (sílabo semanas 11-13; Experimento 8).
    - Modelo de hilos, bloques y grid; `cudaMalloc`/`cudaMemcpy`.
    - Kernels con `atomicAdd`; reducción en `__shared__` con `__syncthreads`; warp shuffles (`__shfl_down_sync`).
    - Thrust: `sort_by_key`, `reduce_by_key`. Medir con `cudaEvent`.

    Solo corre en GPU **NVIDIA**: se practica en las PCs del grupo que la tienen (con CUDA Toolkit) o en Google Colab. La PC de desarrollo principal tiene una AMD RX 580, que no sirve para CUDA. Referencia: Hwu, Kirk y El Hajj, *Programming Massively Parallel Processors*, caps. 2-6 y 10.

11. **Clústeres** (sílabo semanas 2, 3-4 y 10; Experimentos 2, 7, 7a y 9).
    - Redes locales: IP fija, `ping`, `iperf3`, firewall de Windows.
    - Ejecutar MPI en varias PCs: MS-MPI con `smpd -d` y `mpiexec -hosts`, u OpenMPI con SSH sin contraseña y `hostfile`.
    - Modelo híbrido MPI + OpenMP + CUDA y balanceo de carga en un clúster heterogéneo.
    - Modelo de costo de la comunicación (latencia + tamaño/ancho de banda).

    Guía paso a paso: [docs/guia_cluster.md](../docs/guia_cluster.md). Referencia: Pacheco y Malensek, cap. 2 (hardware paralelo y redes de interconexión).

## Sistema e infraestructura

12. **`net` y TCP en Go.** `net.Listen`, `net.Dial`, un JSON por línea. Ver `internal/protocolo` y `entrenamiento/cmd/maestro`.
13. **MongoDB desde Go.** `mongo-driver` (`InsertMany` por lotes, agregaciones con `$group`/`$percentile`). Redis (`go-redis`, GET/SET/TTL) desde la semana 12.
14. **Docker y Docker Compose.** `Dockerfile`, `docker-compose.yml`, redes entre contenedores, réplicas. En Windows, Docker Desktop necesita WSL 2. Se usa para Mongo y Redis, para los trabajadores de Go y opcionalmente para un clúster con OpenMPI.
15. **JWT y WebSocket en Go.** `golang-jwt/jwt`, `gorilla/websocket`. Para la Parte C, semanas 9-11.

## Bibliografía

- *Learn Concurrent Programming with Go* (Cutajar, Manning, 2024): temas 1-2.
- *An Introduction to Parallel Programming* (Pacheco y Malensek, 2.ª ed., 2022), disponible en la biblioteca de la FISI: temas 3 y 7-9.
- *Programming Massively Parallel Processors* (Hwu, Kirk y El Hajj): tema 10.
- Repositorio del curso con los programas de laboratorio de MPI, Pthreads, OpenMP y CUDA: https://github.com/FISI-SM/parallelProgramming-repo/tree/main
