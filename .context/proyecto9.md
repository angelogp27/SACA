# Proyecto 9 — Sistema de alerta temprana de contaminación del aire

Curso: Programación Concurrente y Paralela (202W0805) · UNMSM, Escuela Profesional de Ingeniería de Software · Semestre 2026-2 · Ciclo VIII

Docentes: Javier Antonio Prudencio Vidal (jprudenciov@unmsm.edu.pe), Pablo Edwin López Villanueva (plopezv@unmsm.edu.pe). Prerrequisitos: Sistemas Operativos, Redes y Transmisión de Datos.

Este documento consolida todo el material de referencia del proyecto (ficha técnica, cronograma y contexto del sílabo) que antes estaba repartido en varios archivos.

---

## 1. Resumen ejecutivo

Se construye un sistema distribuido, escrito íntegramente en Go y desplegado con Docker, que procesa de forma concurrente más de quince millones de mediciones horarias reales de calidad del aire de la red de monitoreo de Estados Unidos (EPA), entrena de manera distribuida un modelo de aprendizaje automático sobre un clúster de nodos trabajadores, y expone en tiempo real una predicción útil para la salud pública: en qué estaciones y en qué horas del día siguiente el aire alcanzará un nivel de contaminación que obliga a tomar precauciones.

El proyecto combina las dos dimensiones que exige el curso:

- **Paralelismo** — varias unidades de ejecución trabajando simultáneamente dentro de una misma máquina, mediante goroutines y channels.
- **Distribución** — varios procesos en máquinas o contenedores distintos cooperando para entrenar un único modelo, mediante un esquema maestro-trabajador sobre TCP.

## 2. Fundamentos, para quien recién empieza en paralelismo

**Secuencial vs. concurrente vs. paralelo vs. distribuido.** Un programa secuencial hace una cosa a la vez: si tiene que recorrer 15 millones de líneas, las procesa una por una y el tiempo total es la suma de todos esos pasos. Una computadora moderna tiene varios núcleos (4, 8, 16); con un programa secuencial, uno solo trabaja y el resto está ocioso.

| Término | Qué significa | Ejemplo cotidiano |
|---|---|---|
| **Concurrencia** | Varias tareas *en progreso* al mismo tiempo, aunque no necesariamente se ejecuten en el mismo instante. Es una forma de organizar el programa. | Un cocinero que pone a hervir agua, mientras espera pica cebolla, y vuelve a la olla. Una sola persona, varias tareas en curso. |
| **Paralelismo** | Varias tareas ejecutándose *realmente al mismo tiempo* en distintos núcleos. Es una propiedad del hardware. | Cuatro cocineros, cada uno en su propia hornilla. |
| **Distribución** | Varias computadoras (o contenedores) distintas cooperando por red para resolver un solo problema. No comparten memoria, solo se mandan mensajes. | Cuatro cocinas en cuatro edificios distintos que se coordinan por teléfono. |

El curso enseña dos modelos de paralelismo, y el proyecto usa uno de cada uno:

- **Memoria compartida**: varios hilos en la misma máquina que ven las mismas variables. Es rápido, pero peligroso, porque dos hilos pueden pisarse al escribir en la misma variable. En el laboratorio se ve con Pthreads y OpenMP; en el proyecto, con goroutines de Go.
- **Paso de mensajes**: varios procesos que no comparten nada y se comunican enviándose datos. Es más lento pero escala a muchas máquinas. En el laboratorio se ve con MPI; en el proyecto, con conexiones TCP entre contenedores Docker.

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

### Persistencia

- **MongoDB**: almacenamiento persistente del conjunto de datos ya procesado, del historial de entrenamientos (hiperparámetros y métricas) y del registro de predicciones emitidas.
- **Redis**: capa de cache de baja latencia para predicciones precalculadas y respuestas frecuentes, más el estado compartido de la telemetría del clúster.

### Despliegue

Todo con **Docker Compose**, separando en dos grupos la infraestructura principal (API/maestro, MongoDB, Redis, frontend) de los nodos trabajadores, de modo que el número de nodos de cómputo se pueda escalar sin tocar el resto del sistema. Esa separación es la que permite ejecutar los experimentos de escalabilidad levantando más réplicas de trabajadores.

## 7. Tecnologías

| Área | Tecnología |
|---|---|
| Lenguaje único del backend | Go |
| Concurrencia interna | goroutines, channels, `sync.WaitGroup`, `sync.Mutex`, paquete `sync/atomic` |
| Comunicación entre nodos | TCP interno (maestro-trabajador) |
| Comunicación con el cliente | HTTP/REST y WebSocket |
| Seguridad | JWT |
| Base de datos persistente | MongoDB |
| Cache en memoria | Redis |
| Frontend | SPA en **React** |
| Empaquetado y orquestación | Docker y Docker Compose |
| Modelo de ML | Regresión logística por SGD, implementada desde cero en Go |

## 8. Cómo se evalúa el proyecto

Tres planos, y los tres se reportan.

**Plano 1 — Rendimiento paralelo y concurrente** (el central para el curso). Se establece primero una línea base estrictamente secuencial (T(1)), y contra ella se miden speedup, eficiencia y contraste con la ley de Amdahl (fracción secuencial irreducible: lectura del archivo, agregación final de gradientes, escrituras a base de datos). También escalabilidad fuerte (mismo volumen, más unidades) y débil (volumen creciente proporcional a las unidades), y balanceo de carga entre goroutines/nodos.

Experimentos concretos:

1. **Cargador**: tiempo total de ingesta de los 15M+ registros con 1, 2, 4, 8 y 16 goroutines trabajadoras.
2. **Entrenamiento**: tiempo por época con 1, 2, 4 y 8 nodos trabajadores.
3. **Sincronización**: mutex frente a operaciones atómicas, comparando tiempo por época y estabilidad de convergencia.
4. **Agregación**: modo síncrono frente a asíncrono.

**Plano 2 — Calidad del servicio.** Latencia p50/p95/p99 con y sin cache de Redis (objetivo <100ms), rendimiento bajo carga concurrente de clientes, comportamiento ante la caída de un nodo trabajador durante el entrenamiento.

**Plano 3 — Calidad del modelo.** Como es un problema binario y desbalanceado, no basta con la exactitud: se reportan precisión, exhaustividad, F1 y AUC-ROC, con partición temporal (entrenar con los primeros meses, evaluar con los últimos, nunca mezclando futuro con pasado). El objetivo del modelo es ser correcto y honesto, no ser estado del arte — el aporte del proyecto está en la arquitectura distribuida que lo entrena.

## 9. Organización del equipo

Diseño original: las cuatro partes están pensadas para avanzar simultáneamente entre 4 integrantes (uno por parte A/B/C/D), más trabajo transversal de Docker Compose, integración y ejecución de experimentos. Se acuerdan desde el inicio los contratos entre partes (esquema del registro procesado, protocolo TCP maestro-trabajador, contrato de la API REST) para que cada quien trabaje contra una versión simulada (mock) de las demás partes sin quedar bloqueado.

> Estado actual del proyecto: sin roles asignados todavía.

## 10. Cronograma del proyecto — semanas 3 a 15

Semanas 1 y 2 (ya transcurridas): definición de la idea del proyecto. El trabajo de proyecto que sigue arranca en la semana 3, que es también cuando el sílabo marca la primera revisión de avances.

Hitos fijos del sílabo, no ligados al arranque del proyecto: revisión de avances desde la semana 3, exposiciones en la 13, 14 y 15.

Convención: A = cargador, B = entrenamiento, C = API, D = frontend, T = transversal (Docker, integración, experimentos).

### Fase 1 · Semanas 3-4 · Datos, contratos y línea base secuencial

| Sem. | Trabajo del proyecto | Entregable de la semana |
|---|---|---|
| 3 | Descargar los cuatro `hourly_*.zip` de la EPA y confirmar el conteo real de registros (>15 M). Repartir las partes A/B/C/D. Acordar los tres contratos: esquema del registro procesado, protocolo TCP maestro-trabajador, contrato REST. Crear el repo con estructura Go y Docker Compose vacío. | Los cuatro CSV descargados con el conteo confirmado. Documento de contratos (un `.md` en el repo). Repo inicializado. |
| 4 | A: explorar los CSV, definir limpieza (nulos, qualifiers, unidades) y escribir el cargador **secuencial** completo (una goroutine). B: escribir la regresión logística **secuencial** en Go y entrenarla sobre una muestra. C: esqueleto REST con un endpoint `/health`. D: maqueta de la SPA. T: script que mide el tiempo de ambos. | Features y etiqueta cerradas por escrito, umbrales AQI verificados contra la tabla de la EPA. **T(1) del cargador y T(1) del entrenamiento** — línea base de todo el proyecto. |

### Fase 2 · Semanas 5-8 · Cargador concurrente y persistencia

| Sem. | Trabajo del proyecto | Entregable de la semana |
|---|---|---|
| 5 | A: partir el archivo en bloques ajustados al salto de línea; despachador + worker pool. C: MongoDB en Compose, modelo de documento del registro procesado. | Cargador concurrente que procesa un archivo completo con N goroutines configurable. |
| 6 | A: agregadora que escribe por lotes en MongoDB; cierre ordenado de channels; correr con `go run -race`. B: partir el dataset en particiones por nodo. D: mapa de estaciones con los metadatos del `annual_conc_by_monitor`. | Los 15 M de registros en MongoDB sin fugas de goroutines ni carreras. |
| 7 | T: **Experimento 1** con 1, 2, 4, 8 y 16 goroutines. Calcular speedup y eficiencia. B: protocolo TCP, maestro que acepta conexiones y registra nodos. | Tabla y gráfica de speedup del cargador. Primer análisis con Amdahl. |
| 8 | Semana de consolidación: cerrar y documentar lo avanzado en la Fase 2. | Informe de avance de la Fase 2 (medio ciclo). |

### Fase 3 · Semanas 9-12 · Entrenamiento distribuido y API

| Sem. | Trabajo del proyecto | Entregable de la semana |
|---|---|---|
| 9 | B: trabajador que recibe partición y parámetros, calcula gradiente parcial y lo devuelve. Maestro que agrega en modo **síncrono**. C: endpoints de predicción y de lanzar/consultar entrenamiento. | Entrenamiento distribuido funcionando con 2 nodos en Compose. |
| 10 | B: sección crítica protegida con **mutex**; luego versión con **atómicos** (CAS). C: JWT con dos roles. D: línea de tiempo observado vs predicho. | Las dos estrategias de sincronización implementadas y pasando `-race`. |
| 11 | B: modo **asíncrono** de agregación. Manejo de caída de un nodo. C: WebSocket con pérdida por época y telemetría (CPU, RAM) de cada nodo. | Curva de pérdida visible en tiempo real desde el navegador. |
| 12 | T: **Experimento 2** (1, 2, 4, 8 nodos), **Experimento 3** (mutex vs atómicos), **Experimento 4** (síncrono vs asíncrono). C: Redis como cache de predicciones. | Tablas y gráficas de los cuatro experimentos. Latencia p50/p95/p99 con y sin cache. |

### Fase 4 · Semanas 13-15 · Integración, métricas del modelo y exposición

| Sem. | Trabajo del proyecto | Entregable de la semana |
|---|---|---|
| 13 | D: panel de rendimiento con speedup y eficiencia. B: métricas del modelo (precisión, F1, AUC-ROC) con partición temporal. T: Compose completo, un solo `docker compose up`. **Exposiciones.** | Sistema integrado corriendo de punta a punta. Borrador del informe. |
| 14 | T: prueba de carga sobre la API. Ensayo de la demo en vivo (levantar clúster, lanzar entrenamiento, ver telemetría). Corregir lo que falle. **Exposiciones.** | Informe final con los tres planos de evaluación. Diapositivas. |
| 15 | Exposición y demo en vivo. | Proyecto entregado. |

### Hitos que no se pueden mover

| Semana | Hito | Por qué es crítico |
|---|---|---|
| 3 | Contratos entre partes acordados | Sin esto, nadie puede trabajar sin bloquear a los demás. |
| 4 | T(1) medido | Sin línea base secuencial no hay speedup que reportar. |
| 7 | Experimento 1 terminado | Es el resultado más limpio del proyecto; conviene tenerlo listo con margen antes de la Fase 3. |
| 9 | Entrenamiento distribuido con 2 nodos | Es el componente central; si se retrasa, todo lo demás se cae. |
| 12 | Los cuatro experimentos corridos | Lo que se expone son estos resultados. Después de la semana 12 ya no hay margen para medir. |
| 13 | `docker compose up` levanta todo | La demo en vivo depende de esto. |

### Reglas para no atrasarse

- Cada parte se desarrolla contra una versión simulada (mock) de las otras. La Parte D no espera a que B esté lista para dibujar la curva de pérdida; usa datos falsos hasta la semana 11.
- `go build -race` y `go test -race` desde la semana 5. Una condición de carrera detectada en la semana 14 no se arregla a tiempo.
- El cargador nunca carga el archivo entero en memoria. Bloques de tamaño fijo, siempre.
- Si el speedup se aplana antes de lo esperado, no es un fallo. Se explica con Amdahl y se reporta. Es justamente el análisis que el curso pide.

## 11. Bibliografía y repositorio del curso

Se usa solo como referencia para verificar qué temas contempla el proyecto, no como fuente de fechas ni de evaluación.

- **Bibliografía principal:** Cutajar, *Learn Concurrent Programming with Go* (Manning, 2024); Pacheco & Malensek, *An Introduction to Parallel Programming* (2nd ed., Morgan Kaufmann, 2022) — disponible en biblioteca FISI; Herlihy et al., *The Art of Multiprocessor Programming*; Hwu, Kirk & El Hajj, *Programming Massively Parallel Processors*; entre otros (ver repo del curso).
- **Repositorio del curso** (programas de MPI/Pthreads/OpenMP/CUDA de laboratorio): https://github.com/FISI-SM/parallelProgramming-repo/tree/main

## 12. Fuera de alcance: MPI, Pthreads, OpenMP, CUDA

Ninguna de estas cuatro tecnologías se implementa en el proyecto. El proyecto es íntegramente Go (goroutines, channels, TCP — ver §7). MPI, Pthreads, OpenMP y CUDA son temario de laboratorio del curso, no entregables del proyecto. La tabla siguiente documenta solo la correspondencia conceptual entre lo que enseña cada semana de laboratorio y la pieza equivalente que el proyecto construye en Go: es material de estudio para entender el paralelismo, no una lista de tecnologías a instalar o usar en el proyecto.

| Tema de laboratorio (fuera de alcance del proyecto) | Equivalente que sí se implementa en Go |
|---|---|
| MPI (semanas 3-4 del sílabo): enviar y recibir mensajes, broadcast, reduce | Parte B. El maestro hace un *broadcast* de parámetros y un *reduce* (suma) de gradientes sobre TCP, sin usar MPI. |
| Pthreads (semanas 5-6): hilos, mutex, productor-consumidor | Parte A. El cargador es un productor-consumidor con goroutines y channels. La sección crítica del maestro se protege con `sync.Mutex`, no con pthread_mutex. |
| OpenMP (semanas 7, 9): reducciones, balanceo de carga | Parte A y B. Balanceo de carga entre goroutines y entre nodos, y reducción de gradientes, resuelto con channels y el paquete `sync/atomic`. |
| CUDA (semanas 11-13) | No se implementa. Ver §15, punto 1: pendiente de confirmar con el docente si esperan una comparativa. |

Las métricas y la ley de Amdahl (semanas 1, 10, 13 del sílabo) sí son parte del proyecto: son la base de los cuatro experimentos y el panel de rendimiento (§8, §10).

## 13. Riesgos y cómo se mitigan

- **El volumen de datos no cabe en memoria** → el cargador nunca carga el archivo completo; procesa por bloques y escribe por lotes. La memoria consumida depende del tamaño del bloque, no del tamaño del archivo.
- **Clases muy desbalanceadas** (el modelo aprende a decir siempre que no habrá alerta) → umbral de AQI 51 en lugar de 101, ponderación de la clase minoritaria en la función de pérdida, evaluación por F1 y AUC-ROC en lugar de exactitud.
- **El speedup se aplana antes de lo esperado** → es un resultado válido y esperable, no un fallo. Se analiza con la ley de Amdahl identificando la fracción secuencial y el costo de comunicación entre nodos.
- **Nodos trabajadores con velocidades distintas que retrasan cada época** → particionado por bloques más pequeños y reparto dinámico, más la opción de agregación asíncrona.
- **Errores clásicos de concurrencia en Go** (goroutines filtradas, channels sin cerrar, accesos concurrentes al modelo) → uso sistemático del detector de carreras de Go (`-race`) durante el desarrollo, y revisión del catálogo de errores documentado en la literatura de concurrencia en Go (ver GoBench, §16).

## 14. Qué hace distinto a este proyecto

- Los datos son públicos, reales, verificables y descargables por cualquiera desde el portal de la EPA. No hay datos sintéticos ni simulados.
- Se ejercitan las dos dimensiones del curso en un mismo sistema: concurrencia de memoria compartida dentro de cada nodo, y distribución por paso de mensajes entre nodos.
- La sincronización no es un detalle de implementación sino un objeto de estudio: se implementan dos estrategias y se comparan con mediciones.
- El resultado es demostrable en vivo: se levanta el clúster, se lanza un entrenamiento y se ve la curva de pérdida y la telemetría moverse en el navegador en tiempo real.
- La arquitectura es trasladable a la red de monitoreo de calidad del aire de Lima sin cambios estructurales.

## 15. Puntos a consultar con el docente

1. Si esperan una implementación comparativa de alguno de los núcleos de cómputo en OpenMP, MPI o CUDA, además de la versión en Go con goroutines/channels/TCP.
2. Confirmar el tamaño de equipo esperado, dado que la arquitectura está dividida en cuatro partes paralelizables entre integrantes.
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
- **Semana 8.** Semana de poner en orden todo lo avanzado y entregar un informe de medio ciclo.
- **Semana 9.** Empieza la parte más avanzada: en lugar de que una sola computadora aprenda con todos los datos, se reparte el aprendizaje entre varias computadoras que colaboran entre sí.
- **Semana 10.** Se prueban dos formas distintas de evitar que esas computadoras se pisen entre sí cuando actualizan lo que van aprendiendo juntas, y se compara cuál funciona mejor.
- **Semana 11.** Se agrega la posibilidad de ver, desde un navegador y en tiempo real, cómo va aprendiendo el sistema.
- **Semana 12.** Se corren todos los experimentos de rendimiento y se mide qué tan rápido responde el sistema cuando muchas personas lo consultan a la vez.
- **Semana 13.** Se juntan todas las piezas en un solo sistema que funciona de principio a fin, y se mide qué tan buenas son realmente las predicciones — no solo qué tan rápido corre.
- **Semana 14.** Se prueba que el sistema aguante muchas consultas a la vez y se ensaya la demostración en vivo.
- **Semana 15.** Exposición final con demostración en vivo.

---

*Este documento reemplaza y consolida `proyecto9_introduccion_y_cronograma.md` y `proyecto9_notebooklm.txt`. El sílabo completo (`SILABO 2026-2 ...pdf`) y la ficha de artículos IEEE de los 9 proyectos del curso (`Articulos_IEEE_Proyectos_UNMSM_PConcurrenteyParalela.docx.pdf`) se conservan como fuente primaria por si hace falta un detalle no recogido aquí.*
