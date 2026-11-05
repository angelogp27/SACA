## Experimento 5 — Cargador: Pthreads (C) frente a goroutines (Go), sin MongoDB

Máquina: 16 CPUs lógicas. T(1) = 123.579 s (modo secuencial, mediana).
Fracción serial medida en T(1) (t_indexado_s): f = 0.341 → speedup máximo de Amdahl = 2.93.

### pthreads_particionado

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 15.745 | 7.85 | 785% | — | 1.00 |
| 2 | 3 | 8.398 | 14.72 | 736% | -0.864 | 1.49 |
| 4 | 3 | 5.156 | 23.97 | 599% | -0.278 | 1.98 |
| 8 | 3 | 3.926 | 31.48 | 393% | -0.107 | 2.36 |
| 12 | 3 | 4.043 | 30.57 | 255% | -0.055 | 2.52 |
| 16 | 3 | 4.095 | 30.18 | 189% | -0.031 | 2.61 |

### concurrente_particionado

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 98.162 | 1.26 | 126% | — | 1.00 |
| 2 | 3 | 67.721 | 1.82 | 91% | 0.096 | 1.49 |
| 4 | 3 | 39.989 | 3.09 | 77% | 0.098 | 1.98 |
| 8 | 3 | 23.888 | 5.17 | 65% | 0.078 | 2.36 |
| 12 | 3 | 18.661 | 6.62 | 55% | 0.074 | 2.52 |
| 16 | 3 | 17.601 | 7.02 | 44% | 0.085 | 2.61 |
