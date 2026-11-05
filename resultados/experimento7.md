## Experimento 7 — Entrenamiento con MPI: MPI_Allreduce frente a Send/Recv + Bcast

Máquina: 16 CPUs lógicas. T(1) = 4.374 s (modo secuencial_c, mediana).
Fracción serial estimada con Karp-Flatt en el mayor p: f = 0.167 → speedup máximo de Amdahl = 6.00.

### mpi_colectivo

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 4.489 | 0.97 | 97% | — | 1.00 |
| 2 | 1 | 2.309 | 1.89 | 95% | 0.056 | 1.71 |
| 4 | 1 | 1.640 | 2.67 | 67% | 0.167 | 2.67 |

### mpi_p2p

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 4.474 | 0.98 | 98% | — | 1.00 |
| 2 | 1 | 3.854 | 1.13 | 57% | 0.762 | 1.71 |
| 4 | 1 | 2.485 | 1.76 | 44% | 0.424 | 2.67 |
