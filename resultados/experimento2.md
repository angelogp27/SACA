## Experimento 2 — Entrenamiento distribuido por TCP entre las PCs del grupo, por combinación de nodos

Máquina: 12 CPUs lógicas. T(1) = 43.959 s (modo secuencial, mediana).
Fracción serial estimada con Karp-Flatt en el mayor p: f = 0.000 → speedup máximo de Amdahl = 1000000.00.

### tcp_tailscale:augusto-pc+josue

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 2 | 1 | 5.119 | 8.59 | 429% | -0.767 | 2.00 |

### tcp_tailscale:angelo+augusto-pc+josue

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 3 | 1 | 8.373 | 5.25 | 175% | -0.214 | 3.00 |
