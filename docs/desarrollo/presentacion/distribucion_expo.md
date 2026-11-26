# Distribución de la exposición

8 integrantes, unos 3 minutos cada uno (~25 min en total).

## Reglas

- **Máximo 3 diapositivas por integrante.** Lo demás se muestra en el programa.
- **Alternar diapositivas y programa:** primero la idea en la diapositiva y después se muestra en SACA (página o `saca-top`). Al cambiar de turno, dejar abierta la pantalla que sigue.
- **Una sola persona maneja la laptop del maestro** toda la expo; los demás hablan.
- `saca-top` abierto en una terminal al costado durante toda la expo (CPU, RAM y GPU de cada laptop).
- Todo hasta el integrante 5 va **sin CUDA**; CUDA se explica aparte (integrantes 6 y 7).

## Por integrante

| # | Tema | Diapositivas (máx. 3) | En el programa | Concepto del curso |
|---|---|---|---|---|
| 1 | Introducción y clúster | 1. Problema y datos · 2. Arquitectura maestro-nodos · 3. Recorrido y tecnologías | *Iniciar clúster* / *Unirse*; abrir `saca-top` | Sistema distribuido, maestro-trabajador, memoria compartida vs. distribuida |
| 2 | Limpieza e ing. de features | 1. Diagrama de flujo (qué es paralelo y qué no) · 2. Pthreads vs. Go | Pantalla de Limpieza (gráficas de CPU) y `saca-top` con los núcleos llenos | Paralelismo de datos con hilos, Amdahl (la parte secuencial limita) |
| 3 | Entrenamiento: qué y cómo se reparte | 1. Red neuronal, épocas y mini-lotes · 2. Reparto dinámico / capacidad / iguales | *Nuevo → Entrenamiento*: elegir opciones e iniciar | Descomposición de datos, balanceo de carga |
| 4 | Entrenamiento: comunicación y rendimiento | 1. Árbol, anillo, maestro y MPI Allreduce · 2. OpenMP vs. Go · 3. Secuencial, speedup y eficiencia | Resultados del entrenamiento: tabla de motores, topología y entrenamiento secuencial | Reducción colectiva, sincronización, speedup y eficiencia (E = S/p) |
| 5 | Predicción distribuida | 1. Entrenar vs. predecir · 2. TCP vs. MPI Gatherv · 3. La red como cuello de botella | *Nuevo → Predicción*: comparar OpenMP y Go | Gather vs. Allreduce, cómputo vs. comunicación |
| 6 | CUDA: cómo funciona | 1. CPU + GPU cooperando · 2. Reparto y respaldo | — (teoría) | SIMT, procesamiento heterogéneo |
| 7 | CUDA: demostración | 1. Resultados con y sin CUDA | Entrenamiento y predicción con *Cooperar con CUDA*; GPU en `saca-top` | Speedup con GPU |
| 8 | Resultados finales | 1. Mapa y alerta · 2. Rendimiento (Amdahl, Karp-Flatt) · 3. Conclusiones | *Resultados finales*: mapa 3D, NO2 y probabilidad, últimos procesos | Amdahl, Karp-Flatt, eficiencia |

## Antes de la expo

1. Probar CUDA en la laptop con NVIDIA (aún no se ha probado con una GPU real).
2. Todas las laptops por cable (`192.168.50.x`), mismo commit, `saca.sh` / `saca.ps1`.
3. Dejar en el historial una corrida completa de cada fase; en vivo, corridas cortas (p. ej. 5 épocas).
4. Grabar un video de respaldo de cada fase.
5. Ensayar los cambios de turno con la presentación y el programa abiertos.
