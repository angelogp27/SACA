## Experimento 1b — Entrenamiento: gradiente de la regresión logística en paralelo

Máquina: 12 CPUs lógicas. T(1) = 2.396 s (modo secuencial, mediana).
Fracción serial estimada con Karp-Flatt en el mayor p: f = 0.113 → speedup máximo de Amdahl = 8.86.

### paralelo_reduccion

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 2.393 | 1.00 | 100% | — | 1.00 |
| 2 | 3 | 1.204 | 1.99 | 99% | 0.005 | 1.80 |
| 4 | 3 | 0.662 | 3.62 | 90% | 0.035 | 2.99 |
| 8 | 3 | 0.392 | 6.11 | 76% | 0.044 | 4.47 |
| 12 | 3 | 0.448 | 5.35 | 45% | 0.113 | 5.35 |

### paralelo_mutex

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 2.376 | 1.01 | 101% | — | 1.00 |
| 2 | 3 | 2.058 | 1.16 | 58% | 0.718 | 1.80 |
| 4 | 3 | 3.799 | 0.63 | 16% | 1.781 | 2.99 |
| 8 | 3 | 5.214 | 0.46 | 6% | 2.344 | 4.47 |
| 12 | 3 | 5.254 | 0.46 | 4% | 2.301 | 5.35 |

### paralelo_atomico

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 2.406 | 1.00 | 100% | — | 1.00 |
| 2 | 3 | 3.414 | 0.70 | 35% | 1.850 | 1.80 |
| 4 | 3 | 3.883 | 0.62 | 15% | 1.828 | 2.99 |
| 8 | 3 | 4.192 | 0.57 | 7% | 1.857 | 4.47 |
| 12 | 3 | 4.101 | 0.58 | 5% | 1.776 | 5.35 |
