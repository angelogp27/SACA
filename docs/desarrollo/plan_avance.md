# Plan de avance: aprovechar el clúster

**Estado:** las 5 fases implementadas y probadas, más el reparto dinámico (12-nov-2026). Primeras mediciones en el switch con 2 laptops: ver [Resultados medidos](#resultados-medidos-en-el-switch-12-nov-2026). Lo que sigue está en [Pendientes](#pendientes).

## Por qué

Hoy el clúster mueve muy poco por la red: el modelo es una regresión logística de 13 números (12 pesos y el sesgo), y en cada época viajan ~450 bytes por nodo. Las 8 laptops y el switch de 100 Mbps casi no se usan. El plan agranda el problema **donde el enunciado lo pide** (más variables), cambia el modelo por uno que justifica la red (más parámetros) y reparte el trabajo y la comunicación entre todas las laptops.

Cada fase tiene **su propia pantalla** en la página, en el orden en que ocurre, y sus mediciones para el informe.

## El flujo, pantalla por pantalla

| # | Pantalla | Qué se ve | Fase del plan |
|---|---|---|---|
| 1 | Inicio | Iniciar / Unirse al clúster | (existe) |
| 2 | Clúster iniciado | Nodos con su hardware; Iniciar o Saltar limpieza | (existe) |
| 3 | **Limpieza y variables** | Por nodo y tecnología (Pthreads, Go), una gráfica de % de CPU con las dos fases seguidas y el tiempo de cada una: **leer, validar y unir** los CSV, y **calcular rezagos, medias móviles, etiqueta y normalización** y escribir `muestra.bin`; huellas | 1 |
| 4 | Antes de entrenar | Modelo (con su video), nodos dibujados con la comunicación elegida; **reparto dinámico, según la capacidad o en partes iguales** | 3 y 4 |
| 5 | **Entrenamiento** | Épocas, pérdida, cálculo frente a red; **topología de la comunicación** (maestro central, árbol, anillo) y % de filas de cada nodo | 2, 3 y 4 |
| 6 | **Predicción distribuida** | Cada nodo predice sus estaciones; el maestro junta los resultados y actualiza el mapa | 5 |
| — | **Nuevo / Resultados** (desde el 13-nov) | Arriba de todo: Nuevo inicia cada fase; Resultados la muestra en vivo, con su historial y su CSV; *Resultados finales* reemplaza al Tablero | — |

## Fases

### Fase 1. Variables del enunciado y pantallas separadas

- **Variables nuevas** (enunciado, §4 de `.context/proyecto9.md`): NO2 en t-1, t-2, t-3, t-6, t-12 y t-24 h; media y máximo del NO2 de las 24 h anteriores. Pasan de 12 a **20** variables.
- **Horas faltantes:** si la estación no midió NO2 en t-k, se usa el valor de t (persistencia). La media y el máximo usan solo las horas medidas; si no hay ninguna, el valor de t. Es una regla determinista: Go y Pthreads la aplican igual.
- Se calculan **en el mismo programa** que limpia (los datos limpios ya están en memoria y ordenados), pero se **miden y se muestran aparte**: el cargador marca el inicio de la fase y el nodo informa dos tiempos.
- Go y Pthreads deben seguir generando **el mismo `muestra.bin`** (misma huella).
- Una sola pantalla (3) con las dos fases seguidas en cada gráfica: *limpieza de datos* e *ingeniería de features*, cada una con su tiempo (al principio eran dos pantallas; se unieron porque la gráfica parecía reiniciarse al pasar de una a otra).
- **Medición:** tiempo de limpieza y de preparación por tecnología y por nodo; mejora del modelo (AUC, F1) con 20 variables frente a 12.

### Fase 2. Red neuronal pequeña con mini-lotes (Go)

- Perceptrón multicapa, por ejemplo 20 → 64 → 32 → 1 (~3 500 parámetros), entrenado por mini-lotes: muchas actualizaciones por época.
- Cada intercambio pasa de ~450 bytes a decenas de KB; por época, de 1 a cientos de intercambios. Los mensajes de pesos y gradientes pasan a formato binario (no JSON).
- La regresión logística queda como opción (y los núcleos de C y CUDA siguen con ella como experimentos aparte).
- **Medición:** bytes por época, tiempo de red frente a cálculo, AUC/F1 frente a la regresión logística.

### Fase 3. Reparto según la capacidad de cada nodo (desde la página)

- Ya existe por línea de comandos (`-reparto proporcional`, Experimento 9). Se lleva a la pantalla 5: el maestro propone los pesos con lo medido (la limpieza y la corrida anterior) y se pueden ajustar.
- El reparto proporcional cuenta **filas** por cubeta (no cubetas: son 1024 para ~436 estaciones).
- **Reparto dinámico** (solo la red): al final de cada época el maestro mide filas/ms de cada nodo y les manda su nueva parte (mensaje `reparto`). Por defecto en la página.
- **Medición:** tiempo por época con reparto igual, proporcional y dinámico en el clúster heterogéneo.

### Fase 4. Comunicación entre nodos: árbol y anillo

- Hoy todo pasa por el maestro (7 envíos seguidos por su único puerto).
- **Reducción en árbol:** con 8 laptops, 3 niveles; en cada nivel, los pares se comunican a la vez por sus propios puertos. Luego difusión en árbol.
- **Anillo (*ring allreduce*), si da el tiempo:** todas las laptops transmiten a la vez.
- Los nodos hablan **entre ellos**: cada laptop abre un puerto más (TCP 9002).
- **Medición:** tiempo de comunicación por época con maestro central, árbol y anillo, por switch y por Tailscale.

### Fase 5. Predicción distribuida

- Con el modelo entrenado, cada nodo predice la probabilidad de alerta para **todas** las horas de sus estaciones y se la envía al maestro (*gather*, decenas de MB), que regenera el mapa. Reemplaza la regeneración desde MongoDB.
- **Medición:** tiempo y bytes del *gather*.

## Resultados medidos en el switch (12-nov-2026)

Dos laptops por el switch de 100 Mbps: **fedora** (maestro y nodo; Ryzen 7 7735HS, 16 hilos, 13 GB) y **usuario** (HP; Intel, 4 hilos, 8 GB, Windows). Red neuronal 20→64→32→1, 20 épocas. Datos en `resultados/cluster.csv` y `resultados/transferencias.csv` de la laptop del maestro.

**Tráfico por la red**

| Qué | MB |
|---|---|
| Entrenamiento, por época (2 nodos) | 1,3 con 25 mini-lotes · 5,3 con 100 · 10,6 con 200 |
| Predicción distribuida (3 342 599 filas) | 63,8 (10,8–12,3 s con fedora + HP; 5,0 s con fedora sola) |

**Tiempo por época** (la fila *secuencial* mide 1 época con 1 hilo; las demás, el promedio de 20)

| Corrida | Mini-lotes | Por época | Cálculo | Red y espera | Total |
|---|---|---|---|---|---|
| Secuencial (1 hilo) | — | ~17–19 s | — | — | ~6 min (estimado) |
| fedora sola | 200 | 2,88 s | 2,75 s | 0,12 s | 57,6 s |
| fedora sola | 25 | **2,67 s** | 2,62 s | 0,05 s | **53,4 s** |
| fedora + HP, reparto fijo, árbol | 25 | 2,83–2,93 s | 2,65–2,76 s | 0,17 s | 56,6–58,7 s |
| fedora + HP, reparto fijo, árbol | 200 | 3,76–4,06 s | 2,87–3,23 s | 0,84–0,88 s | 75,1–81,3 s |
| fedora + HP, reparto fijo, anillo | 200 | 3,76 s | 2,95 s | 0,81 s | 75,2 s |
| fedora + HP, **dinámico**, árbol | 200 | 4,86 s | 3,66 s | 1,20 s | 97,2 s |
| fedora + HP, **dinámico**, árbol | 25 | 3,34 s | 3,11 s | 0,23 s | 66,8 s |

**Conclusiones**

1. **Frente al secuencial, todo gana** (5–6,4×), pero casi todo viene de los 16 hilos de fedora, no de sumar laptops.
2. **fedora + HP es más lento que fedora sola** (0,77–0,80×). La HP aporta como mucho ~21 % de la capacidad de fedora (techo teórico 1,21×), y la comunicación más la barrera de cada mini-lote cuestan más que eso.
3. **El reparto dinámico funciona:** balancea en una época (50/50 → 79/21; con 200 mini-lotes los dos nodos calculan 3,2 s), pero no puede superar ese techo.
4. **Más mini-lotes, más comunicación:** de 25 a 200, la red y espera sube de ~0,2 s a ~1 s por época (de ~5 % a ~25 %). La pérdida mejora de 25 a 100 mini-lotes (0,041 → 0,031) y ya no de 100 a 200.
5. **Con 2 nodos, árbol y anillo empatan** (hacen el mismo intercambio); la diferencia debería aparecer con 4 o más laptops.
6. **Pendiente de explicar:** en el clúster, fedora calcula su ~79 % en ~3,1–3,2 s, más que su 100 % cuando está sola (2,6–2,75 s). Hipótesis: el rezagado de cada mini-lote (la época suma el nodo más lento de *cada* paso, y la HP es irregular) y que fedora corre a la vez el maestro, su nodo y la página.

## Pendientes

- **Medir con laptops parecidas** (4 a 8): es donde el clúster debería mostrar speedup real y donde árbol, anillo y *por el maestro* se diferencian. Por switch y por Tailscale.
- **Explicar la conclusión 6:** guardar por paso el cálculo de cada nodo (o un resumen del rezagado) y medir fedora como nodo sin la página abierta.
- **Calidad del modelo:** calibrar el umbral de alerta (hoy 0,5 da F1 bajo y ~la mitad de las estaciones en alerta) y compararlo con la regla "la próxima hora igual que ahora".
- **HP:** comprobar que el cargador de Pthreads compila con el gcc de 64 bits (MSYS2 UCRT64) y que su limpieza termina.
- **Opcional:** más datos (otro año) para subir el cálculo por mini-lote frente a la comunicación (ley de Gustafson).

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
| 12-nov-2026 | — | Primeras mediciones en el switch con 2 laptops (fedora + HP): ver [Resultados medidos](#resultados-medidos-en-el-switch-12-nov-2026). Nuevo `resultados/transferencias.csv` (MB por la red de cada entrenamiento de la red y de cada predicción, con el reparto). Pantalla *Antes de entrenar*: la comunicación elegida se dibuja con los nodos (el maestro se repite al centro en *todo por el maestro*), botón ▶ con el video de cada modelo |
| 13-nov-2026 | — | Página reorganizada: **Nuevo** (paneles para iniciar cada fase; sin botones en las laptops que no son el maestro) / **Resultados** (la fase en vivo y su historial) con la línea de fases arriba. El maestro guarda cada limpieza, entrenamiento y predicción terminados en `resultados/historial/` (`GET /api/historial`); desde la página se vuelve a ver cualquiera y se baja su CSV. Entrenamiento: un cuadro por época (más oscuro = menor pérdida) y nodos de tamaño según su parte de las filas. *Resultados finales* reemplaza al Tablero (mapa, línea de tiempo y el último proceso de cada fase); se quitaron el panel *Clúster en vivo* y los paneles de experimentos antiguos |
