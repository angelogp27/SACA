## Experimento 2 — Entrenamiento distribuido maestro-trabajador por TCP (procesos en una PC)

Máquina: 12 CPUs lógicas. T(1) = 2.681 s (modo secuencial, mediana).
Fracción serial estimada con Karp-Flatt en el mayor p: f = 0.313 → speedup máximo de Amdahl = 3.19.

### tcp_local

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 2.574 | 1.04 | 104% | — | 1.00 |
| 2 | 3 | 1.653 | 1.62 | 81% | 0.233 | 1.52 |
| 4 | 3 | 1.367 | 1.96 | 49% | 0.346 | 2.06 |
| 8 | 3 | 1.069 | 2.51 | 31% | 0.313 | 2.51 |
