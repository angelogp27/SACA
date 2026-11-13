# Plan de avance: aprovechar el clúster

**Estado:** las 5 fases implementadas y probadas (12-nov-2026). Falta medir en el clúster real (switch y Tailscale) para el informe.

## Por qué

Hoy el clúster mueve muy poco por la red: el modelo es una regresión logística de 13 números (12 pesos y el sesgo), y en cada época viajan ~450 bytes por nodo. Las 8 laptops y el switch de 100 Mbps casi no se usan. El plan agranda el problema **donde el enunciado lo pide** (más variables), cambia el modelo por uno que justifica la red (más parámetros) y reparte el trabajo y la comunicación entre todas las laptops.

Cada fase tiene **su propia pantalla** en la página, en el orden en que ocurre, y sus mediciones para el informe.

## El flujo, pantalla por pantalla

| # | Pantalla | Qué se ve | Fase del plan |
|---|---|---|---|
| 1 | Inicio | Iniciar / Unirse al clúster | (existe) |
| 2 | Clúster iniciado | Nodos con su hardware; Iniciar o Saltar limpieza | (existe) |
| 3 | **Limpieza y variables** | Por nodo y tecnología (Pthreads, Go), una gráfica de % de CPU con las dos fases seguidas y el tiempo de cada una: **leer, validar y unir** los CSV, y **calcular rezagos, medias móviles, etiqueta y normalización** y escribir `muestra.bin`; huellas | 1 |
| 5 | Antes de entrenar | Elegir nodos; **reparto según la capacidad de cada uno** | 3 |
| 6 | **Entrenamiento** | Épocas, pérdida, cálculo frente a red; **topología de la comunicación** (maestro central, árbol, anillo) | 2 y 4 |
| 7 | **Predicción distribuida** | Cada nodo predice sus estaciones; el maestro junta los resultados y actualiza el mapa | 5 |

## Fases

### Fase 1. Variables del enunciado y pantallas separadas

- **Variables nuevas** (enunciado, §4 de `.context/proyecto9.md`): NO2 en t-1, t-2, t-3, t-6, t-12 y t-24 h; media y máximo del NO2 de las 24 h anteriores. Pasan de 12 a **20** variables.
- **Horas faltantes:** si la estación no midió NO2 en t-k, se usa el valor de t (persistencia). La media y el máximo usan solo las horas medidas; si no hay ninguna, el valor de t. Es una regla determinista: Go y Pthreads la aplican igual.
- Se calculan **en el mismo programa** que limpia (los datos limpios ya están en memoria y ordenados), pero se **miden y se muestran aparte**: el cargador marca el inicio de la fase y el nodo informa dos tiempos.
- Go y Pthreads deben seguir generando **el mismo `muestra.bin`** (misma huella).
- Pantallas 3 y 4 separadas.
- **Medición:** tiempo de limpieza y de preparación por tecnología y por nodo; mejora del modelo (AUC, F1) con 20 variables frente a 12.

### Fase 2. Red neuronal pequeña con mini-lotes (Go)

- Perceptrón multicapa, por ejemplo 20 → 64 → 32 → 1 (~3 500 parámetros), entrenado por mini-lotes: muchas actualizaciones por época.
- Cada intercambio pasa de ~450 bytes a decenas de KB; por época, de 1 a cientos de intercambios. Los mensajes de pesos y gradientes pasan a formato binario (no JSON).
- La regresión logística queda como opción (y los núcleos de C y CUDA siguen con ella como experimentos aparte).
- **Medición:** bytes por época, tiempo de red frente a cálculo, AUC/F1 frente a la regresión logística.

### Fase 3. Reparto según la capacidad de cada nodo (desde la página)

- Ya existe por línea de comandos (`-reparto proporcional`, Experimento 9). Se lleva a la pantalla 5: el maestro propone los pesos con lo medido (la limpieza y la corrida anterior) y se pueden ajustar.
- **Medición:** tiempo por época con reparto igual frente a proporcional en el clúster heterogéneo.

### Fase 4. Comunicación entre nodos: árbol y anillo

- Hoy todo pasa por el maestro (7 envíos seguidos por su único puerto).
- **Reducción en árbol:** con 8 laptops, 3 niveles; en cada nivel, los pares se comunican a la vez por sus propios puertos. Luego difusión en árbol.
- **Anillo (*ring allreduce*), si da el tiempo:** todas las laptops transmiten a la vez.
- Los nodos hablan **entre ellos**: cada laptop abre un puerto más (TCP 9002).
- **Medición:** tiempo de comunicación por época con maestro central, árbol y anillo, por switch y por Tailscale.

### Fase 5. Predicción distribuida

- Con el modelo entrenado, cada nodo predice la probabilidad de alerta para **todas** las horas de sus estaciones y se la envía al maestro (*gather*, decenas de MB), que regenera el mapa. Reemplaza la regeneración desde MongoDB.
- **Medición:** tiempo y bytes del *gather*.

## Registro de avance

| Fecha | Fase | Qué se hizo |
|---|---|---|
| 12-nov-2026 | — | Plan aprobado |
| 12-nov-2026 | 1 | 20 variables (8 de la serie del NO2) en Go y Pthreads, mismo `muestra.bin` (huella `be888f96fb78`) con cualquier número de hilos. El nodo mide limpieza y preparación por separado (`resultados/limpieza.csv`: `t_limpieza_s`, `t_variables_s`). Pantallas 3 (Limpieza) y 4 (Preparación de variables). En el Ryzen 7 7735HS: Pthreads 5,1 s + 5,5 s; Go 19,1 s + 11,2 s. La preparación usa poca CPU (15 % en C): ordenar, normalizar y escribir es secuencial (Amdahl). `exportar` pide también el día anterior para los rezagos. Referencia de los núcleos nativos regenerada (pérdida 0,133646) |
| 12-nov-2026 | 2 | Red neuronal 20→64→32→1 (3 457 parámetros, `internal/red`): gradiente por *backpropagation* con goroutines (reducción con casillas privadas, verificado contra derivadas numéricas) y Adam. Entrenamiento distribuido por mini-lotes (25 por época): los vectores viajan en binario. Con 2 nodos locales: **2,6 MB por la red por época** (antes ~1 KB), pérdida 0,1025 en 3 épocas (la logística, 0,1336 en 200), AUC 0,993; la primera época distribuida coincide con la secuencial (diferencia 1,7e-16). La página permite elegir el modelo, las épocas y los mini-lotes, y muestra los MB por época |
| 12-nov-2026 | 3 | El maestro mide la capacidad de cada nodo (ejemplos por ms de cálculo en el último entrenamiento; si falta, 1/tiempo de su limpieza con Go) y propone el reparto (`/api/estado`: `pesos_sugeridos`, 1 = el más lento). En la pantalla *Antes de entrenar* se elige *según la capacidad* o *partes iguales*, y cada tarjeta muestra su peso. Probado: la corrida con reparto proporcional coincide con el secuencial |
| 12-nov-2026 | 4 | `internal/pares`: los nodos se conectan entre ellos (TCP 9002) y suman el gradiente con **reducción en árbol** (binomial: log₂ n niveles, y difusión de vuelta) o **en anillo** (reduce-scatter + allgather); todos terminan con los mismos bits y cada nodo aplica Adam por su cuenta; el maestro solo da la señal de cada paso. Probado con 1 a 8 nodos y en el entrenamiento (árbol y anillo coinciden con el secuencial y con *por el maestro*). Pantalla del entrenamiento con el diagrama de la topología. 3 nodos en una misma laptop (sin red real): por el maestro ~3,3 s/época, árbol 3,34 s, anillo 3,24 s; la diferencia se mide en el switch |
| 12-nov-2026 | 5 | `muestra.bin` guarda la hora de cada fila (huella `c1f84fd1da78`, igual en Go y Pthreads). Al terminar un entrenamiento, *Predicción distribuida*: el maestro manda el modelo y reparte las estaciones; cada nodo predice todas sus horas con goroutines y devuelve trozos binarios de 20 bytes por fila (*gather*). Con 3 nodos: **3 342 599 filas, 63,8 MB en 4,2 s**. El maestro regenera sin MongoDB `series.json` (NO2 observado, idéntico al de MongoDB, y la probabilidad del modelo nuevo), `estaciones.json` (alerta del día) y `resumen.json`; el mapa tiene el modo *Alerta del modelo*. Pantalla propia con el avance, los MB y los tiempos de cada nodo |
| 12-nov-2026 | 1 | Correcciones tras probar con la HP (Windows, 8 GB): el lanzador ya no relee los 360 MB de `muestra.bin` cada 2 s para la huella (en Windows impedía reemplazarlo: "Access is denied"); Go y C reintentan el reemplazo. La preparación de variables en Go usa arreglos densos de NO2 por estación y suelta cada registro al usarlo: 3,8 → 2,6 GB vivos, 11,4 → 7,8 s, misma huella. El avance nunca retrocede. Limpieza y variables en una sola pantalla. El cargador C exige gcc de 64 bits en Windows (con 32 bits fallaba con "sin memoria para los registros") |
| 12-nov-2026 | 3 | **Reparto dinámico** (balanceo de carga dinámico, solo la red): al final de cada época el maestro mide filas/ms de cada nodo y les manda su nueva parte (mensaje `reparto`: solo la lista de cubetas, porque todos tienen la muestra completa). Además, el reparto proporcional ahora cuenta **filas** por cubeta (antes contaba cubetas: 1024 cubetas para 436 estaciones, muchas vacías, y la proporción real de filas se desviaba). Prueba con datos reales, un nodo de 2 hilos y otro de 8: época 0 a partes iguales 7,8 s (cálculo 6,6 s frente a 2,2 s); tras reajustar a 22,5 % / 77,5 %, 3,7 s por época (×2,1), con los dos nodos calculando ~3,5 s. El reparto queda en `cluster.csv` (sufijo `_dinamico` o `_capacidad` en el modo) y en `transferencias.csv` (columna `reparto`) |
