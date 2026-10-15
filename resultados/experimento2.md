## Experimento 2 — Entrenamiento distribuido por TCP entre PCs del grupo por Tailscale, muestra completa (2,5 M ejemplos)

Máquina: 12 CPUs lógicas. T(1) = 41.451 s (modo secuencial, mediana).
Fracción serial estimada con Karp-Flatt en el mayor p: f = 0.000 → speedup máximo de Amdahl = 1000000.00.

### tcp_tailscale

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 5 | 2.277 | 18.21 | 1821% | — | 1.00 |
| 2 | 5 | 3.923 | 10.57 | 528% | -0.811 | 2.00 |
| 3 | 4 | 7.286 | 5.69 | 190% | -0.236 | 3.00 |
