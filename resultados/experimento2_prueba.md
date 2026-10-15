## Experimento 2 (muestra de prueba) — Entrenamiento distribuido por TCP con la muestra de prueba (146 mil ejemplos): local y por Tailscale

Máquina: 12 CPUs lógicas. T(1) = 2.663 s (modo secuencial, mediana).
Fracción serial estimada con Karp-Flatt en el mayor p: f = 0.316 → speedup máximo de Amdahl = 3.16.

### tcp_local

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 2.574 | 1.03 | 103% | — | 1.00 |
| 2 | 3 | 1.653 | 1.61 | 81% | 0.241 | 1.52 |
| 4 | 3 | 1.367 | 1.95 | 49% | 0.351 | 2.05 |
| 8 | 3 | 1.069 | 2.49 | 31% | 0.316 | 2.49 |

### tcp_tailscale

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 2.055 | 1.30 | 130% | — | 1.00 |
