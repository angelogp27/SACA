## Experimento 6 — Entrenamiento con OpenMP: reducción del gradiente con schedule static, dynamic y guided

Máquina: 16 CPUs lógicas. T(1) = 9.167 s (modo secuencial_c, mediana).
Fracción serial estimada con Karp-Flatt en el mayor p: f = 0.000 → speedup máximo de Amdahl = 1000000.00.

### openmp_static

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 4.425 | 2.07 | 207% | — | 1.00 |
| 2 | 1 | 2.230 | 4.11 | 205% | -0.513 | 2.00 |
| 4 | 1 | 1.137 | 8.06 | 202% | -0.168 | 4.00 |
| 8 | 1 | 0.800 | 11.45 | 143% | -0.043 | 8.00 |

### openmp_dynamic

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 4.399 | 2.08 | 208% | — | 1.00 |
| 2 | 1 | 2.218 | 4.13 | 207% | -0.516 | 2.00 |
| 4 | 1 | 1.130 | 8.11 | 203% | -0.169 | 4.00 |
| 8 | 1 | 0.783 | 11.70 | 146% | -0.045 | 8.00 |

### openmp_guided

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 4.422 | 2.07 | 207% | — | 1.00 |
| 2 | 1 | 2.228 | 4.11 | 206% | -0.514 | 2.00 |
| 4 | 1 | 1.145 | 8.00 | 200% | -0.167 | 4.00 |
| 8 | 1 | 0.824 | 11.12 | 139% | -0.040 | 8.00 |
