# Contrato — formato binario y resultados de los núcleos nativos

Define cómo Go entrega los datos a los programas en C/C++ de `nativo/` (Pthreads, OpenMP, MPI, CUDA) y cómo estos devuelven sus mediciones. Gracias a este contrato, cada núcleo nativo se puede desarrollar y probar **sin MongoDB, sin red y sin Go**: basta con el archivo binario.

Estado: **acordado, pendiente de implementar** (semana 8). Lo genera `herramientas/cmd/exportar-muestra`.

## Reglas generales

- Todos los números son **little-endian**, el orden nativo de x86-64.
- Los reales son `float64` IEEE 754 (`double` en C).
- Las matrices se guardan **por filas** (row-major): el elemento (i, j) de una matriz n×d está en la posición `i*d + j`.
- Un dato faltante se representa con **NaN** (en C, `isnan(x)`).
- Cada archivo empieza con un encabezado de tamaño fijo que se lee con un solo `fread`.

## Archivo 1: `data/muestra.bin` (entrenamiento)

Es la misma muestra que usa `entrenamiento/cmd/secuencial`: etiquetada, partida por mes (entrenamiento = meses 1-9, prueba = 10-12) y **ya normalizada** con las constantes del conjunto de entrenamiento. Un núcleo nativo no tiene que normalizar nada.

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
