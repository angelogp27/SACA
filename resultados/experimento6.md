## Experimento 6 — Entrenamiento con OpenMP: reducción del gradiente con schedule static, dynamic y guided

Máquina: 16 CPUs lógicas. T(1) = 7.726 s (modo secuencial_c, mediana).
Fracción serial estimada con Karp-Flatt en el mayor p: f = 0.000 → speedup máximo de Amdahl = 1000000.00.

### openmp_static

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 4.446 | 1.74 | 174% | — | 1.00 |
| 2 | 1 | 2.240 | 3.45 | 172% | -0.420 | 2.00 |
| 4 | 1 | 1.281 | 6.03 | 151% | -0.112 | 4.00 |
| 8 | 1 | 0.909 | 8.50 | 106% | -0.008 | 8.00 |

### openmp_dynamic

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 4.425 | 1.75 | 175% | — | 1.00 |
| 2 | 1 | 2.237 | 3.45 | 173% | -0.421 | 2.00 |
| 4 | 1 | 1.259 | 6.14 | 153% | -0.116 | 4.00 |
| 8 | 1 | 0.896 | 8.62 | 108% | -0.010 | 8.00 |

### openmp_guided

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 4.446 | 1.74 | 174% | — | 1.00 |
| 2 | 1 | 2.243 | 3.45 | 172% | -0.419 | 2.00 |
| 4 | 1 | 1.265 | 6.11 | 153% | -0.115 | 4.00 |
| 8 | 1 | 0.897 | 8.61 | 108% | -0.010 | 8.00 |
