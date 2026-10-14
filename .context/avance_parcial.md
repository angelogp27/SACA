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

## Resultados del Experimento 1

Ver [docs/experimento1.md](../docs/experimento1.md) y [resultados/experimento1.md](../resultados/experimento1.md) (tabla generada por `analisis`).

## Pendientes inmediatos (Fase 3, semanas 9-12)

- Semana 9: pasos 3-6 del protocolo (difundir `parametros`, `gradiente_parcial`, agregación síncrona). El trabajador debe cargar su partición con `particion.Pertenece` y llamar a `logistica.GradienteParcial`, que ya está probada.
- Semana 9-10: endpoints de la API (`/predicciones` puede usar `dataset.ModeloGuardado.Probabilidad`), JWT con dos roles, Redis como caché.
- Semana 10: Experimento 3 formal (mutex frente a atómicos) sobre el **vector de parámetros del maestro**, que es la sección crítica que pide el enunciado. `gradienteMutex` y `gradienteAtomico` ya sirven de base.
- Features faltantes del enunciado: rezagos t-1..t-24, medias y máximos móviles de 24 h, y tipo de zona.
- Dockerfiles para maestro y trabajadores, y servicios en `docker-compose.yml` para escalar trabajadores.
- Instalar WSL 2 en la PC de desarrollo (Windows) para usar Docker Desktop.

## Cómo retomar

```powershell
go run ./herramientas/cmd/descargar -verificar     # ¿tengo los datos correctos?
powershell -ExecutionPolicy Bypass -File scripts/pipeline.ps1 -SinDocker   # o sin -SinDocker si Docker funciona
go test -race ./...
cd frontend; npm install; npm run dev
```
