## Experimento 1 — Cargador: ingesta de los 4 CSV horarios de la EPA

Máquina: 12 CPUs lógicas. T(1) = 132.396 s (modo secuencial, mediana).
Fracción serial medida en T(1) (t_indexado_s + t_mongo_s): f = 0.797 → speedup máximo de Amdahl = 1.25.

### concurrente

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 143.987 | 0.92 | 92% | — | 1.00 |
| 2 | 3 | 117.351 | 1.13 | 56% | 0.773 | 1.11 |
| 4 | 3 | 121.264 | 1.09 | 27% | 0.888 | 1.18 |
| 8 | 3 | 123.071 | 1.08 | 13% | 0.920 | 1.22 |
| 12 | 3 | 124.491 | 1.06 | 9% | 0.935 | 1.23 |
| 16 | 3 | 128.099 | 1.03 | 6% | 0.965 | 1.24 |

### concurrente_particionado

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 140.455 | 0.94 | 94% | — | 1.00 |
| 2 | 3 | 86.116 | 1.54 | 77% | 0.301 | 1.11 |
| 4 | 3 | 57.567 | 2.30 | 57% | 0.246 | 1.18 |
| 8 | 3 | 41.304 | 3.21 | 40% | 0.214 | 1.22 |
| 12 | 3 | 37.335 | 3.55 | 30% | 0.217 | 1.23 |
| 16 | 3 | 35.094 | 3.77 | 24% | 0.216 | 1.24 |
