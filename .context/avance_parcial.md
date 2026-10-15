# Avance para el examen parcial — semanas 3 a 7 + valor agregado

Bitácora del trabajo hecho entre el 13 y el 14 de octubre de 2026 para entregar el avance del proyecto como parte del parcial. Registra qué se hizo, por qué, qué se encontró y qué queda pendiente. El cronograma de referencia está en [proyecto9.md](proyecto9.md) §10.

## Objetivo de la entrega

Cumplir todo lo que el cronograma pide hasta la semana 7, más dos valores agregados elegidos por el grupo:

1. **Entrenamiento paralelo con goroutines**: adelanta la Parte B sin depender de la red.
2. **Frontend con datos reales**: reemplaza los mocks donde ya hay datos.

## Estado frente al cronograma

| Sem. | Pedido | Estado | Dónde |
|---|---|---|---|
| 3 | 4 CSV descargados + conteo confirmado | ✅ 16 972 287 filas horarias, descarga reproducible con SHA-256 | `herramientas/cmd/descargar`, `data/manifiesto.json` |
| 3 | Contratos A↔B↔C↔D | ✅ Actualizados con la implementación real | `docs/contrato_*.md` |
| 4 | Cargador secuencial + T(1) | ✅ T(1) = 128 s con Mongo (37 s sin Mongo) | `cargador/cmd/secuencial` |
| 4 | Regresión logística secuencial + T(1) | ✅ 2,4 s (200 épocas, 147 mil ejemplos) | `entrenamiento/cmd/secuencial` |
| 4 | Features y etiqueta escritas; umbrales verificados | ✅ `dataset.NombresFeatures`, `UmbralNO2 = 53` | `internal/dataset` |
| 4 | API `/health` | ✅ (ya estaba) | `api/cmd/api` |
| 5 | Cargador concurrente por bloques | ✅ Más agregadoras particionadas (mejora propia) | `cargador/cmd/concurrente` |
| 6 | Escritura por lotes en Mongo, cierre ordenado de channels, `-race` | ✅ Escritores concurrentes; `go test -race ./...` en verde | `internal/persistencia`, tests |
| 6 | Particionar el dataset por nodo | ✅ Hash FNV de `estacion_id` | `internal/particion` |
| 6 | Mapa con metadatos de `annual_conc_by_monitor` | ✅ Nombre local, ciudad y CBSA reales | `herramientas/cmd/exportar`, `PanelMapa` |
| 7 | Experimento 1 (1..16 goroutines) + speedup + Amdahl | ✅ 3 repeticiones, mediana, Karp-Flatt | `scripts/experimento1.*`, `experimentos/cmd/analisis`, `docs/experimento1.md` |
| 7 | Maestro TCP que acepta y registra nodos | ✅ Más rechazo de sobrantes y detección de caídas | `entrenamiento/cmd/maestro`, `trabajador`, `internal/cluster`, `internal/protocolo` |
| — | **Valor agregado 1**: entrenamiento paralelo + Experimento 1b + métricas del modelo | ✅ | `internal/logistica/paralelo.go`, `entrenamiento/cmd/paralelo`, `internal/metricas` |
| — | **Valor agregado 2**: frontend con datos reales | ✅ Mapa, serie con probabilidad del modelo, panel de rendimiento real | `frontend/` |

## Decisiones tomadas (y por qué)

- **Descarga en Go, no en un script de shell.** Así funciona igual en Windows y Linux. Es idempotente y deja un manifiesto versionado con el SHA-256 de cada CSV, para que todos los integrantes verifiquen que tienen los mismos datos (`-verificar`). Si la EPA actualiza un archivo, la verificación lo detecta.
- **Unidades.** El contrato decía "convertir ppm↔ppb, nunca descartar", pero el código descartaba. Se alineó el código con el contrato. En 2025 todas las filas vienen en la unidad esperada (0 conversiones), pero la regla queda cubierta por tests.
- **Qualifiers.** Se excluyen los códigos de solicitud de exclusión por evento excepcional (prefijo `R` + letra, p. ej. `RS`, `RT`). Se verificó con los datos reales: en SO2 2025 aparecen `RS` (15 filas), informativos `I*` y de QA (`MD`, `SX`, `QX`, numéricos), que se conservan. En total se excluyen 1625 filas.
- **Agregadoras particionadas** (`-agregadoras K`, con `0` = tantas como trabajadoras). Se mantiene `-agregadoras 1` como el diseño original, para poder comparar en el Experimento 1. La partición por hash garantiza que cada clave (estación, hora) tenga una sola agregadora dueña, sin locks.
- **Escritores concurrentes a Mongo**, uno por índice. Con una agregadora la escritura sigue siendo serial, igual que en el diseño original.
- **Experimento 1 con dos series**: `concurrente` (1 agregadora) y `concurrente_particionado` (N agregadoras), ambas contra el mismo T(1) secuencial. La fracción serial de Amdahl se **mide** (indexado + Mongo del secuencial), no se supone.
- **Entrenamiento paralelo con tres estrategias**: reducción con casillas privadas, mutex por ejemplo y CAS por ejemplo. Las dos últimas existen a propósito, para mostrar el costo de la contención. Es un adelanto del Experimento 3 (mutex frente a atómicos) del cronograma.
- **Caché de la muestra** (`data/muestra.gob`). El Experimento 1b mide solo el cómputo, no la construcción de la muestra contra Mongo. La muestra se arma con `$match` (tiene NO2) + `$sample` y se etiqueta con consultas `$or` en lotes de 1000 (200 consultas en vez de 200 000 `FindOne`).
- **Partición temporal**: se entrena con ene-sep y se evalúa con oct-dic. El normalizador se ajusta solo con entrenamiento.
- **Modelo guardado en JSON** (`-modelo data/modelo.json`): pesos, sesgo y normalización. Lo usa el exportador del frontend y lo usará la API.
- **Frontend sin doble eje**: concentración y probabilidad van en gráficas separadas. El riesgo del mapa se rotula honestamente como "histórico", porque aún no hay predicción en vivo. El panel del clúster queda rotulado como maqueta.
- **Commits**: uno por cambio pequeño y coherente, en español, sin atribución a herramientas.

## Hallazgos (para el informe y la exposición)

1. **Causa del "sin memoria" de la semana 4.** En Go, `Estado` y `Condado` eran subcadenas de la línea completa del CSV, y `&v.Valor` hacía escapar al heap todo el `ValorLeido`. Así, el índice mantenía vivas las 17 M líneas. Internar las cadenas bajó el pico de **8,7 GB a 4,4 GB**, medido con el pico de working set del proceso en el mismo equipo.
2. **La agregadora única era el cuello de botella.** Con 8 trabajadoras, 16 de 27 s se iban en indexar. Con 8 agregadoras el procesamiento bajó a 11,5 s.
3. **Mongo era la siguiente parte serial.** Escribiendo con un escritor por índice, pasó de 83 s a 26,5 s. La ingesta completa bajó de 1m37s a 38 s.
4. **Entrenamiento (Experimento 1b).** Con reducción, S(8) = 6,11 (E = 76 %). Mutex y CAS por ejemplo son más lentos que el secuencial desde p = 2-4 (E del 4-7 % con p = 8). Con p = 12 empeora respecto de p = 8: hay 6 núcleos físicos (Ryzen 5 3600).
5. **El problema está muy desbalanceado.** Solo el 0,02 % de las horas de entrenamiento cruza NO2 > 53 ppb (0,08 % en oct-dic). El modelo logra AUC 0,995 y exhaustividad 0,97, pero precisión 0,011. Es esperable, y se reporta así. Opciones para la Fase 4: usar O3 en 8 h como objetivo, el umbral AQI 101, o los rezagos de la serie, que hoy no están.
6. **Docker en la PC de desarrollo.** Docker Desktop está instalado pero no arranca porque falta **WSL 2**, que requiere permisos de administrador y reiniciar. Para medir se usó un MongoDB 7.0.14 portable (zip oficial, sin instalar) en `localhost:27017`. El pipeline admite `-SinDocker` para ese caso. En la PC Linux del grupo, `docker compose up -d mongo` funciona tal cual.

7. **Mongo se trabó a mitad del primer intento del Experimento 1.** En la repetición 2, una escritura quedó 35 min bloqueada hasta el timeout, con avisos `WTEVICT` de WiredTiger en el log. La causa: la caché de WiredTiger usa por defecto ~50 % de la RAM (~7,5 GB), el cargador ocupa 4-6 GB y la PC tiene 16 GB. Al escribir con varios escritores a la vez, el desalojo de caché se atascó. La solución fue `--wiredTigerCacheSizeGB 2`, ya incluida en `docker-compose.yml`. El experimento se repitió completo con esa configuración, para no mezclar corridas de configuraciones distintas. Las 17 corridas del intento fallido se descartaron.

8. **Una condición de carrera lógica en el cargador** (14 de octubre de 2026). Al recargar los datos para verificar la reproducibilidad, unas pocas estaciones tenían valores distintos a los de la carga anterior.
   - **Causa:** 53 sitios miden el mismo gas con varios monitores (columna POC), y 335 331 filas (1,98 %) repiten (estación, hora, gas). El índice guardaba "el último valor que llegaba", y con goroutines ese orden cambia en cada corrida.
   - **Por qué `-race` no la detectó:** no hay accesos simultáneos a memoria, porque cada índice tiene una sola goroutine dueña; el problema era la regla.
   - **Solución:** gana el monitor de POC más bajo, en cualquier orden. Se agregó un test de independencia del orden, y la prueba end-to-end ahora compara una huella de los **valores** (cantidad y suma por gas), no solo la cantidad de registros.
   - **Verificación:** dos cargas completas seguidas producen exportaciones idénticas.
   - **Efecto sobre los experimentos ya medidos:** la cantidad de registros no cambia y el costo de la regla es una comparación por valor, así que los tiempos del Experimento 1 siguen siendo válidos.

## Resultados del Experimento 1

39 corridas: 3 repeticiones del secuencial y de 6 valores de p × 2 diseños, con MongoDB y caché de 2 GB.

- **T(1) = 132,4 s** (secuencial: 39 s de procesamiento y 94 s de escritura en Mongo).
- **1 agregadora:** S(p) entre 1,03 y 1,13. La fracción serial medida es f = 0,797, así que Amdahl da un techo de 1,25×: lo predijo bien. Karp-Flatt ≈ f.
- **Agregadoras particionadas:** S(2) = 1,54, S(4) = 2,30, S(8) = 3,21, **S(16) = 3,77** (35,1 s), con Karp-Flatt estable en ~0,21. Supera el techo de Amdahl del diseño original porque paraleliza lo que antes era serial.

Análisis completo en [docs/experimento1.md](../docs/experimento1.md); tablas en [resultados/experimento1.md](../resultados/experimento1.md).

### Cierre de esta entrega (14 de octubre de 2026)

- Modelo guardado en `data/modelo.json` y datos reales del frontend exportados (`frontend/public/datos/`, versionados).
- El riesgo del mapa pasó de "p98 / umbral" a **% de horas sobre el umbral AQI 51**. Con la métrica anterior, 1075 de 1447 estaciones salían en "alto" y el mapa no distinguía nada.
- Verificación final: `go build`, `go vet` y `go test -race ./...` en verde; `npm run lint` y `npm run build` en verde; el frontend sirve todos los JSON.

## Cambio de alcance: todas las tecnologías del sílabo (13 de octubre de 2026)

El grupo decidió implementar MPI, Pthreads, OpenMP y CUDA dentro del proyecto, y no solo sus equivalentes en Go. Se reescribieron las guías:

- [proyecto9.md](proyecto9.md): §1, §6 "Núcleos nativos", §7, §8 (8 experimentos), §10 (cronograma de las semanas 3-16), §12, §13, §15 y §17;
- [plan_aprendizaje.md](plan_aprendizaje.md);
- [docs/conceptos_curso.md](../docs/conceptos_curso.md);
- el nuevo [docs/contrato_formato_binario.md](../docs/contrato_formato_binario.md).

Entorno comprobado en la PC de desarrollo (Ryzen 5 3600, Windows 11):

| Tecnología | Estado |
|---|---|
| gcc 13.2 de MSYS2 con `omp.h` y `pthread.h` | ✅ Pthreads y OpenMP se pueden compilar ya |
| MPI | ❌ No instalado. Requiere MS-MPI (administrador) u OpenMPI en Linux/Docker |
| CUDA | ❌ La GPU es una **AMD Radeon RX 580**. El código CUDA se escribe aquí y se compila y mide en **las PCs del grupo con NVIDIA**; Colab queda como respaldo |

## Pendientes inmediatos

Semana 8, para el parcial si alcanza el tiempo (temas de las semanas 3-7 del sílabo):

1. `herramientas/cmd/exportar-muestra`: escribe `data/muestra.bin`, `data/series.bin` y `data/muestra_referencia.json` según el contrato.
2. `nativo/comun`: lectura del binario y escritura del CSV de resultados.
3. `nativo/openmp/entrenamiento`: `reduction` sobre el gradiente; validar la pérdida contra Go.
4. `nativo/pthreads/cargador`: productor-consumidor con buffer acotado y variables de condición; validar los conteos contra Go (16 972 287 leídas, 1625 excluidas).
5. `nativo/mpi/entrenamiento`, versiones punto a punto y colectiva: instalar antes MS-MPI.

Clúster físico ([docs/guia_cluster.md](../docs/guia_cluster.md)). El grupo tiene **2 PCs con NVIDIA** y las demás aportan CPU:

- Semana 8: inventario de las PCs y elección del sistema operativo común para MPI.
- Semana 9: maestro Go con trabajadores en PCs reales.
- Semana 10: clúster MPI + ping-pong (Experimento 7a).
- Semanas 12-13: híbrido MPI + OpenMP + CUDA con reparto proporcional (Experimento 9).

Fase 3 (semanas 9-12):

- Semana 9:
  - pasos 3-6 del protocolo TCP en Go (difundir `parametros`, `gradiente_parcial`, agregación síncrona). El trabajador carga su partición con `particion.Pertenece` y llama a `logistica.GradienteParcial`;
  - OpenMP `task`/`schedule` para rezagos y medias móviles por estación (Experimento 6).
- Semanas 9-10: endpoints de la API (`/predicciones` con `dataset.ModeloGuardado.Probabilidad`), JWT, Redis. Experimento 7 (MPI frente a TCP).
- Semana 10: Experimento 3 sobre el vector de parámetros del maestro (mutex frente a CAS).
- Semanas 11-13: CUDA (kernels atomic, shared y shuffle; luego Thrust sort/MapReduce) en las PCs con NVIDIA. Experimento 8.
- Dockerfiles para maestro y trabajadores; servicio OpenMPI opcional en `docker-compose.yml`.
- Instalar WSL 2 en la PC de desarrollo para usar Docker Desktop.

## Verificación y documentación para el grupo (14 de octubre de 2026)

- `go run ./herramientas/cmd/e2e`: prueba end-to-end de 15 pasos (~1-2 min) con un subconjunto de los datos reales y una base temporal. No toca `data/` ni `resultados/`. Resultado: **TODO OK**.
- `scripts/mongo-local.ps1`: MongoDB portable para Windows sin Docker. El pipeline lo usa con `-MongoLocal`.
- Pipeline probado de punta a punta con `-MongoLocal`: 2,8 min sin la descarga, y el modelo resultó idéntico al versionado (se reutiliza la caché de la muestra).
- Documentación para ejecutar el proyecto:
  - [docs/guia_ejecucion.md](../docs/guia_ejecucion.md): instalación, 5 tutoriales, problemas frecuentes;
  - [docs/arquitectura.md](../docs/arquitectura.md): diagramas de componentes, datos y flujos;
  - [docs/funcionalidades.md](../docs/funcionalidades.md): catálogo de programas y opciones.

## Clúster distribuido para la presentación virtual (15 de octubre de 2026)

El grupo debe presentar el avance del clúster de forma **virtual**: las PCs están en casas distintas, y solo algunas tienen GPU NVIDIA. Se adelantaron los pasos 3-6 del protocolo, previstos para la semana 9.

**Decisiones**
- **TCP en Go y no MPI para la demo.** MPI entre casas por internet exige el mismo usuario y la misma versión en todas las PCs, más `smpd` y puertos abiertos en ambos sentidos: es frágil en vivo. En TCP solo los nodos se conectan al maestro. MPI sigue en el plan (Experimento 7) y se comparará contra este TCP.
- **Tailscale** para unir las PCs: IPs `100.x` sin tocar el router. Basta con compartir el nodo del maestro.
- **Los nodos no usan Mongo.** Cada uno carga `data/muestra.gob` y repite la preparación determinista (`entrenador.PrepararDesde`). La **huella SHA-256** de la muestra va en el registro, y el maestro rechaza a quien tenga otra.
- **Heterogeneidad con un motor por nodo.** El motor `go` usa goroutines con reducción. El motor `cuda` es un proceso C++/CUDA (`nativo/cuda/gradiente_servidor.cu`) al que el trabajador habla por stdin/stdout en binario: los datos se copian a la GPU una vez y por época viajan 13 números. Antes de usarlo, el trabajador **valida** su gradiente contra el de Go; si no coincide, sigue con goroutines.
- **Kernel CUDA:** reducción en árbol en memoria compartida por bloque, y la CPU suma los bloques en orden fijo. Es determinista y no necesita `atomicAdd(double)` (sm_60+); las variantes atomic/shuffle quedan para el Experimento 8.
- **Sin locks en el modelo.** Las goroutines de cada conexión pasan los gradientes por un channel al bucle de entrenamiento, que es el único que toca los pesos.
- **Modo síncrono y aborto ante una caída.** La redistribución y el modo asíncrono quedan para el Experimento 4.

**Hallazgos**
1. **La pérdida distribuida es igual a la secuencial:** diferencia de 2,6e-15 con la muestra real; solo cambia el orden de las sumas.
2. **Experimento 2 local** (procesos en una PC, 1 goroutine por nodo, mediana de 3): S(2) = 1,62, S(4) = 1,96, S(8) = 2,51. La red por localhost es solo el 3-5 % de cada época. Lo que limita es el desbalance de la partición por hash (con 4 nodos, entre 34 mil y 39 mil ejemplos) y que hay 6 núcleos físicos. Karp-Flatt ≈ 0,31.
3. **Motor CUDA probado sin GPU NVIDIA.** El `.cu` se compiló con g++ emulando CUDA en la CPU (`nativo/cuda/emulacion/`: un hilo por hilo CUDA, `std::barrier` como `__syncthreads`). Pasó la validación contra Go y el entrenamiento llegó a la misma pérdida. Falta compilarlo con `nvcc` en una PC del grupo con NVIDIA.
4. **Condición de carrera encontrada por los tests.** Si un nodo se cae entre dos épocas, el maestro podía fallar al enviarle los parámetros antes de leer el aviso de caída, y el mensaje era confuso. Ahora ambos casos se reportan como "el nodo X se cayó". `go test -race -count=20` en verde.

**Entregables**
- Código: `internal/distribuido`, `entrenamiento/cmd/maestro` y `trabajador`, `nativo/cuda/`.
- Scripts: `scripts/cluster-local.*`, `nodo.*`, `experimento2.ps1`.
- Documentos: guías por rol en [docs/cluster/](../docs/cluster/README.md) (maestro, nodo sin GPU NVIDIA y nodo con GPU NVIDIA, para quien no conoce el código) y [docs/guia_presentacion_cluster.md](../docs/guia_presentacion_cluster.md) (guion, demo, plan B y preguntas).
- La prueba e2e (paso 12) ahora entrena distribuido y compara con el secuencial: **TODO OK**.

**Pendiente antes de presentar**
- Inventario y Tailscale en las PCs del grupo; ensayo general con la tabla de latencias.
- Compilar el motor CUDA con `nvcc` en las 2 PCs con NVIDIA.
- Correr el Experimento 2 por Tailscale (`-modo tcp_tailscale`) y actualizar la diapositiva 5.
- Opcional: panel del clúster del frontend con los datos reales del Experimento 2 (hoy es una maqueta).

## Clúster real por Tailscale con todos los datos (15 de octubre de 2026, tarde)

**Primer ensayo real:** maestro en Windows (Piero) y nodo en Fedora (angelo), en casas distintas. Con la muestra de prueba (146 mil ejemplos): 2,8 ms de cálculo + 7,4 ms de red por época, **72 % de comunicación**, misma pérdida que la secuencial y la caída del nodo detectada en la época 239. No justificaba un clúster.

**Decisión:** el clúster usa **todos los registros con NO2 etiquetables**.
- Se construye con `secuencial -muestra 0 -cache data/muestra.gob`: 3 342 599 ejemplos, 2 499 636 de entrenamiento, 337 MB, huella `c2c8f8832a30`, ~10 min desde Mongo.
- La muestra anterior pasó a `data/muestra_prueba.gob` y sigue siendo la caché por defecto de `secuencial`/`paralelo`, para el Experimento 1b y la e2e.
- Los resultados con la muestra de prueba quedaron en `resultados/experimento2_prueba.*`.

**Experimento 2 con la muestra completa** (3 rondas por terminal + corridas desde la página; mediana; T(1) secuencial = 41,5 s, 207 ms por época):

| Nodos | T(p) | S | Cálculo del más lento | Red | % red |
|---|---:|---:|---:|---:|---:|
| augusto (RTX 3060 Ti) | 2,28 s | 18,2 | 3,1 ms | 8,3 ms | 73 % |
| + josue (RTX 4060 Laptop) | 3,92 s | 10,6 | 5,4 ms | 12,7 ms | 70 % |
| + angelo (CPU, 16 hilos) | 7,29 s | 5,7 | 18,1 ms | 18,1 ms | 50 % |

**Hallazgos:**
- Una GPU remota calcula una época de 2,5 M ejemplos en ~3 ms. Con eso la red vuelve a dominar, y agregar nodos empeora el tiempo.
- Con el nodo CPU, el modo síncrono espera al más lento. Motiva el reparto proporcional al rendimiento (Experimento 9): se estima que bajaría el cálculo por época de ~18 ms a ~3,5 ms.

**Incidentes:**
- **PC del maestro sin memoria.** Al pasar `-Nodos 1,2,4` desde otra terminal, PowerShell lo recibió como un texto y `[int[]]` lo convirtió en **124**: se lanzaron 124 trabajadores de ~1,5 GB cada uno. Ahora `scripts/memoria.ps1` lee la lista correctamente, limita el rango a 1-32 y **comprueba la RAM libre antes de lanzar procesos** (~4,5 veces el tamaño de la muestra por proceso).
- **Ctrl+C en la ventana del maestro.** La prueba de caída se hacía presionando Ctrl+C en el maestro y no en el nodo. Ahora el maestro distingue los dos casos en su mensaje.
- **Nodos que llegaban tarde.** Algunos se registraban durante la verificación secuencial. Ahora el maestro deja de aceptar nodos al terminar el entrenamiento y avisa durante la verificación.

## Cómo retomar

```powershell
go run ./herramientas/cmd/descargar -verificar     # ¿tengo los datos correctos?
powershell -ExecutionPolicy Bypass -File scripts/pipeline.ps1 -MongoLocal   # o sin -MongoLocal si Docker funciona
go run ./herramientas/cmd/e2e                       # ¿funciona todo?
cd frontend; npm install; npm run dev
```
