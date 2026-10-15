# Proyecto 9 — Sistema de alerta temprana de contaminación del aire

Curso: Programación Concurrente y Paralela (202W0805) · UNMSM, Escuela Profesional de Ingeniería de Software · Semestre 2026-2 · Ciclo VIII

Docentes: Javier Antonio Prudencio Vidal (jprudenciov@unmsm.edu.pe), Pablo Edwin López Villanueva (plopezv@unmsm.edu.pe). Prerrequisitos: Sistemas Operativos, Redes y Transmisión de Datos.

Este documento consolida todo el material de referencia del proyecto (ficha técnica, cronograma y contexto del sílabo) que antes estaba repartido en varios archivos.

---

## 1. Resumen ejecutivo

Se construye un sistema que procesa en paralelo los **16,97 millones** de mediciones horarias reales de calidad del aire de 2025 de la red de monitoreo de Estados Unidos (EPA). Con ellas entrena un modelo de aprendizaje automático y expone una predicción útil para la salud pública: en qué estaciones y en qué horas el aire alcanzará un nivel de contaminación que obliga a tomar precauciones.

El sistema base está escrito en **Go** y se despliega con Docker. Desde el 13 de octubre de 2026 el proyecto también implementa **todas las tecnologías del sílabo** como núcleos de cómputo propios, escritos en C/C++:

| Tecnología del sílabo | Semanas del sílabo | Qué resuelve en SACA |
|---|---|---|
| **MPI** | 3-4 | Entrenamiento distribuido entre procesos: scatter de datos, broadcast de pesos y reduce de gradientes |
| **Pthreads** | 5-6 | Cargador productor-consumidor con mutex, variables de condición y barreras |
| **OpenMP** | 7, 9-10 | Gradiente con `reduction` y cálculo de variables por estación con `task`/`schedule` (balanceo de carga) |
| **CUDA** | 11-13 | Gradiente en GPU (reducción en memoria compartida y warp shuffles) y ordenamiento/MapReduce en GPU |
| Go: goroutines, channels, TCP | todo el curso | Sistema completo: cargador, maestro-trabajador, API y orquestación |

Todos los núcleos resuelven **el mismo problema con exactamente los mismos datos** (un archivo binario que exporta Go, ver [docs/contrato_formato_binario.md](../docs/contrato_formato_binario.md)). Por eso pueden compararse de forma directa: misma pérdida final y distinto tiempo.

El proyecto combina las tres dimensiones del curso:

- **Paralelismo en memoria compartida**: goroutines, Pthreads y OpenMP dentro de una máquina.
- **Distribución por paso de mensajes**: maestro-trabajador sobre TCP en Go, y MPI.
- **Aceleración en GPU**: CUDA.

Todo se ejecuta también en un **clúster físico heterogéneo** formado por las computadoras del grupo: **2 PCs con GPU NVIDIA** y las demás aportando su procesador, conectadas por la red local. Se usa un modelo híbrido, como en HPC: MPI (o TCP en Go) entre PCs, OpenMP (o goroutines) dentro de cada PC y CUDA en las GPUs. Ver [docs/guia_cluster.md](../docs/guia_cluster.md).

## 2. Fundamentos, para quien recién empieza en paralelismo

**Secuencial vs. concurrente vs. paralelo vs. distribuido.** Un programa secuencial hace una cosa a la vez: si tiene que recorrer 15 millones de líneas, las procesa una por una y el tiempo total es la suma de todos esos pasos. Una computadora moderna tiene varios núcleos (4, 8, 16); con un programa secuencial, uno solo trabaja y el resto está ocioso.

| Término | Qué significa | Ejemplo cotidiano |
|---|---|---|
| **Concurrencia** | Varias tareas *en progreso* al mismo tiempo, aunque no necesariamente se ejecuten en el mismo instante. Es una forma de organizar el programa. | Un cocinero que pone a hervir agua, mientras espera pica cebolla, y vuelve a la olla. Una sola persona, varias tareas en curso. |
| **Paralelismo** | Varias tareas ejecutándose *realmente al mismo tiempo* en distintos núcleos. Es una propiedad del hardware. | Cuatro cocineros, cada uno en su propia hornilla. |
| **Distribución** | Varias computadoras (o contenedores) distintas cooperando por red para resolver un solo problema. No comparten memoria, solo se mandan mensajes. | Cuatro cocinas en cuatro edificios distintos que se coordinan por teléfono. |

El curso enseña tres modelos de paralelismo, y el proyecto implementa los tres, cada uno con las mismas herramientas del laboratorio y además con su equivalente en Go:

- **Memoria compartida**: varios hilos en la misma máquina que ven las mismas variables. Es rápido, pero peligroso, porque dos hilos pueden pisarse al escribir en la misma variable. En el proyecto se hace con **Pthreads**, **OpenMP** y goroutines de Go.
- **Paso de mensajes**: varios procesos que no comparten nada y se comunican enviándose datos. Es más lento pero escala a muchas máquinas. En el proyecto se hace con **MPI** y con conexiones TCP entre contenedores Docker (Go).
- **GPU (SIMT)**: miles de hilos ligeros ejecutan la misma instrucción sobre datos distintos. Sirve para cálculos masivos y regulares, pero copiar los datos a la tarjeta tiene un costo fijo, así que solo conviene si hay suficiente trabajo. En el proyecto se hace con **CUDA**.

**Condición de carrera.** Si dos hilos leen la variable `contador = 5`, cada uno le suma 1 y la guarda, lo esperado es 7. Pero si ambos leyeron el 5 antes de que el otro escribiera, ambos guardan 6: un incremento se perdió. Es el error más importante que el curso pide evitar. Las herramientas para evitarlo son el **mutex** (un candado que solo un hilo puede tener a la vez) y las **operaciones atómicas** (instrucciones del procesador que hacen leer-modificar-escribir sin que nadie pueda meterse en medio).

**Ley de Amdahl.** Si duplicas los núcleos, no vas a ir el doble de rápido. Siempre hay una parte del programa que no se puede paralelizar (abrir el archivo, juntar los resultados finales, escribir en la base de datos). Si esa parte es el 10 % del tiempo, aunque tengas infinitos núcleos, nunca irás más de 10 veces más rápido. Esta ley explica por qué las curvas de rendimiento se aplanan, y el curso espera que se use para analizar los resultados.

**Métricas que se miden en todo el proyecto:**

- **Speedup** S(p) = T(1) / T(p). Cuántas veces más rápido es con *p* unidades respecto a la versión secuencial. Ideal: S(p) = p.
- **Eficiencia** E(p) = S(p) / p. Qué fracción de cada núcleo añadido se está aprovechando. Ideal: 1.0. En la práctica baja conforme se añaden núcleos.

Por eso el proyecto exige primero una **versión secuencial** de todo. Sin T(1) no hay contra qué comparar.

## 3. Problema e impacto social

La contaminación atmosférica es un problema de salud pública cuya principal dificultad no es medirla sino anticiparla. Las redes oficiales de monitoreo publican lo que **ya ocurrió**: cuando una persona con asma, un adulto mayor o un colegio se enteran de que el aire está dañino, la exposición ya sucedió.

El valor social del sistema está en desplazar esa información hacia adelante en el tiempo. Si se puede anticipar con horas de anticipación que una zona concreta cruzará el umbral de riesgo, son posibles decisiones concretas: suspender la actividad física escolar al aire libre, reprogramar el horario de trabajadores expuestos, avisar a pacientes respiratorios, o restringir temporalmente el tráfico en un corredor.

El mismo problema es directamente trasladable a Lima Metropolitana, que cuenta con red de monitoreo de calidad del aire operada por SENAMHI: el sistema no es un ejercicio abstracto sino una arquitectura reutilizable en el contexto peruano.

## 4. Pregunta predictiva

**En lenguaje natural:** ¿en qué estaciones de monitoreo y en qué horas del próximo día el aire superará el nivel considerado aceptable para la población general?

**Formulación técnica:** dada una estación de monitoreo *s*, un contaminante *p* y una hora *t*, predecir la probabilidad de que en la hora *t + h* la concentración de *p* medida en *s* cruce el umbral del índice de calidad del aire, con *h* configurable entre 1 y 24 horas.

- Tipo de problema: **clasificación binaria**.
- Salida: probabilidad en el intervalo [0,1], más la etiqueta resultante.
- Modelo: **regresión logística** entrenada por descenso de gradiente estocástico (SGD).

**Por qué este planteamiento y no otro:**

a. Es binario, y por lo tanto entrenable con regresión logística, el modelo que sugiere el enunciado. El foco del curso es la paralelización del entrenamiento, no la sofisticación del modelo.
b. El gradiente de la regresión logística es una suma sobre ejemplos independientes, lo que la hace perfectamente descomponible por particiones de datos: cada nodo trabajador calcula el gradiente parcial de su porción y el maestro los combina. Es el caso de libro del patrón *parameter server*.
c. La salida es directamente accionable: una alerta, no un número abstracto.

**Definición de la etiqueta**, según los puntos de corte oficiales del Air Quality Index (AQI) de la EPA. La etiqueta principal marca el cruce del AQI 51 (paso de "Good" a "Moderate" o superior):

| Contaminante | Umbral AQI 51 |
|---|---|
| Ozono (O3), promedio móvil de 8 horas | > 0.054 ppm |
| Dióxido de nitrógeno (NO2), 1 hora | > 53 ppb |
| Dióxido de azufre (SO2), 1 hora | > 35 ppb |
| Monóxido de carbono (CO), promedio de 8 horas | > 4.4 ppm |

Se elige el umbral de AQI 51 y no el de AQI 101 ("Unhealthy for Sensitive Groups") porque los episodios por encima de 101 son poco frecuentes en la mayoría de estaciones y producirían un conjunto de entrenamiento severamente desbalanceado. El umbral 101 se conserva como etiqueta secundaria para reportar el desempeño del sistema en el escenario crítico.

> Nota de verificación: los puntos de corte deben contrastarse con la tabla vigente del documento técnico del AQI de la EPA antes de fijarlos en código.

**Variables de entrada (features):**

1. Rezagos de la propia serie: concentración en t, t-1, t-2, t-3, t-6, t-12 y t-24 horas.
2. Estadísticos móviles: media y máximo de las últimas 24 horas.
3. Estacionalidad temporal: hora del día y día de la semana codificados como seno y coseno (para preservar su naturaleza cíclica), más el mes.
4. Contaminantes cruzados: concentración simultánea de los otros tres gases en la misma estación, porque comparten fuentes de emisión (tráfico, industria) y sus picos están correlacionados.
5. Contexto geográfico de la estación: latitud, longitud y tipo de zona urbana, obtenidos del archivo de resumen anual.

Esta combinación sigue el criterio validado en la literatura de predicción urbana: un componente espacio-temporal (rezagos + estacionalidad) más un componente de atributos externos (contaminantes cruzados + contexto de la estación).

## 5. Origen y volumen de los datos

Fuente: United States Environmental Protection Agency (EPA), sistema Air Quality System (AQS), sección de archivos pre-generados de descarga directa. Datos públicos, de acceso libre y sin registro previo.

- Portal: https://aqs.epa.gov/aqsweb/airdata/download_files.html#Raw — cada integrante del equipo descarga los 4 ZIP desde aquí, en la sección "Tables of Hourly Data" del año 2025 y descomprimirlo (colocarlo en data/raw)
- Patrón de URL: `https://aqs.epa.gov/aqsweb/airdata/hourly_<codigo>_<anio>.zip`

Archivos a descargar, uno por cada gas contaminante criterio:

| Archivo | Contaminante |
|---|---|
| `hourly_44201_<anio>.zip` | Ozono (O3) |
| `hourly_42602_<anio>.zip` | Dióxido de nitrógeno (NO2) |
| `hourly_42401_<anio>.zip` | Dióxido de azufre (SO2) |
| `hourly_42101_<anio>.zip` | Monóxido de carbono (CO) |

Cada archivo horario contiene del orden de dos a nueve millones de filas por año, según la cantidad de monitores desplegados para ese contaminante. Los cuatro archivos juntos superan holgadamente los quince millones de registros, muy por encima del millón que exige el enunciado.

Se usa un año calendario completo y ya publicado, de modo que el ciclo estacional esté íntegro. Si se necesita más volumen o historia, se encadenan dos años consecutivos sin ningún cambio en el procesamiento, porque el esquema de columnas es idéntico entre años.

Granularidad: una fila por estación de monitoreo, contaminante y hora.

**Estructura de cada fila** (archivo horario de AQS): `State Code, County Code, Site Num, Parameter Code, POC, Latitude, Longitude, Datum, Parameter Name, Date Local, Time Local, Date GMT, Time GMT, Sample Measurement, Units of Measure, MDL, Uncertainty, Qualifier, Method Type, Method Code, Method Name, State Name, County Name, Date of Last Change`. El campo `Sample Measurement` es la concentración medida y es la base tanto de las variables de entrada como de la etiqueta.

**Archivo complementario ya obtenido:** `annual_conc_by_monitor_<anio>.csv`, resumen anual por monitor. Contiene 27,894 filas que describen 1,824 sitios de monitoreo distribuidos en 52 estados, con 279 parámetros medidos. No sirve como fuente principal porque está agregado por año y carece de serie temporal, pero cumple un papel útil como tabla de metadatos: aporta ubicación, nombre local, condado y área metropolitana de cada estación, además de percentiles y máximos anuales.

**Preparación de los datos:** se descartan las filas con medición nula o marcadas con qualifier de evento excepcional. Se unifican las unidades por contaminante. Se alinean los cuatro contaminantes por la clave compuesta (estación, fecha, hora) para construir las variables cruzadas. Se normalizan las variables numéricas antes del entrenamiento, porque el descenso de gradiente es sensible a la escala de las entradas.

## 6. Arquitectura del sistema

El sistema tiene cuatro componentes, diseñados para desarrollarse en paralelo entre los integrantes del equipo (ver §9 sobre organización).

```
   CSV crudo (15 M+ filas)
        │
        ▼
 ┌──────────────────┐
 │ PARTE A          │  goroutines + channels
 │ Cargador         │  (memoria compartida)
 │ concurrente      │
 └────────┬─────────┘
          │ escribe por lotes
          ▼
     ┌─────────┐        ┌─────────┐
     │ MongoDB │        │  Redis  │
     └────┬────┘        └────┬────┘
          │                  │ cache
          ▼                  │
 ┌──────────────────┐        │
 │ PARTE B          │        │
 │ Maestro (TCP)    │◄───────┼──── Trabajador 1  (contenedor)
 │ parámetros del   │◄───────┼──── Trabajador 2  (contenedor)
 │ modelo           │◄───────┼──── Trabajador N  (contenedor)
 └────────┬─────────┘        │      (paso de mensajes)
          │ modelo entrenado │
          ▼                  │
 ┌──────────────────┐        │
 │ PARTE C          │◄───────┘
 │ API REST + JWT   │
 │ WebSocket        │
 └────────┬─────────┘
          ▼
 ┌──────────────────┐
 │ PARTE D          │
 │ SPA (mapa,       │
 │ telemetría,      │
 │ curvas speedup)  │
 └──────────────────┘
```

### Parte A — Cargador de datos concurrente (memoria compartida)

Responsabilidad: convertir quince millones de líneas de CSV crudo en un conjunto de entrenamiento limpio, en el menor tiempo posible y usando toda la CPU disponible.

Diseño: el archivo se divide en bloques de bytes de tamaño fijo, ajustados hacia adelante hasta el siguiente salto de línea para no partir un registro por la mitad. Un *despachador* emite los límites de cada bloque por un channel; un *pool de goroutines* trabajadoras consume de ese channel, parsea su bloque, valida y transforma las filas, y emite los registros resultantes por un channel de salida. Una goroutine *agregadora* los recibe y los escribe en MongoDB por lotes.

Patrones de concurrencia empleados: **worker pool**, **fan-out** (despachador → trabajadoras), **fan-in** (trabajadoras → agregadora), y `sync.WaitGroup` para esperar la terminación ordenada. El cierre explícito de los channels evita el error clásico de goroutines filtradas.

Parámetro de control del experimento: el número de goroutines trabajadoras.

### Parte B — Motor de entrenamiento distribuido (paso de mensajes)

Es el corazón del proyecto. Responsabilidad: entrenar la regresión logística repartiendo el cómputo entre varios procesos que corren en contenedores o máquinas distintas.

Diseño: patrón **parameter server** (maestro-trabajador). El maestro mantiene el vector de parámetros del modelo y escucha en un puerto TCP interno. Cada trabajador se conecta, recibe su partición de datos y la copia vigente de los parámetros, calcula el gradiente parcial por SGD sobre su partición y lo devuelve. El maestro agrega los gradientes recibidos, actualiza los parámetros y difunde la nueva versión. Eso es una *época*; se repite hasta que el modelo converge.

Por qué funciona: el gradiente de la regresión logística es una suma sobre ejemplos independientes, así que sumar los gradientes parciales de varios trabajadores da exactamente el mismo resultado que calcularlo todo en uno solo.

**Sección crítica:** la actualización del vector de parámetros, exactamente el punto donde el enunciado exige demostrar ausencia de condiciones de carrera. Se implementan y comparan dos estrategias: actualización protegida por **mutex**, y actualización sin bloqueos mediante **operaciones atómicas** de comparar-e-intercambiar (CAS). Esa comparación (tiempo por época y estabilidad de convergencia) es un resultado presentable por sí mismo.

**Modo de agregación:** se implementa el esquema **síncrono** (el maestro espera a todos los trabajadores antes de actualizar) y opcionalmente el **asíncrono** (actualiza conforme llegan los gradientes). El síncrono es más estable; el asíncrono tolera mejor a los nodos lentos.

Parámetro de control del experimento: el número de nodos trabajadores.

### Parte C — API de predicciones en tiempo real

- Servicio REST en Go para consultar predicciones por estación y horizonte temporal, y para lanzar/consultar entrenamientos.
- Autenticación por **JWT**, distinguiendo consumidores de solo lectura de usuarios con permiso para disparar entrenamientos.
- Canal **WebSocket** que envía al frontend, en vivo, el avance del entrenamiento (pérdida por época), la telemetría de los nodos (CPU, RAM) y las alertas nuevas.
- Servidor TCP interno que actúa como maestro del clúster de nodos de cómputo (Parte B), incluyendo registro y baja de nodos.
- Objetivo de latencia: **menos de 100 ms** por predicción, medido en el percentil 95.
- Cache en **Redis** de las predicciones ya calculadas.

> La API atiende muchas peticiones a la vez, pero eso lo hace Go solo (goroutines por request). No cuenta como mérito de paralelización propio del equipo y no conviene presentarlo como tal.

### Parte D — Visualización de impacto

SPA (React) que presenta:

- Mapa de las estaciones de monitoreo coloreado por nivel de riesgo previsto para las próximas horas.
- Línea de tiempo de un día para una estación seleccionada, contrastando concentración observada vs. predicha.
- Panel de estado del clúster alimentado por WebSocket: nodos activos, consumo de CPU/RAM, época en curso y curva de pérdida.
- Panel de rendimiento con las curvas de speedup y eficiencia obtenidas en los experimentos — la evidencia visual de la parte concurrente y paralela del trabajo.

### Núcleos nativos (C/C++): Pthreads, OpenMP, MPI y CUDA

Cada tecnología del sílabo tiene un programa propio en `nativo/`, y todos comparten el mismo flujo:

1. Go exporta el conjunto de datos ya limpio a un **formato binario simple**, con encabezado y matrices `float64` en orden fila (`herramientas/cmd/exportar-muestra`). Hay dos variantes:
   - la muestra de entrenamiento y prueba, ya normalizada;
   - los registros horarios por estación, para el cálculo de variables.
2. El núcleo nativo lee ese archivo, hace el cálculo con su tecnología y agrega una fila al **mismo CSV de resultados** que usan los programas en Go (`modo, goroutines, ..., t_total_s`). La columna `goroutines` significa "unidades de ejecución": hilos, procesos MPI o bloques de CUDA, según el caso.
3. `experimentos/cmd/analisis` resume todos los CSV con el mismo método: mediana, speedup, eficiencia, Karp-Flatt y Amdahl.

| Programa | Tecnología | Pieza del sistema que replica | Conceptos del sílabo |
|---|---|---|---|
| `nativo/pthreads/cargador` | Pthreads | Parte A: limpieza de los CSV de la EPA | Creación y unión de hilos, mutex, **productor-consumidor con buffer acotado y variables de condición**, barrera, thread-safety |
| `nativo/openmp/entrenamiento` | OpenMP | Parte B: gradiente de la regresión logística | `parallel for`, variables privadas y compartidas, **`reduction`** sobre arreglos |
| `nativo/openmp/variables` | OpenMP | Rezagos t-1..t-24 y medias/máximos móviles de 24 h por estación (variables pendientes del enunciado) | **`task`**, `schedule(static/dynamic/guided)`, **balanceo de carga**: las estaciones tienen tamaños muy distintos |
| `nativo/mpi/entrenamiento` | MPI | Parte B distribuida | `MPI_Send/Recv` (versión punto a punto) frente a `MPI_Scatterv`, **`MPI_Bcast`** y **`MPI_Allreduce`** (versión colectiva) |
| `nativo/cuda/gradiente` | CUDA | Gradiente de la Parte B en GPU | Kernels, memoria global y compartida, **reducción en árbol**, **warp shuffles**, `atomicAdd` |
| `nativo/cuda/estaciones` | CUDA + Thrust | Ordenamiento por (estación, hora) y estadísticas por estación | **Sorting paralelo** y **MapReduce en GPU** (`sort_by_key`, `reduce_by_key`) |

Reglas comunes:

- **Misma respuesta.** Con los mismos datos y la misma cantidad de épocas, cada implementación debe dar la misma pérdida final que `entrenamiento/cmd/secuencial`, dentro de 1e-6. Es la prueba de corrección de cada núcleo.
- **Mismas reglas de limpieza.** El cargador en Pthreads aplica las de [docs/contrato_registro_procesado.md](../docs/contrato_registro_procesado.md) y debe reportar los mismos conteos que el cargador en Go (16 972 287 filas leídas, 1625 excluidas por qualifier).
- **Dónde corre cada uno:**
  - Pthreads y OpenMP: gcc de MSYS2 en Windows, o gcc en Linux.
  - MPI: MS-MPI en Windows, u OpenMPI en Linux o en contenedores Docker.
  - CUDA: solo en GPU **NVIDIA**. El código se escribe en cualquier PC, pero se compila, valida y mide en **las computadoras del grupo que tienen GPU NVIDIA**. La PC de desarrollo principal tiene una AMD RX 580, que no ejecuta CUDA. Google Colab (GPU T4) queda como respaldo (ver §12).

### Clúster físico del grupo

Las computadoras de los integrantes forman un **multicomputador MIMD de memoria distribuida y heterogéneo**:

| Nodo | Cantidad | Qué corre |
|---|---|---|
| Principal | 1 | MongoDB, maestro Go, API, frontend, rank 0 de MPI |
| GPU | 2 (NVIDIA) | Un proceso MPI que calcula su parte del gradiente con CUDA; trabajador Go |
| CPU | el resto | Un proceso MPI con hilos OpenMP; trabajador Go |

El mismo entrenamiento se lanza de tres formas, de menor a mayor complejidad. Los pasos de instalación, firewall, `smpd`/`hostfile` y la lista de verificación están en [docs/guia_cluster.md](../docs/guia_cluster.md).

1. **Go por TCP**: el maestro en la PC principal y un `trabajador.exe` en cada PC. Ya funciona y no requiere instalar nada en los nodos.
2. **MPI entre PCs**: MS-MPI con `smpd -d` y `mpiexec -hosts` si todas usan Windows; OpenMPI con SSH sin contraseña y `hostfile` si todas usan Linux. No se mezclan sistemas operativos en un mismo trabajo MPI.
3. **Híbrido MPI + OpenMP + CUDA**: un proceso por PC. `MPI_Scatterv` reparte la muestra **en proporción al rendimiento medido de cada nodo** (balanceo de carga heterogéneo), cada nodo calcula su gradiente con OpenMP o CUDA, y `MPI_Allreduce` los suma. No hace falta MPI "CUDA-aware": por época solo se intercambian 13 números.

**Lección central que se mide:** con la muestra actual, una época dura ~12 ms de cálculo, mientras que la red agrega ~0,2-1 ms por mensaje en cable y bastante más por WiFi. El clúster solo gana cuando el trabajo por época crece (todos los registros, rezagos). Encontrar ese punto es parte de los resultados, junto con el ping-pong MPI que mide la latencia y el ancho de banda reales.

### Persistencia

- **MongoDB**: almacenamiento persistente del conjunto de datos ya procesado, del historial de entrenamientos (hiperparámetros y métricas) y del registro de predicciones emitidas.
- **Redis**: capa de cache de baja latencia para predicciones precalculadas y respuestas frecuentes, más el estado compartido de la telemetría del clúster.

### Despliegue

Todo con **Docker Compose**, separando en dos grupos la infraestructura principal (API/maestro, MongoDB, Redis, frontend) de los nodos trabajadores, de modo que el número de nodos de cómputo se pueda escalar sin tocar el resto del sistema. Esa separación es la que permite ejecutar los experimentos de escalabilidad levantando más réplicas de trabajadores.

## 7. Tecnologías

| Área | Tecnología |
|---|---|
| Backend y orquestación | Go |
| Concurrencia en Go | goroutines, channels, `sync.WaitGroup`, `sync.Mutex`, paquete `sync/atomic` |
| Hilos POSIX | **Pthreads** (C, gcc) |
| Paralelismo por directivas | **OpenMP** (C, `gcc -fopenmp`) |
| Paso de mensajes | **MPI** (C; MS-MPI en Windows, OpenMPI en Linux/Docker) y TCP propio en Go |
| GPU | **CUDA** (C++, `nvcc`, Thrust) en las PCs del grupo con GPU NVIDIA; Google Colab como respaldo |
| Comunicación con el cliente | HTTP/REST y WebSocket |
| Seguridad | JWT |
| Base de datos persistente | MongoDB |
| Cache en memoria | Redis |
| Frontend | SPA en **React** |
| Empaquetado y orquestación | Docker y Docker Compose |
| Modelo de ML | Regresión logística por descenso de gradiente, implementada desde cero en Go, C y CUDA |

## 8. Cómo se evalúa el proyecto

Tres planos, y los tres se reportan.

**Plano 1 — Rendimiento paralelo y concurrente** (el central para el curso). Se establece primero una línea base estrictamente secuencial (T(1)), y contra ella se miden speedup, eficiencia y contraste con la ley de Amdahl (fracción secuencial irreducible: lectura del archivo, agregación final de gradientes, escrituras a base de datos). También escalabilidad fuerte (mismo volumen, más unidades) y débil (volumen creciente proporcional a las unidades), y balanceo de carga entre goroutines/nodos.

Experimentos concretos. Todos se miden con 3 repeticiones, se resumen con la mediana y se comparan contra su T(1) secuencial:

| # | Experimento | Tecnología | Variable que se cambia | Estado |
|---|---|---|---|---|
| 1 | Cargador | Go (goroutines) | 1, 2, 4, 8, 12, 16 goroutines; 1 agregadora frente a N agregadoras | ✅ |
| 1b | Entrenamiento en memoria compartida | Go (goroutines) | 1-12 goroutines; reducción, mutex y CAS | ✅ |
| 2 | Entrenamiento distribuido | Go (TCP), nodos CPU o CUDA | 1, 2, 4, 8 nodos trabajadores | ✅ local (procesos); clúster por Tailscale en el ensayo |
| 3 | Sincronización del vector de parámetros del maestro | Go | mutex frente a atómicos (CAS) | semana 12 |
| 4 | Agregación | Go (TCP) | síncrono frente a asíncrono | semana 12 |
| 5 | Productor-consumidor | **Pthreads** frente a goroutines | hilos productores y consumidores; tamaño del buffer | semana 8-9 |
| 6 | Reducciones y balanceo de carga | **OpenMP** | hilos × `schedule(static/dynamic/guided)` × tamaño de bloque | semana 8-10 |
| 7 | Paso de mensajes | **MPI** frente a TCP en Go | 1-8 procesos; punto a punto frente a colectivas | semana 9-10 |
| 8 | GPU | **CUDA** frente a CPU (OpenMP) | tamaño del problema (10³ a 10⁷ ejemplos); kernel ingenuo, memoria compartida y warp shuffle | semana 11-13 |
| 7a | Red del clúster | **MPI ping-pong** entre PCs | tamaño del mensaje (8 B a 1 MB) → latencia y ancho de banda | semana 10 |
| 9 | Clúster heterogéneo | **MPI + OpenMP + CUDA** en las PCs del grupo | reparto igual frente a proporcional al rendimiento; con y sin los 2 nodos GPU; escalabilidad débil | semana 12-13 |

Los experimentos 2 y 7 se corren primero en una sola PC (procesos o contenedores) y después en el **clúster físico** (ver §6, "Clúster físico del grupo"). La diferencia entre ambos es el costo real de la red.

El experimento 8 está pensado para mostrar el **punto de cruce**. Con el problema actual (147 mil ejemplos y 12 variables) la GPU pierde contra la CPU, porque copiar los datos a la tarjeta cuesta más que el cálculo. Al crecer el número de ejemplos y de variables (rezagos), la GPU empieza a ganar. Medir dónde ocurre ese cruce es el resultado.

**Plano 2 — Calidad del servicio.** Latencia p50/p95/p99 con y sin cache de Redis (objetivo <100ms), rendimiento bajo carga concurrente de clientes, comportamiento ante la caída de un nodo trabajador durante el entrenamiento.

**Plano 3 — Calidad del modelo.** Como es un problema binario y desbalanceado, no basta con la exactitud: se reportan precisión, exhaustividad, F1 y AUC-ROC, con partición temporal (entrenar con los primeros meses, evaluar con los últimos, nunca mezclando futuro con pasado). El objetivo del modelo es ser correcto y honesto, no ser estado del arte — el aporte del proyecto está en la arquitectura distribuida que lo entrena.

## 9. Organización del equipo

Diseño original: las cuatro partes están pensadas para avanzar simultáneamente entre 4 integrantes (uno por parte A/B/C/D), más trabajo transversal de Docker Compose, integración y ejecución de experimentos. Se acuerdan desde el inicio los contratos entre partes (esquema del registro procesado, protocolo TCP maestro-trabajador, contrato de la API REST) para que cada quien trabaje contra una versión simulada (mock) de las demás partes sin quedar bloqueado.

Con los núcleos nativos, el trabajo se reparte mejor entre los 8 integrantes del grupo. Una propuesta: uno por cada parte A/B/C/D en Go, uno por cada tecnología nativa (Pthreads, OpenMP, MPI, CUDA), y los experimentos y la integración como trabajo transversal. El formato binario común es el contrato que permite avanzar los núcleos nativos sin esperar a la Parte B en Go.

> Estado actual del proyecto: sin roles asignados todavía.

> Avance real al 14 de octubre de 2026 (semanas 3-7 completas + entrenamiento paralelo + frontend con datos reales): ver [avance_parcial.md](avance_parcial.md).

## 10. Cronograma del proyecto — semanas 3 a 16

Semanas 1 y 2 (ya transcurridas): definición de la idea del proyecto. El trabajo del proyecto arranca en la semana 3, que es también cuando el sílabo marca la primera revisión de avances.

**Cambio de alcance (13 de octubre de 2026).** Se decidió implementar todas las tecnologías del sílabo (MPI, Pthreads, OpenMP, CUDA) y no solo su equivalente en Go. El cronograma se reorganizó para que cada tecnología se implemente en la semana en que el sílabo la enseña o, si esa semana ya pasó, lo antes posible. MPI, Pthreads y OpenMP son temas de las semanas 3-7, que son las que cubre el examen parcial.

Hitos fijos del sílabo: revisión de avances desde la semana 3, examen parcial en la 8, exposiciones en la 13, 14 y 15, y examen final en la 16.

Convención: A = cargador, B = entrenamiento, C = API, D = frontend, N = núcleos nativos (C/C++), T = transversal (Docker, integración, experimentos).

### Fase 1 · Semanas 3-4 · Datos, contratos y línea base secuencial ✅

| Sem. | Trabajo del proyecto | Estado |
|---|---|---|
| 3 | Descargar los 4 CSV horarios (16 972 287 filas) y acordar los contratos | ✅ Descarga reproducible con manifiesto SHA-256 (`herramientas/cmd/descargar`) |
| 4 | Cargador secuencial y regresión logística secuencial: T(1) de ambos | ✅ Cargador: 128-164 s con Mongo (37 s sin Mongo). Entrenamiento: 2,4 s |

### Fase 2 · Semanas 5-8 · Paralelismo en memoria compartida y paso de mensajes (temas del parcial)

| Sem. | Sílabo | Trabajo del proyecto | Estado |
|---|---|---|---|
| 5-6 | Pthreads | A (Go): cargador concurrente con worker pool, fan-out/fan-in y agregadoras particionadas; escritura concurrente en Mongo; `-race` | ✅ |
| 6 | — | B: partición de datos por hash de estación. D: mapa con metadatos reales | ✅ |
| 7 | OpenMP | T: Experimento 1 (cargador) y 1b (entrenamiento con goroutines: reducción, mutex, CAS). B: maestro TCP que registra nodos | ✅ |
| 8 | Examen parcial | **N: contrato del formato binario + `exportar-muestra`**. **N: Pthreads** (cargador productor-consumidor) y **OpenMP** (gradiente con `reduction`). Validación: misma pérdida que Go | pendiente, prioridad para el parcial |
| 8 | Examen parcial | **N: MPI** (entrenamiento punto a punto y colectivo con `mpiexec -n 1..8`) | pendiente, prioridad para el parcial si MS-MPI queda instalado |
| 8 | Arquitecturas paralelas (sílabo sem. 2) | **T: inventario del clúster** (CPU, núcleos, RAM, GPU, sistema operativo e IP de cada PC) y elegir el sistema operativo común para MPI | pendiente |

Si no hay tiempo de tener MPI, Pthreads y OpenMP para la entrega del parcial, se entregan en la semana 9 y el parcial se defiende con la tabla de equivalencias de [docs/conceptos_curso.md](../docs/conceptos_curso.md).

### Fase 3 · Semanas 9-12 · Distribución, API y GPU

| Sem. | Sílabo | Trabajo del proyecto | Entregable |
|---|---|---|---|
| 9 | OpenMP: tareas y balanceo de carga | **N: OpenMP `task` + `schedule`** para rezagos y medias móviles por estación (Experimento 6). B: entrenamiento distribuido síncrono en Go con 2 nodos, primero locales y luego **2 PCs reales del clúster**. C: `/predicciones` y lanzar/consultar entrenamiento | Distribuido funcionando; variables de rezago calculadas |
| 10 | Descomposición de datos y tareas; balanceo y eficiencia | **T: clúster MPI armado** (MS-MPI con `smpd` u OpenMPI con `hostfile`) y **ping-pong** (Experimento 7a). **N: Experimento 7 (MPI frente a TCP en Go)** en una PC y en el clúster. B: sección crítica del maestro con mutex y con atómicos. C: JWT con dos roles | Las dos estrategias pasando `-race`; tabla MPI frente a TCP |
| 11 | CUDA: arquitectura, modelo y kernels | **N: kernel CUDA del gradiente** (versión ingenua con `atomicAdd`), compilado y validado contra la pérdida de Go en una PC del grupo con NVIDIA. B: modo asíncrono y caída de nodos. C: WebSocket con pérdida por época | Primer kernel correcto; curva de pérdida en el navegador |
| 12 | CUDA: memoria compartida, reducción, warp shuffles | **N: reducción en memoria compartida y con warp shuffles (Experimento 8).** T: Experimentos 2, 3 y 4; Experimento 2 en el clúster físico. **N: híbrido MPI + OpenMP + CUDA** con reparto proporcional (Experimento 9). C: Redis | Tablas de los experimentos 2-8; latencias p50/p95/p99 |

### Fase 4 · Semanas 13-16 · Algoritmos en GPU, integración y exposición

| Sem. | Sílabo | Trabajo del proyecto | Entregable |
|---|---|---|---|
| 13 | GPU: sorting y MapReduce; buscar concurrencia; evaluación empírica de la aceleración | **N: `sort_by_key`/`reduce_by_key` (Thrust)** para ordenar por (estación, hora) y estadísticas por estación. D: panel de rendimiento con todas las tecnologías y el clúster. B: métricas del modelo. T: `docker compose up` levanta todo. **Exposiciones** | Sistema integrado; borrador del informe |
| 14 | Estructura de un algoritmo paralelo; IA y deep learning en GPU | T: prueba de carga sobre la API; ensayo de la demo (clúster Go + `mpiexec` + CUDA en una PC con NVIDIA). **Exposiciones** | Informe final con los tres planos de evaluación |
| 15 | Estructuras de soporte | Exposición y demo en vivo | Proyecto entregado |
| 16 | Examen final | — | — |

### Hitos que no se pueden mover

| Semana | Hito | Por qué es crítico |
|---|---|---|
| 3 | Contratos entre partes acordados ✅ | Sin esto, nadie puede trabajar sin bloquear a los demás. |
| 4 | T(1) medido ✅ | Sin línea base secuencial no hay speedup que reportar. |
| 7 | Experimento 1 terminado ✅ | Es el resultado más limpio del proyecto. |
| 8 | Formato binario común y primer núcleo nativo validado | Todos los núcleos nativos dependen de él. |
| 9 | Entrenamiento distribuido en Go con 2 nodos | Es el componente central del sistema. |
| 10 | Clúster MPI funcionando entre al menos 3 PCs (ping-pong medido) | Los experimentos 7, 7a y 9 dependen de él. |
| 11 | CUDA Toolkit instalado y probado en las 2 PCs del grupo con NVIDIA | Sin esto no hay CUDA. Respaldo: Colab. |
| 12 | Los experimentos 2-9 corridos | Lo que se expone son estos resultados. |
| 13 | `docker compose up` levanta todo | La demo en vivo depende de esto. |

### Reglas para no atrasarse

- Cada parte se desarrolla contra una versión simulada (mock) de las otras. Los núcleos nativos se desarrollan contra el archivo binario, sin depender de Mongo ni de la red.
- `go test -race ./...` en cada commit de Go. En C: compilar con `-Wall -Wextra` y probar los programas de Pthreads y OpenMP con ThreadSanitizer (`-fsanitize=thread`) en Linux.
- Cada núcleo nativo se valida primero contra la pérdida de Go (corrección) y después se mide (rendimiento). Un núcleo rápido pero con otra respuesta no cuenta.
- El cargador nunca carga el archivo entero en memoria. Bloques de tamaño fijo, siempre.
- Si el speedup se aplana antes de lo esperado, no es un fallo. Se explica con Amdahl y se reporta. Es justamente el análisis que el curso pide.

## 11. Bibliografía y repositorio del curso

Se usa solo como referencia para verificar qué temas contempla el proyecto, no como fuente de fechas ni de evaluación.

- **Bibliografía principal:** Cutajar, *Learn Concurrent Programming with Go* (Manning, 2024); Pacheco & Malensek, *An Introduction to Parallel Programming* (2nd ed., Morgan Kaufmann, 2022) — disponible en biblioteca FISI; Herlihy et al., *The Art of Multiprocessor Programming*; Hwu, Kirk & El Hajj, *Programming Massively Parallel Processors*; entre otros (ver repo del curso).
- **Repositorio del curso** (programas de MPI/Pthreads/OpenMP/CUDA de laboratorio): https://github.com/FISI-SM/parallelProgramming-repo/tree/main

## 12. Tecnologías del sílabo en el proyecto: MPI, Pthreads, OpenMP y CUDA

Hasta el 13 de octubre de 2026 estas cuatro tecnologías estaban fuera de alcance y el proyecto solo implementaba sus equivalentes en Go. **Desde esa fecha se implementan todas**, como núcleos de cómputo en `nativo/` que resuelven partes reales del sistema (ver §6, "Núcleos nativos"). Las versiones en Go se mantienen, porque son el sistema que funciona de punta a punta. La comparación entre ambas es parte del análisis.

| Semana del sílabo | Tema | Implementación en SACA | Equivalente en Go (ya hecho) |
|---|---|---|---|
| 3 | MPI: envío y recepción | `nativo/mpi/entrenamiento --modo p2p`: el rank 0 envía particiones y pesos con `MPI_Send` y recibe gradientes con `MPI_Recv` | Maestro y trabajador TCP (`internal/protocolo`) |
| 4 | MPI: broadcast, scatter, gather, reducciones | `--modo colectivo`: `MPI_Scatterv` de la muestra, `MPI_Bcast` de pesos y `MPI_Allreduce` de gradientes | Reducción de gradientes en el maestro |
| 5 | Pthreads: hilos y mutex | `nativo/pthreads/cargador`: `pthread_create/join`; conteos protegidos con `pthread_mutex_t` | Cargador concurrente |
| 6 | Pthreads: productor-consumidor, barreras, variables de condición | Buffer circular acotado con `pthread_cond_wait/signal` entre lectores y agregadores; `pthread_barrier_t` entre fases | Channels con buffer y `WaitGroup` |
| 7 | OpenMP: directivas, variables privadas y compartidas, reducciones | `nativo/openmp/entrenamiento`: `#pragma omp parallel for reduction(+:grad[:d])` | `gradienteReduccion` (Experimento 1b) |
| 9 | OpenMP: tareas, planificación y balanceo | `nativo/openmp/variables`: una `task` por estación; `schedule(static/dynamic/guided)` | — (nuevo) |
| 10 | Descomposición de datos y tareas | Partición por estación (datos) frente a partición por fase (tareas), medida en los experimentos 6 y 7 | `internal/particion` |
| 11 | CUDA: arquitectura, kernels | `nativo/cuda/gradiente`: un hilo por ejemplo, `atomicAdd` al gradiente | — |
| 12 | CUDA: memoria compartida, reducción, warp shuffles | Reducción por bloques en `__shared__` y luego con `__shfl_down_sync` | — |
| 13 | Sorting paralelo y MapReduce en GPU | `nativo/cuda/estaciones`: `thrust::sort_by_key` por (estación, hora) y `reduce_by_key` para estadísticas por estación | Agregación por estación en MongoDB (`exportar`) |

**Entorno de cada tecnología:**

| Tecnología | Windows (PC de desarrollo) | Linux / Docker | Comprobado |
|---|---|---|---|
| Pthreads | gcc 13.2 de MSYS2 (`C:\msys64\ucrt64`), `-pthread` | gcc | ✅ `pthread.h` disponible |
| OpenMP | gcc 13.2 de MSYS2, `-fopenmp` | gcc | ✅ `omp.h` disponible |
| MPI | Instalar **MS-MPI** (runtime + SDK, necesita administrador) y `pacman -S mingw-w64-ucrt-x86_64-msmpi` | `apt install openmpi-bin libopenmpi-dev`; o contenedores con OpenMPI en Docker Compose | ❌ falta instalar |
| CUDA | Solo en las **PCs del grupo con GPU NVIDIA**, con CUDA Toolkit (`nvcc`) y Visual Studio Build Tools (compilador `cl.exe` que usa `nvcc` en Windows). La PC de desarrollo principal tiene una AMD Radeon RX 580, que no ejecuta CUDA: ahí el código se escribe pero no se compila | En Linux con NVIDIA: `nvcc` del CUDA Toolkit. Respaldo: Google Colab (GPU T4) con `nativo/cuda/SACA_CUDA.ipynb` | ⏳ código escribible aquí; pruebas en las PCs con NVIDIA |

**Clúster (varias PCs):**

| Forma | Windows | Linux |
|---|---|---|
| Go por TCP | `trabajador.exe -maestro IP:9000`; abrir el puerto 9000 | ídem (compilado con `GOOS=linux`) |
| MPI entre PCs | MS-MPI, la misma versión en todas; `smpd -d` en cada PC; `mpiexec -hosts N ip1 1 ip2 1 ...`; ejecutable en la misma ruta; reglas de firewall | OpenMPI; SSH sin contraseña entre todas; `mpirun --hostfile` |
| Híbrido + CUDA | `nvcc` (con `cl.exe`) + `msmpi.lib` en los 2 nodos GPU | `nvcc -ccbin mpicxx` |

Detalle completo en [docs/guia_cluster.md](../docs/guia_cluster.md).

**Flujo de trabajo de CUDA:**

1. El código se escribe y se revisa en cualquier PC. El build no forma parte de `go build`, así que no rompe nada en las máquinas sin NVIDIA.
2. En una PC con NVIDIA:
   - `git pull`;
   - copiar `data/muestra.bin` (o regenerarlo con `go run ./herramientas/cmd/exportar-muestra`, que necesita los datos y Mongo);
   - `nvcc -O3 -o gradiente nativo/cuda/gradiente.cu`;
   - correr.
3. Validar primero que la pérdida final coincida con la de Go (corrección) y después medir.
4. Subir el CSV de resultados al repositorio. El análisis y el frontend lo toman igual que los demás.

Los tiempos de la GPU se comparan con los de OpenMP **medidos en la misma PC con NVIDIA**, no en la PC de desarrollo, para que la comparación CPU frente a GPU sea justa. En el CSV se anotan el modelo de GPU y de CPU de esa máquina.

Las métricas y la ley de Amdahl (semanas 1, 10 y 13 del sílabo) son la base común de los 8 experimentos y del panel de rendimiento (§8).

## 13. Riesgos y cómo se mitigan

- **El volumen de datos no cabe en memoria** → el cargador nunca carga el archivo completo; procesa por bloques y escribe por lotes. La memoria consumida depende del tamaño del bloque, no del tamaño del archivo.
- **Clases muy desbalanceadas** (el modelo aprende a decir siempre que no habrá alerta) → umbral de AQI 51 en lugar de 101, ponderación de la clase minoritaria en la función de pérdida, evaluación por F1 y AUC-ROC en lugar de exactitud.
- **El speedup se aplana antes de lo esperado** → es un resultado válido y esperable, no un fallo. Se analiza con la ley de Amdahl identificando la fracción secuencial y el costo de comunicación entre nodos.
- **Nodos trabajadores con velocidades distintas que retrasan cada época** → particionado por bloques más pequeños y reparto dinámico, más la opción de agregación asíncrona.
- **La PC de desarrollo principal no tiene GPU NVIDIA** (es una AMD RX 580) → el código CUDA se escribe en cualquier PC y se compila y mide en las computadoras del grupo que tienen NVIDIA. Las comparaciones CPU/GPU se hacen en esa misma máquina. Antes de la semana 11 se instala y prueba el CUDA Toolkit en al menos una de ellas; Colab queda como respaldo.
- **MPI no está instalado en Windows y requiere permisos de administrador** → instalar MS-MPI una vez. Si no es posible, se usa OpenMPI en la PC Linux del grupo o en contenedores Docker.
- **Las implementaciones en distintos lenguajes dan resultados distintos** → todas leen el mismo archivo binario y se validan contra la pérdida final de Go (tolerancia 1e-6) antes de medir tiempos.
- **Demasiadas tecnologías para el tiempo disponible** → cada núcleo nativo es pequeño (un archivo de C de unos cientos de líneas) y no reemplaza al sistema en Go. Si uno se atrasa, el sistema sigue funcionando y la comparación de ese núcleo se reporta en la siguiente entrega.
- **El clúster da speedup bajo o negativo** → es esperable con la muestra actual, porque la latencia de red pesa frente a ~12 ms de cálculo por época. Se mide con el ping-pong, se explica con un modelo de costo (cálculo/p + latencia × mensajes) y se repite con un problema más grande. Se usa cable y no WiFi para las mediciones finales.
- **PCs con sistemas operativos o versiones de MPI distintos** → un trabajo MPI requiere el mismo sistema operativo y la misma versión de MPI en todas. Se elige uno al hacer el inventario (semana 8). La forma Go por TCP sí funciona con cualquier mezcla.
- **Firewalls y redes de la universidad que bloquean puertos** → probar en una red propia (router del grupo). Abrir solo los puertos necesarios y solo en el perfil de red privada.
- **Errores de concurrencia en C** (carreras, deadlocks con variables de condición, olvidar `pthread_join`) → ThreadSanitizer en Linux, y la regla de siempre esperar la condición dentro de un `while`.
- **Errores clásicos de concurrencia en Go** (goroutines filtradas, channels sin cerrar, accesos concurrentes al modelo) → uso sistemático del detector de carreras de Go (`-race`) durante el desarrollo, y revisión del catálogo de errores documentado en la literatura de concurrencia en Go (ver GoBench, §16).

## 14. Qué hace distinto a este proyecto

- Los datos son públicos, reales, verificables y descargables por cualquiera desde el portal de la EPA. No hay datos sintéticos ni simulados.
- Se ejercitan las dos dimensiones del curso en un mismo sistema: concurrencia de memoria compartida dentro de cada nodo, y distribución por paso de mensajes entre nodos.
- La sincronización no es un detalle de implementación sino un objeto de estudio: se implementan dos estrategias y se comparan con mediciones.
- El resultado es demostrable en vivo: se levanta el clúster, se lanza un entrenamiento y se ve la curva de pérdida y la telemetría moverse en el navegador en tiempo real.
- La arquitectura es trasladable a la red de monitoreo de calidad del aire de Lima sin cambios estructurales.

## 15. Puntos a consultar con el docente

1. ~~Si esperan una implementación comparativa en OpenMP, MPI o CUDA además de la versión en Go~~ → **decidido por el grupo (13 de octubre de 2026): se implementan las cuatro**. Queda por confirmar si el docente prefiere ver los núcleos nativos ya en el avance del parcial o en la entrega final.
2. Para CUDA se usarán las PCs del grupo con GPU NVIDIA. Consultar si el docente quiere que la demo de CUDA sea en vivo (llevando una de esas PCs) o si basta con los resultados medidos.
3. Confirmar si la exposición debe incluir una demostración en vivo del clúster en ejecución o si basta con la presentación de resultados y mediciones.

## 16. Referencias IEEE

Organizadas según el componente de la arquitectura al que sustentan. La ficha de origen (`Articulos_IEEE_Proyectos_UNMSM_PConcurrenteyParalela.docx.pdf`, Proyecto 9 de 9 propuestos por el curso) seleccionó 10 artículos IEEE agrupados en 4 bloques.

**Grupo 1 — Entrenamiento distribuido y arquitecturas parameter server (Parte B)**

1. K. Zhang, S. Alqahtani y M. Demirbas, "A Comparison of Distributed Machine Learning Platforms", Proc. ICCCN 2017, IEEE. https://ieeexplore.ieee.org/abstract/document/8038464 — Compara el diseño arquitectónico de plataformas de ML distribuido (Spark, PMLS, TensorFlow, MXNet) e identifica cuellos de botella de comunicación y control. Sustenta la decisión de estructurar el sistema como maestro y nodos trabajadores.
2. H. Wang, D. Niu y B. Li, "Distributed Machine Learning with a Serverless Architecture", Proc. IEEE INFOCOM 2019, pp. 1288-1296. https://ieeexplore.ieee.org/document/8737391/ — Presenta SIREN, framework asíncrono de ML distribuido basado en funciones serverless, hasta 44 % menos tiempo de entrenamiento. Aporta la alternativa de despliegue frente al nodo maestro fijo.
3. K. Bäckström, I. Walulya, M. Papatriantafilou y P. Tsigas, "Consistent Lock-free Parallel Stochastic Gradient Descent for Fast and Stable Convergence", Proc. IPDPS 2021 (mención honorífica al mejor artículo). https://ieeexplore.ieee.org/document/9460457/ — Propone Leashed-SGD, esquema de sincronización sin bloqueos para SGD paralelo en memoria compartida. Es la base directa de la estrategia sin locks que el proyecto implementa y compara contra la versión con mutex.

**Grupo 2 — Predicción espacio-temporal en entornos urbanos**

4. S. Liao, J. Chen et al., "Large-Scale Short-Term Urban Taxi Demand Forecasting Using Deep Learning", Proc. ASP-DAC 2018, IEEE (artículo invitado). https://ieeexplore.ieee.org/document/8297361/ — Muestra que los modelos profundos solo superan a las técnicas tradicionales cuando la arquitectura incorpora conocimiento del dominio sobre patrones espacio-temporales. Justifica priorizar un buen diseño de variables sobre un modelo complejo.
5. Y. Lai, K. Zhang, J. Lin, F. Yang y Y. Fan, "Taxi Demand Prediction with LSTM-Based Combination Model", Proc. IEEE ISPA/BDCloud/SocialCom/SustainCom 2019, pp. 944-950. https://ieeexplore.ieee.org/abstract/document/9047248/ — Combina un componente espacio-temporal con un componente de atributos externos. Es el criterio que se traslada al diseño de features: rezagos propios + contaminantes cruzados + contexto de la estación.
6. C. Zhang, F. Zhu, Y. Lv, P. Ye y F. Wang, "MLRNN: Taxi Demand Prediction Based on Multi-Level Deep Learning and Regional Heterogeneity Analysis", IEEE Trans. Intelligent Transportation Systems, vol. 23, num. 9, pp. 8412-8422, 2022. https://ieeexplore.ieee.org/document/9439926/ — Agrupa zonas con patrones similares y combina predicción por clúster con predicción global. Sustenta que las estaciones de monitoreo no son homogéneas.

**Grupo 3 — Microservicios, contenedores y almacenamiento en memoria (despliegue con Docker, Parte D)**

7. R. Rufino, M. Alam, J. Ferreira, A. Ur Rehman y K. F. Tsang, "Orchestration of Containerized Microservices for IIoT Using Docker", Proc. ICIT 2017, pp. 1532-1536. https://ieeexplore.ieee.org/document/7915594/ — Arquitectura modular basada en virtualización ligera donde la orquestación de contenedores habilita despliegues distribuidos con tolerancia a fallos. Es el patrón que sigue el Docker Compose del proyecto.
8. J. Stubbs, W. Moreira y R. Dooley, "Distributed Systems of Microservices Using Docker and Serfnode", Proc. 2015 7th International Workshop on Science Gateways, IEEE. https://ieeexplore.ieee.org/document/7217926/ — Introduce Serfnode, mecanismo descentralizado de descubrimiento de servicios para clústeres de contenedores. Referencia para escalar el número de nodos trabajadores sin reconfigurar manualmente el maestro.
9. S. Chen, X. Tang, Z. Wang, H. Zhao y M. Guo, "Towards Scalable and Reliable In-Memory Storage System: A Case Study with Redis", Proc. 2016 IEEE Trustcom/BigDataSE/ISPA, pp. 1660-1667. https://ieeexplore.ieee.org/document/7847138/ — Estudia los límites de rendimiento del clúster de Redis y propone un método de cache cliente-nodo. Sustenta el diseño de la capa de cache de predicciones.

**Grupo 4 — Comunicación en tiempo real con el frontend (Parte C)**

10. V. Pimentel y B. G. Nickerson, "Communicating and Displaying Real-Time Data with WebSocket", IEEE Internet Computing, vol. 16, num. 4, pp. 45-53, 2012. https://ieeexplore.ieee.org/document/6197172/ — Mide la latencia de transmisión en tiempo real comparando WebSocket contra HTTP polling y long polling. Justificación técnica de servir telemetría y alertas por WebSocket.

**Referencias transversales sobre concurrencia en Go**

11. J. Whitney, C. Gifford y M. Pantoja, "Distributed execution of communicating sequential process-style concurrency: Golang case study", The Journal of Supercomputing, vol. 75, num. 3, pp. 1396-1409, 2019. DOI: 10.1007/s11227-018-2649-2 (Springer, no IEEE). https://link.springer.com/article/10.1007/s11227-018-2649-2 — Go ofrece concurrencia nativa al estilo CSP mediante goroutines y channels, pero no incluye soporte para distribuir goroutines entre máquinas. Los autores construyen una biblioteca (Gluster) que cumple, para Go, un rol equivalente al de MPI. Explica por qué el proyecto debe implementar su propia capa TCP maestro-trabajador.
12. N. Dilley y J. Lange, "An Empirical Study of Messaging Passing Concurrency in Go Projects", Proc. IEEE SANER 2019, pp. 377-387. DOI: 10.1109/SANER.2019.8668036. https://ieeexplore.ieee.org/document/8668036/ — Analiza cómo se usa realmente la concurrencia por paso de mensajes en 865 proyectos reales de Go en GitHub. De aquí se toman los patrones de diseño del cargador: worker pool, fan-out y fan-in.
13. T. Yuan, G. Li, J. Lu, C. Liu, L. Li y J. Xue, "GoBench: A Benchmark Suite of Real-World Go Concurrency Bugs", Proc. IEEE/ACM CGO 2021. DOI: 10.1109/CGO51591.2021.9370317. https://ieeexplore.ieee.org/document/9370317/ — Primer banco de pruebas de errores de concurrencia en Go a partir de 82 casos reales de aplicaciones open source. Catálogo de errores que el equipo revisa para no reproducirlos (channels sin cerrar, goroutines fugadas, condiciones de carrera).

**Implementación de referencia:** repositorio NeuroOps / NYC-Taxi-Demand, desarrollado por estudiantes de la UPC sobre el conjunto de viajes de taxi de Nueva York: https://github.com/YairJeri/NYC-Taxi-Demand — Implementa el patrón parameter server sobre TCP (puerto 9090), expone una API REST (puerto 8080), usa MongoDB para el dataset procesado e historial de entrenamientos, Redis para cachear inferencias, y un panel web con telemetría del clúster en tiempo real, todo desplegado con Docker Compose. Prueba de que el alcance propuesto es alcanzable por un equipo de estudiantes en un semestre.

## 17. Cronograma explicado en simple

La versión de §10 es la técnica, para trabajar. Esta es la misma idea semana por semana, en lenguaje llano, útil para explicarle el avance a alguien que no necesita los detalles de implementación.

- **Semana 3.** Bajamos los datos reales de contaminación del aire de la página del gobierno de EEUU — más de 16 millones de filas — y dejamos escrito, antes de programar nada, cómo se van a comunicar entre sí las distintas partes del sistema.
- **Semana 4.** Construimos la primera versión, la más simple posible, de las dos piezas centrales: un programa que limpia y ordena esos 16 millones de datos, y un programa que aprende a predecir si el aire se va a poner malo. Las dos funcionan de principio a fin con datos reales, pero de la forma más lenta posible a propósito — esa lentitud es la referencia contra la que se compara todo lo que viene después.
- **Semanas 5-6.** Hacemos que el programa que limpia los datos use varios procesos a la vez en lugar de uno solo, para que sea más rápido. Los datos ya limpios quedan guardados en una base de datos.
- **Semana 7.** Medimos cuánto más rápido es usar 1, 2, 4, 8 o 16 procesos a la vez frente a uno solo, y armamos la primera gráfica de resultados.
- **Semana 8.** Se entrega el avance del parcial. Además, se escriben las mismas piezas de cálculo con las herramientas "clásicas" que enseña el curso en C: hilos (Pthreads), directivas (OpenMP) y mensajes entre procesos (MPI), y se comprueba que dan exactamente la misma respuesta que la versión en Go.
- **Semana 9.** En lugar de que una sola computadora aprenda con todos los datos, se reparte el aprendizaje entre varias computadoras que colaboran entre sí. Con OpenMP se calculan los datos del pasado de cada estación (las últimas 24 horas), repartiendo bien el trabajo, porque algunas estaciones tienen muchos más datos que otras.
- **Semana 10.** Se conectan las computadoras del grupo en red para formar un pequeño "superordenador" (clúster), y se mide qué tan rápido se pueden enviar datos entre ellas. Se prueban dos formas de evitar que las computadoras se pisen entre sí al actualizar lo que aprenden juntas, y se compara la versión en Go con la de MPI.
- **Semana 11.** Se usa por primera vez la tarjeta gráfica (GPU) con CUDA, en las computadoras del grupo que tienen una tarjeta NVIDIA, y se puede ver desde el navegador cómo va aprendiendo el sistema en tiempo real.
- **Semana 12.** Se optimiza el programa de la GPU y se corren todos los experimentos de rendimiento: cuántas veces más rápido es cada tecnología y por qué. El entrenamiento se reparte entre todas las computadoras del grupo a la vez: las 2 con tarjeta gráfica NVIDIA usan la GPU y las demás, su procesador. A cada una se le da trabajo según qué tan rápida es.
- **Semana 13.** Se ordenan y resumen millones de datos directamente en la GPU, se juntan todas las piezas en un sistema que funciona de principio a fin, y se mide qué tan buenas son las predicciones.
- **Semana 14.** Se prueba que el sistema aguante muchas consultas a la vez y se ensaya la demostración en vivo.
- **Semana 15.** Exposición final con demostración en vivo.
- **Semana 16.** Examen final.

---

*Este documento reemplaza y consolida `proyecto9_introduccion_y_cronograma.md` y `proyecto9_notebooklm.txt`. El sílabo completo (`SILABO 2026-2 ...pdf`) y la ficha de artículos IEEE de los 9 proyectos del curso (`Articulos_IEEE_Proyectos_UNMSM_PConcurrenteyParalela.docx.pdf`) se conservan como fuente primaria por si hace falta un detalle no recogido aquí.*
