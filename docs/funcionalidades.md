# Funcionalidades: qué hace cada programa y cómo se usa

Catálogo de todos los ejecutables del proyecto, con sus opciones, entradas, salidas y un ejemplo. Todos se ejecutan **desde la raíz del repositorio** con `go run ./ruta/del/programa [opciones]`. Con `-h`, cada uno muestra sus opciones.

Para el orden en que se usan, ver [guia_ejecucion.md](guia_ejecucion.md). Para cómo encajan entre sí, [arquitectura.md](arquitectura.md).

| Programa | Para qué | Necesita Mongo |
|---|---|---|
| [`herramientas/cmd/descargar`](#descargar) | Bajar y verificar los datos de la EPA | No |
| [`cargador/cmd/secuencial`](#cargadores) | Cargar los CSV en Mongo con 1 goroutine (línea base) | Sí (salvo `-sin-mongo`) |
| [`cargador/cmd/concurrente`](#cargadores) | Lo mismo con N goroutines y K agregadoras | Sí (salvo `-sin-mongo`) |
| [`entrenamiento/cmd/secuencial`](#entrenamiento) | Entrenar el modelo y evaluarlo | Solo la primera vez (luego usa la caché) |
| [`entrenamiento/cmd/paralelo`](#entrenamiento) | Entrenar con N goroutines y 3 estrategias de sincronización | Ídem |
| [`entrenamiento/cmd/maestro`](#cluster-tcp) | Maestro del clúster TCP: entrenamiento distribuido síncrono | No (usa `data/muestra.gob`) |
| [`entrenamiento/cmd/trabajador`](#cluster-tcp) | Nodo trabajador del clúster (motor go o cuda) | No (usa `data/muestra.gob`) |
| [`herramientas/cmd/exportar`](#exportar) | Generar los JSON reales del frontend | Sí |
| [`experimentos/cmd/analisis`](#analisis) | Speedup, eficiencia y Amdahl a partir de un CSV de corridas | No |
| [`herramientas/cmd/e2e`](#e2e) | Prueba end-to-end de todo el sistema | Sí |
| [`api/cmd/api`](#api) | API REST (por ahora `/health`) | No |

Scripts en `scripts/`:

| Script | Para qué |
|---|---|
| `pipeline.ps1` / `pipeline.sh` | Todo de punta a punta: datos → Mongo → carga → modelo → (experimentos) → frontend |
| `mongo-local.ps1` | MongoDB portable en Windows sin Docker (`-Estado`, `-Detener`) |
| `experimento1.ps1` / `.sh` | Experimento 1 completo (cargador, ~1 hora) |
| `experimento1b.ps1` / `.sh` | Experimento 1b (entrenamiento, ~5 min) |

---

## descargar

`go run ./herramientas/cmd/descargar` baja de la EPA los 4 CSV horarios de 2025 y el resumen anual por monitor. Los descarga en paralelo (3 a la vez), los descomprime en `data/raw/` y escribe `data/manifiesto.json` con el tamaño, el SHA-256 y el número de filas de cada uno.

| Opción | Por defecto | Qué hace |
|---|---|---|
| `-verificar` | `false` | No descarga: comprueba que los CSV locales coincidan con el manifiesto |
| `-anio` | `2025` | Año de los archivos |
| `-destino` | `data/raw` | Carpeta de los CSV |
| `-paralelas` | `3` | Descargas simultáneas |

Es **idempotente**: si un CSV ya está y coincide con el manifiesto, lo salta. Unos 4 GB y ~10 min la primera vez.

```
hourly_44201_2025.csv     8867266 filas  2033.2 MB
total de filas horarias (4 gases): 16972287
```

## Cargadores

`go run ./cargador/cmd/secuencial` y `go run ./cargador/cmd/concurrente` leen los 4 CSV, aplican las reglas de limpieza ([contrato_registro_procesado.md](contrato_registro_procesado.md)), unen los gases por (estación, hora) y **reemplazan** la colección `registros_procesados` de Mongo.

| Opción | Por defecto | Qué hace |
|---|---|---|
| `-datos` | `data/raw` | Carpeta de los CSV |
| `-db` | `proyecto9` | Base de datos de Mongo |
| `-mongo-uri` | `mongodb://localhost:27017` | Conexión |
| `-sin-mongo` | `false` | Solo procesa, sin escribir (mide la parte de cómputo) |
| `-resultado` | — | CSV al que se agrega una fila con los tiempos por fase |
| `-goroutines` (concurrente) | `4` | Goroutines trabajadoras |
| `-agregadoras` (concurrente) | `1` | Goroutines agregadoras. **`0` = tantas como trabajadoras (recomendado)**; `1` = diseño original |
| `-bloques-por-goroutine` (concurrente) | `4` | Bloques por archivo y goroutine (balanceo de carga) |

Ejemplo recomendado (~40 s con Mongo, ~6 GB de RAM):

```powershell
go run ./cargador/cmd/concurrente -goroutines 8 -agregadoras 0
```

Salida:

```
total: leidas=16972287 descartadas=1625 convertidas=0 (sin_medicion=0 qualifier_excluido=1625 ...)
registros unicos (estacion, hora): 10838667
T(8) concurrente_particionado: total=41s procesamiento=13.5s indexado=3.2s mongo=27.8s memoria=5766 MB
```

## Entrenamiento

`go run ./entrenamiento/cmd/secuencial` y `go run ./entrenamiento/cmd/paralelo` entrenan una regresión logística que predice si el NO2 de una estación superará 53 ppb (AQI 51) **una hora después**:

1. Sortean una muestra de Mongo.
2. Entrenan con enero-septiembre.
3. Evalúan con octubre-diciembre.

| Opción | Por defecto | Qué hace |
|---|---|---|
| `-muestra` | `200000` | Registros con NO2 que se sortean |
| `-epocas` | `200` | Épocas de descenso de gradiente |
| `-tasa` | `0.1` | Tasa de aprendizaje |
| `-mes-corte` | `10` | Desde qué mes se evalúa |
| `-cache` | `data/muestra.gob` | Caché de la muestra. **Bórrala si recargaste Mongo con otros datos** |
| `-modelo` | — | Guarda el modelo en JSON (p. ej. `data/modelo.json`) |
| `-resultado` | — | CSV con tiempo, pérdida, F1 y AUC |
| `-db`, `-mongo-uri` | | Igual que en los cargadores |
| `-goroutines` (paralelo) | `4` | Goroutines por época |
| `-estrategia` (paralelo) | `reduccion` | `reduccion` (casillas privadas), `mutex` o `atomico` (CAS) |

```powershell
go run ./entrenamiento/cmd/secuencial -modelo data/modelo.json
go run ./entrenamiento/cmd/paralelo -goroutines 8 -estrategia reduccion   # ~6× más rápido
go run ./entrenamiento/cmd/paralelo -goroutines 8 -estrategia mutex       # más lento que el secuencial
```

Salida:

```
entrenamiento (meses 1-9): 146715 ejemplos, 0.02% positivos | prueba (meses 10-12): 49264 ejemplos, 0.08% positivos
evaluacion en prueba: VP=37 FP=3268 VN=45958 FN=1 | exactitud=0.934 precision=0.011 exhaustividad=0.974 F1=0.022 AUC=0.995
T(1) entrenamiento secuencial (200 epocas): 2.33s (11.660 ms por epoca)
```

Las tres estrategias dan **la misma pérdida final**. Si no coincide, hay un error.

## Clúster TCP

`entrenamiento/cmd/maestro` y `entrenamiento/cmd/trabajador` forman el clúster de **entrenamiento distribuido síncrono**:

- El maestro escucha por TCP, registra a los trabajadores y le asigna a cada uno una partición distinta de las estaciones.
- Rechaza a los nodos que sobran y a los que tienen otra muestra (huella SHA-256).
- Cuando están todos, en cada época difunde los pesos, recibe el gradiente parcial de cada nodo, los suma y actualiza.
- Al final muestra una tabla por nodo y el desglose cálculo/red, y verifica que la pérdida sea igual a la del entrenamiento secuencial.
- Si un nodo se cae, aborta con un mensaje claro.

| Programa | Opción | Por defecto |
|---|---|---|
| maestro | `-escuchar` | `:9000` |
| maestro | `-nodos` | `2` (particiones = nodos esperados; el entrenamiento empieza cuando están todos) |
| maestro | `-muestra` | `data/muestra.gob` (los nodos deben tener una copia idéntica) |
| maestro | `-epocas`, `-tasa`, `-mes-corte` | `200`, `0.1`, `10` |
| maestro | `-espera` | `2m` (cuánto se espera el gradiente de un nodo antes de abortar) |
| maestro | `-verificar` | `true` (entrena también en secuencial: misma pérdida y speedup) |
| maestro | `-resultado`, `-modo`, `-maquina` | CSV del Experimento 2; `tcp_sincrono`; texto libre |
| trabajador | `-maestro` | `localhost:9000` (por Tailscale: `100.x.y.z:9000`) |
| trabajador | `-id` | nombre de la PC + PID |
| trabajador | `-muestra` | `data/muestra.gob` |
| trabajador | `-motor` | `auto`: CUDA si existe `bin/gradiente_cuda.exe` y valida contra Go; si no, goroutines. También `go` o `cuda` |
| trabajador | `-hilos` | `0` = todos los núcleos lógicos (motor go) |

El trabajador no necesita MongoDB y reintenta la conexión si el maestro todavía no arrancó. Con Ctrl+C se despide con un mensaje `baja`. Mensajes: [contrato_protocolo_tcp.md](contrato_protocolo_tcp.md). Manual para unir las PCs del grupo por internet: [manual_cluster.md](manual_cluster.md). Motor CUDA: [nativo/cuda/README.md](../nativo/cuda/README.md).

Scripts:

- `scripts/cluster-local.ps1` (o `.sh`): el maestro y N trabajadores en una PC; con `-Ventanas`, cada trabajador en su ventana.
- `scripts/nodo.ps1 -Maestro IP:9000 -Id nombre` (o `nodo.sh`): une una PC al clúster.
- `scripts/experimento2.ps1`: el Experimento 2 con 1, 2, 4 y 8 nodos locales.

## exportar

`go run ./herramientas/cmd/exportar` genera los datos reales del frontend en `frontend/public/datos/`:

| Archivo | Contenido |
|---|---|
| `estaciones.json` | Las ~1447 estaciones con ubicación, nombre local (del resumen anual), y horas, p98, máximo y **% de horas sobre el umbral AQI 51** de cada gas |
| `series.json` | Las 24 horas del día con más horas de NO2 sobre el umbral en toda la red, con la **probabilidad del modelo** por hora |
| `resumen.json` | Totales, umbrales y métricas del modelo |

| Opción | Por defecto |
|---|---|
| `-modelo` | `data/modelo.json` |
| `-fecha` | vacío = día con más excedencias |
| `-salida` | `frontend/public/datos` |
| `-db`, `-mongo-uri`, `-anual`, `-manifiesto` | |

## analisis

`go run ./experimentos/cmd/analisis` lee un CSV de corridas (de los cargadores o del entrenamiento), toma la **mediana** de las repeticiones y calcula S(p), E(p), Karp-Flatt y la curva de Amdahl. Escribe un JSON para el panel de rendimiento y una tabla Markdown.

| Opción | Ejemplo |
|---|---|
| `-entrada` | `resultados/experimento1.csv` |
| `-json` / `-md` | Salidas |
| `-base` | Modo que da T(1) (`secuencial`) |
| `-paralelo` | Modos a comparar, separados por coma; el primero es la serie principal |
| `-serial` | Columnas que suman la parte serial (`t_indexado_s,t_mongo_s`); `-serial=` vacío = Karp-Flatt |

Sirve para volver a analizar resultados **sin volver a correr** los experimentos.

## e2e

`go run ./herramientas/cmd/e2e` es la prueba end-to-end. Verifica en unos minutos que **todo** funcione, usando un subconjunto de los datos reales y una base temporal `saca_e2e` que borra al final. Ver [guia_ejecucion.md](guia_ejecucion.md#verificar-que-todo-funcione-prueba-end-to-end).

| Opción | Por defecto | Qué hace |
|---|---|---|
| `-filas` | `150000` | Filas por CSV en el subconjunto |
| `-verificar-datos` | `true` | Comprobar los CSV contra el manifiesto (~20 s) |
| `-sin-frontend` | `false` | Saltar lint, build y servidor del frontend |
| `-sin-race` | `false` | `go test` sin detector de carreras (si no hay compilador de C) |
| `-mantener` | `false` | No borrar la carpeta temporal ni la base |

## api

`go run ./api/cmd/api` levanta la API en `-puerto` (por defecto 8080). Por ahora solo tiene `GET /health` → `{"status":"ok"}`. El resto de rutas está definido en [contrato_api_rest.md](contrato_api_rest.md) (semanas 9-11).

## Frontend

En `frontend/`:

```powershell
npm install
npm run dev
```

| Panel | Muestra |
|---|---|
| Mapa de estaciones | Las estaciones reales coloreadas por **riesgo histórico** (bajo < 2 %, moderado 2-10 %, alto ≥ 10 % de horas sobre el umbral). Al pasar el cursor se ve el detalle; con un clic se selecciona |
| Serie horaria | NO2 observado con la línea del umbral y, debajo, la probabilidad que da el modelo de cruzarlo en la hora siguiente |
| Rendimiento | Pestañas Cargador / Entrenamiento: speedup, eficiencia, ideal y Amdahl, con tabla de valores |
| Clúster | Maqueta hasta la semana 11 |

Si falta algún JSON, el panel usa datos de ejemplo y lo dice en su subtítulo.
