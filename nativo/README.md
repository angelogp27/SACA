# Núcleos nativos (C, C++ y CUDA)

Las mismas partes del sistema que ya resuelve Go, programadas con las tecnologías del laboratorio. Cada núcleo lee los datos del contrato [docs/contrato_formato_binario.md](../docs/contrato_formato_binario.md), se valida contra la referencia de Go y deja una fila por corrida en un CSV que procesa `experimentos/cmd/analisis`.

| Carpeta | Tecnología | Qué hace | Se compara con |
|---|---|---|---|
| `pthreads/cargador` | Pthreads (sem. 5-6) | limpia los 4 CSV de la EPA: bloques, cola productor-consumidor, mutex y agregadoras dueñas de su índice | `cargador/cmd/concurrente -sin-mongo` (mismos conteos de filas, registros y duplicados) |
| `openmp/entrenamiento` | OpenMP (sem. 7, 9-10) | gradiente con `reduction` y `schedule` static/dynamic/guided | `logistica.Entrenar` (misma pérdida, 1e-6) |
| `mpi/entrenamiento` | MPI (sem. 3-4) | `Scatterv` de la muestra, `Allreduce` o `Send`/`Recv` + `Bcast` del gradiente | ídem |
| `cuda/gradiente` | CUDA (sem. 11-12) | kernels `atomic`, `shared` y `shuffle` | ídem, y el gradiente de la CPU en la época 0 |
| `cuda/gradiente_servidor.cu` | CUDA | motor del trabajador Go del clúster | ver [cuda/README.md](cuda/README.md) |
| `comun/saca.h` | – | lectura de `muestra.bin`, F1/AUC, verificación y CSV | – |

```sh
go run ./herramientas/cmd/exportar-muestra   # data/muestra.bin y data/muestra_referencia.json
sh scripts/nativo.sh compilar                # gcc, mpicc y nvcc si están instalados
sh scripts/nativo.sh exp6                    # OpenMP; exp5 Pthreads, exp7 MPI, exp8 CUDA
```

Requisitos: `gcc` con OpenMP y Pthreads; `mpicc`/`mpiexec` (paquete `openmpi-devel` o `libopenmpi-dev`) para MPI; `nvcc` y una GPU NVIDIA para CUDA.

Al terminar, el programa avisa si la pérdida coincide con la de Go (`CORRECTO`) y sale con código 1 si no. Un núcleo que no coincide no se mide.

## Verlos en la página

El panel **Las tecnologías en C** (`frontend/src/components/PanelTecnologias.jsx`) dibuja los resultados reales de los experimentos 5 a 8: una pestaña por tecnología, con el speedup frente al ideal y la tabla de tiempos (Pthreads frente a Go en el 5; OpenMP, MPI y CUDA frente a la línea base secuencial). Cada `sh scripts/nativo.sh expN` deja su `frontend/public/datos/experimentoN.json` al terminar, y la página lo vuelve a pedir cada 5 s, así que la corrida aparece sola (con `npm run dev`; un `npm run build` ya hecho solo ve los JSON de ese momento).

No hay datos de ejemplo: si todavía no existe el JSON de una pestaña, esta dice qué comando correr. En el Experimento 8 el nombre del equipo (`MAQUINA=angelo sh scripts/nativo.sh exp8`) agrupa las filas de OpenMP y de CUDA de la misma PC.
