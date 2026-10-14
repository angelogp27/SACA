# Conceptos del curso aplicados en SACA

Relaciona el temario completo del sílabo (unidades I a IV, semanas 1-15) con el código del proyecto. Para cada concepto se indica dónde verlo y qué decir de él en la exposición.

Cada tema se ve de dos formas:

- **En Go**, dentro del sistema que funciona de punta a punta. Go trae las mismas ideas integradas en el lenguaje:

  | Laboratorio (C) | Equivalente en Go |
  |---|---|
  | hilos de Pthreads | goroutines |
  | `pthread_join` | `sync.WaitGroup` |
  | `pthread_mutex_t` | `sync.Mutex` |
  | operaciones atómicas | `sync/atomic` |
  | cola productor-consumidor con variables de condición | `chan` |
  | procesos MPI que intercambian mensajes | procesos que se comunican por TCP |

- **Con la tecnología del laboratorio** (Pthreads, OpenMP, MPI, CUDA), en los núcleos nativos de `nativo/`. Resuelven las mismas partes del sistema con los mismos datos ([contrato_formato_binario.md](contrato_formato_binario.md)). El grupo lo decidió el 13 de octubre de 2026; el estado de cada uno está en la última tabla.

## Unidad I: fundamentos

| Concepto | Dónde está | Qué mostrar |
|---|---|---|
| Creación y terminación de tareas | [cargador/cmd/concurrente/main.go](../cargador/cmd/concurrente/main.go) | `go trabajador(...)` crea N tareas; `trabajadoras.Wait()` + `close(salida)` es la terminación ordenada, sin goroutines huérfanas. |
| Grafo de dependencias | El cargador concurrente | despachador → N trabajadoras (independientes entre sí) → K agregadoras → escritura en Mongo. Las trabajadoras no dependen unas de otras; la agregación depende de todas. |
| Paralelismo ideal y sus límites | [resultados/experimento1.md](../resultados/experimento1.md) | El speedup se compara con el ideal S(p)=p y con la ley de Amdahl usando la fracción serial **medida**. |
| Métricas de rendimiento | [experimentos/cmd/analisis](../experimentos/cmd/analisis/main.go) | Speedup, eficiencia, Karp-Flatt y predicción de Amdahl calculados con la mediana de 3 repeticiones. |
| Memoria compartida vs. distribuida | Parte A frente a Parte B | El cargador y el entrenamiento paralelo comparten memoria (goroutines). El maestro y los trabajadores no comparten nada y solo se envían mensajes TCP. |
| Divide y vencerás | [internal/aqs/aqs.go](../internal/aqs/aqs.go) `LimitesBloques` | Cada archivo se divide en bloques de bytes ajustados al salto de línea, se procesan por separado y los resultados se combinan. |

## Unidad II: memoria compartida y paso de mensajes

| Concepto (semana) | Dónde está | Qué mostrar |
|---|---|---|
| Paso de mensajes: envío y recepción (sem. 3) | [internal/protocolo](../internal/protocolo/protocolo.go), [maestro](../entrenamiento/cmd/maestro/main.go), [trabajador](../entrenamiento/cmd/trabajador/main.go) | `Enviar`/`Recibir` hacen el papel de `MPI_Send`/`MPI_Recv`, con un JSON por línea sobre TCP. |
| Comunicación colectiva: broadcast, scatter, gather, reduce (sem. 4) | Contrato TCP; [internal/logistica/paralelo.go](../internal/logistica/paralelo.go) | `particion_asignada` es el scatter del dataset. `parametros` a todos los nodos es el broadcast (semana 9). La suma de gradientes es el reduce. |
| Hilos, mutex y exclusión mutua (sem. 5) | [internal/cluster](../internal/cluster/cluster.go); el conteo de filas en el cargador concurrente | El registro de nodos es estado compartido entre las goroutines de cada conexión, así que todas sus operaciones toman el mutex. El conteo de filas se suma bajo mutex una vez por bloque y no por fila, para que la contención sea mínima. |
| Productor-consumidor (sem. 6) | Cargador concurrente | Las trabajadoras producen `ValorLeido` y las agregadoras los consumen por channels con buffer de 1000, que regulan el ritmo si la agregación va más lenta. |
| Barreras (sem. 6) | `wg.Wait()` en `gradienteReduccion` | Nadie suma los gradientes parciales hasta que terminan todas las goroutines de la época. |
| Thread-safety (sem. 6) | `Indice.Agregar` (no es seguro) frente a `procesarFila` (sí lo es) | Diseño por **propiedad**: cada índice tiene una sola goroutine dueña, así que no necesita lock. Se verifica con `go test -race`. |
| Reducciones y paralelización de bucles (sem. 7, OpenMP) | `gradienteReduccion` | Equivale a `#pragma omp parallel for reduction(+:grad)`: bloques estáticos de ejemplos, acumuladores privados por goroutine y suma final. |
| Variables compartidas frente a privadas (sem. 7) | `gradienteMutex` / `gradienteAtomico` frente a `gradienteReduccion` | Es el mismo cálculo. Con un acumulador **compartido**, se pierde contra el secuencial. Con acumuladores **privados**, llega a 6,1× con 8 goroutines (Experimento 1b). |

## Unidades III y IV: tareas, balanceo, GPU y patrones de diseño

| Concepto (semana) | Dónde estará | Qué mostrar |
|---|---|---|
| Programación basada en tareas, planificación y balanceo (sem. 9) | `nativo/openmp/variables` | Una `task` por estación para calcular rezagos y medias móviles. Las estaciones tienen entre unas pocas y 8760 horas por gas, así que `schedule(static)` deja hilos ociosos y `dynamic`/`guided` los equilibra (Experimento 6). |
| Descomposición de datos y tareas; eficiencia (sem. 10) | `internal/particion`, cargador con agregadoras particionadas, Experimentos 6 y 7 | Partición por estación (datos) frente a partición por fase (tareas). |
| Arquitectura de GPU, modelo CUDA, kernels (sem. 11) | `nativo/cuda/gradiente --kernel atomic` | Un hilo por ejemplo y `atomicAdd` al gradiente. |
| Memoria global frente a compartida, reducción, warp shuffles (sem. 12) | `--kernel shared` y `--kernel shuffle` | Reducción en árbol en `__shared__` y luego con `__shfl_down_sync`, con su tiempo frente al kernel ingenuo. |
| Sorting paralelo y MapReduce en GPU (sem. 13) | `nativo/cuda/estaciones` | `thrust::sort_by_key` por (estación, hora) y `reduce_by_key` para estadísticas por estación. |
| Evaluación empírica de la aceleración (sem. 13) | `experimentos/cmd/analisis` y los 8 experimentos | El mismo análisis (mediana, S(p), E(p), Karp-Flatt, Amdahl) para todas las tecnologías. |
| Espacios de diseño: buscar concurrencia, estructura del algoritmo, estructuras de soporte (sem. 13-15) | Informe final | Dónde está la concurrencia en SACA (por fila, por estación, por ejemplo), qué estructura se eligió (descomposición geométrica, *pipeline*, maestro-trabajador) y qué estructuras de soporte (SPMD en MPI, fork-join en OpenMP, colas compartidas en Pthreads). |

## Estado de cada tecnología del sílabo

| Tecnología | Semanas | Versión en Go | Núcleo nativo | Experimento |
|---|---|---|---|---|
| MPI | 3-4 | ✅ maestro TCP con registro de nodos | pendiente (semana 8): `nativo/mpi/entrenamiento` | 7 |
| Pthreads | 5-6 | ✅ cargador concurrente | pendiente (semana 8): `nativo/pthreads/cargador` | 5 |
| OpenMP | 7, 9-10 | ✅ `gradienteReduccion` | pendiente (semanas 8-9): `nativo/openmp/entrenamiento` y `variables` | 6 |
| CUDA | 11-13 | — | pendiente (semanas 11-13); se compila y mide en las PCs del grupo con NVIDIA | 8 |

## Hallazgos para defender oralmente

1. **La agregadora única era la fracción serial.** Con 8 trabajadoras, 16 de los 27 s de procesamiento se iban en indexar. Particionar el índice por hash de estación en K agregadoras, cada una dueña de su parte y sin locks, bajó el procesamiento a 11,5 s. Es la ley de Amdahl en acción: no sirve agregar trabajadoras si la parte serial no se reduce.
2. **La escritura en Mongo era la siguiente parte serial.** Con un escritor por índice, pasó de 83 s a 26,5 s.
3. **Grano de la sincronización.** Un mutex o un CAS **por ejemplo** hacen que el entrenamiento con 8 goroutines sea 2× más lento que el secuencial (eficiencia del 6-7 %). Acumular en privado y reducir una vez por época da S(8)=6,1. La lección es sincronizar lo menos posible, no solo "usar un lock".
4. **Hyper-threading.** Con 12 goroutines el entrenamiento rinde menos que con 8: la máquina de pruebas (AMD Ryzen 5 3600) tiene 12 hilos lógicos pero **6 núcleos físicos**. Más allá de 6-8 goroutines, los hilos hermanos compiten por las mismas unidades de punto flotante, y el reparto de bloques más pequeños aumenta el peso de la barrera por época.
5. **Memoria.** El cargador original retenía cada línea del CSV en el índice, porque las subcadenas de Go comparten el arreglo de la línea completa. Internar las cadenas bajó el pico de 8,7 GB a 4,4 GB.
