## Experimento 2 — Entrenamiento distribuido por TCP entre PCs del grupo por Tailscale, muestra completa (2,5 M ejemplos)

Máquina: 12 CPUs lógicas. T(1) = 41.451 s (modo secuencial, mediana).
Fracción serial estimada con Karp-Flatt en el mayor p: f = 0.000 → speedup máximo de Amdahl = 1000000.00.

### tcp_tailscale

| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 2.277 | 18.21 | 1821% | — | 1.00 |
| 2 | 3 | 3.877 | 10.69 | 535% | -0.813 | 2.00 |
| 3 | 3 | 6.854 | 6.05 | 202% | -0.252 | 3.00 |
