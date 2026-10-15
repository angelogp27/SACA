## Experimento 2 — Entrenamiento distribuido por TCP entre las PCs del grupo, por combinación de nodos

Máquina: 12 CPUs lógicas. T(1) = 46.805 s (modo secuencial, mediana).
Fracción serial estimada con Karp-Flatt en el mayor p: f = 0.000 → speedup máximo de Amdahl = 1000000.00.

### tcp_tailscale:augusto-pc

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 2.568 | 18.22 | 1822% | — | 1.00 |

### tcp_tailscale:josue

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 4.609 | 10.16 | 1016% | — | 1.00 |

### tcp_tailscale:augusto-pc+josue

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 2 | 1 | 5.119 | 9.14 | 457% | -0.781 | 2.00 |

### tcp_tailscale:angelo+augusto-pc+josue

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 3 | 3 | 7.846 | 5.97 | 199% | -0.249 | 3.00 |
