# Contrato — formato binario y resultados de los núcleos nativos

Define cómo Go entrega los datos a los programas en C/C++ de `nativo/` (Pthreads, OpenMP, MPI, CUDA) y cómo estos devuelven sus mediciones. Gracias a este contrato, cada núcleo nativo se puede desarrollar y probar **sin MongoDB, sin red y sin Go**: basta con el archivo binario.

Estado: **implementado** para el archivo 1 (`data/muestra.bin`, generado por `go run ./herramientas/cmd/exportar-muestra`, que deja también `data/muestra_referencia.json`). Los núcleos que ya lo leen están en `nativo/`: `openmp/entrenamiento`, `mpi/entrenamiento`, `pthreads/cargador` (lee los CSV de la EPA directamente) y `cuda/gradiente`. El archivo 2 (`series.bin`) sigue pendiente. Las utilidades comunes (lectura, F1/AUC, verificación contra la referencia, CSV) están en `nativo/comun/saca.h`.

Opciones que los núcleos agregaron a la interfaz común: `--ejemplos N` (recorta o repite la muestra hasta N ejemplos, para la escala del Experimento 8; se anota en `maquina` y no se compara con la referencia), `--schedule static|dynamic|guided` y `--secuencial` (OpenMP), `--modo colectivo|p2p` (MPI) y `--agregadoras K` (Pthreads).

## Reglas generales

- Todos los números son **little-endian**, el orden nativo de x86-64.
- Los reales son `float64` IEEE 754 (`double` en C).
- Las matrices se guardan **por filas** (row-major): el elemento (i, j) de una matriz n×d está en la posición `i*d + j`.
- Un dato faltante se representa con **NaN** (en C, `isnan(x)`).
- Cada archivo empieza con un encabezado de tamaño fijo que se lee con un solo `fread`.

## Archivo 1: `data/muestra.bin` (entrenamiento)

Es la muestra de entrenamiento: etiquetada (NO2 de la hora siguiente > 53 ppb), partida por mes (entrenamiento = meses 1-9, prueba = 10-12) y **ya normalizada** con las constantes del conjunto de entrenamiento. Un núcleo nativo no tiene que normalizar nada.

**Versión 2 (la actual).** La generan directamente desde los CSV, sin MongoDB, los dos cargadores:

```
bin/cargador_pthreads --hilos 8 --agregadoras 4 --bin data/muestra.bin
go run ./cargador/cmd/concurrente -sin-mongo -goroutines 16 -agregadoras 0 -bin data/muestra.bin
```

Los dos escriben **el mismo archivo byte a byte** (misma huella SHA-256) con cualquier número de hilos: el clúster compara esa huella antes de entrenar. Para lograrlo:

- las filas se ordenan por (estación, hora) antes de escribir;
- los monitores repetidos y las coordenadas se resuelven con reglas que no dependen del orden de llegada (`internal/aqs.Indice.Agregar`);
- la media y la desviación se suman secuencialmente en ese orden, sin FMA (`-ffp-contract=off` en C, conversiones explícitas en Go);
- el seno y coseno de hora/día/mes salen de una tabla generada desde Go (`nativo/comun/tabla_ciclica.h`), no de la `libm` de C.

La versión 2 tiene el mismo encabezado (con `versión = 2`) y el mismo cuerpo que la 1, y agrega al final lo que necesita el entrenamiento distribuido; `saca.h` lee las dos y simplemente ignora lo agregado:

| Bloque | Tipo y tamaño |
|---|---|
| medias de normalización | `float64` × d |
| desviaciones de normalización | `float64` × d |
| n_estaciones | `uint64` |
| ids de estación (ordenados) | `char[16]` × n_estaciones, rellenos con `\0` |
| estación de cada fila de entrenamiento | `uint32` × n_entrenamiento (índice en la tabla) |
| estación de cada fila de prueba | `uint32` × n_prueba |

La versión 1 (sin ese final) la sigue escribiendo `exportar-muestra` desde la antigua `muestra.gob`; con `-muestra data/muestra.bin` solo escribe la referencia.

### Encabezado (48 bytes)

| Offset | Tipo | Campo | Valor |
|---:|---|---|---|
| 0 | `char[4]` | magia | `"SACA"` |
| 4 | `uint32` | versión | `1` |
| 8 | `uint32` | tipo | `1` = muestra |
| 12 | `uint32` | d | número de variables (hoy 12, ver `dataset.NombresFeatures`) |
| 16 | `uint64` | n_entrenamiento | filas de entrenamiento |
| 24 | `uint64` | n_prueba | filas de prueba |
| 32 | `float64` | peso_clase_0 | `logistica.CalcularPesosClase` sobre el entrenamiento |
| 40 | `float64` | peso_clase_1 | ídem |

### Cuerpo (en este orden)

| Bloque | Tamaño en `float64` |
|---|---|
| X_entrenamiento | n_entrenamiento × d |
| y_entrenamiento (0 o 1) | n_entrenamiento |
| X_prueba | n_prueba × d |
| y_prueba | n_prueba |

```c
typedef struct {
    char     magia[4];
    uint32_t version, tipo, d;
    uint64_t n_entrenamiento, n_prueba;
    double   peso_clase_0, peso_clase_1;
} EncabezadoMuestra;   /* 48 bytes, sin relleno */
```

### Archivo de referencia: `data/muestra_referencia.json`

`exportar-muestra` entrena también la versión secuencial en Go con los parámetros por defecto y escribe el resultado esperado:

```json
{ "tasa": 0.1, "epocas": 200, "perdida_final": 0.125734, "pesos": [ ... ], "sesgo": -2.0031 }
```

**Regla de corrección:** con los mismos hiperparámetros (`tasa` 0,1, 200 épocas, pesos y sesgo iniciales en 0, descenso de gradiente por lote completo con pesos de clase), la pérdida final de cualquier núcleo nativo debe coincidir con `perdida_final` dentro de **1e-6**. Si no coincide, el núcleo no se mide.

El algoritmo exacto que hay que replicar está en `internal/logistica/logistica.go` (`GradienteParcial` y `Entrenar`).

## Archivo 2: `data/series.bin` (registros horarios por estación)

Es la entrada de `nativo/openmp/variables` (rezagos y medias móviles) y de `nativo/cuda/estaciones` (sort y MapReduce en GPU). Contiene los 10,8 M de registros procesados **sin ordenar**, tal como salen de MongoDB.

### Encabezado (32 bytes)

| Offset | Tipo | Campo |
|---:|---|---|
| 0 | `char[4]` | `"SACA"` |
| 4 | `uint32` | versión `1` |
| 8 | `uint32` | tipo `2` = series |
| 12 | `uint32` | n_estaciones |
| 16 | `uint64` | n_registros |
| 24 | `uint64` | reservado (0) |

### Cuerpo

| Bloque | Tipo y tamaño | Contenido |
|---|---|---|
| ids de estación | `char[16]` × n_estaciones | `"06-037-0002"` relleno con `\0` |
| estación | `uint32` × n_registros | índice en la tabla de ids |
| hora | `int64` × n_registros | horas desde 1970-01-01 UTC (`unix / 3600`) |
| no2_ppb, o3_ppm, so2_ppb, co_ppm | `float64` × n_registros cada uno | NaN si la estación no midió ese gas en esa hora |

El formato es de columnas separadas (struct of arrays) y no de un arreglo de structs, porque así lo necesitan las lecturas coalescentes en la GPU y la vectorización en OpenMP.

## Resultados: un CSV por experimento

Cada núcleo agrega **una fila por corrida** al CSV que se le indique con `--resultado archivo.csv`, escribiendo el encabezado si el archivo es nuevo. Las columnas son las mismas que en Go, para que `experimentos/cmd/analisis` los procese sin cambios.

**Entrenamiento** (OpenMP, MPI, CUDA), igual que `internal/entrenador`:

```
modo,goroutines,epocas,t_total_s,t_epoca_ms,perdida_final,f1,auc,maquina
```

**Cargador** (Pthreads), igual que `internal/medicion`:

```
modo,goroutines,t_procesamiento_s,t_indexado_s,t_mongo_s,t_total_s,filas_leidas,registros,memoria_mb,maquina
```

- `goroutines` significa **unidades de ejecución**: hilos (Pthreads/OpenMP), procesos (`mpiexec -n`) o bloques de CUDA (o el tamaño del problema en el Experimento 8; se documenta en cada caso). Se mantiene el nombre para no romper el análisis.
- `t_mongo_s` vale 0 en el cargador de Pthreads, que no escribe en Mongo y se compara con `cargador/cmd/concurrente -sin-mongo`.
- `t_total_s` mide **solo el cómputo**, sin leer el archivo binario, igual que el entrenamiento en Go. La lectura se puede reportar aparte por la salida estándar.
- `maquina` es texto libre con CPU y GPU (p. ej. `"Ryzen 5 3600"` o `"i7-12700 + RTX 3060"`). El análisis ignora las columnas que no son numéricas, pero sirve para no mezclar mediciones de PCs distintas.
- Valores de `modo`:
  - `pthreads`
  - `openmp_static`, `openmp_dynamic`, `openmp_guided`
  - `mpi_p2p`, `mpi_colectivo`
  - `cuda_atomic`, `cuda_shared`, `cuda_shuffle`
  - además, la línea base de su experimento: `secuencial` o `secuencial_c`

## Interfaz de línea de comandos común

Todos los núcleos de entrenamiento aceptan:

```
--datos data/muestra.bin   --tasa 0.1   --epocas 200   --hilos N   --resultado resultados/experimentoX.csv   --maquina "texto"
```

En MPI el número de procesos lo da `mpiexec -n N` y no `--hilos`. En CUDA, `--hilos` es el tamaño del bloque (p. ej. 256) y se agrega `--kernel atomic|shared|shuffle`.
