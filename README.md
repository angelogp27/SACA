# SACA — Sistema de Alerta de Calidad del Aire

Proyecto del curso **Programación Concurrente y Paralela (202W0805)**, UNMSM-FISI, semestre 2026-2, grupo 3.

SACA procesa en paralelo los **16,97 millones** de mediciones horarias de 2025 de la red de monitoreo de la EPA (EE. UU.). Con ellas entrena una regresión logística escrita desde cero que predice si el NO2 de una estación cruzará el umbral AQI 51 en la hora siguiente, y muestra los resultados en una SPA.

Está escrito en Go (backend) y React (frontend), con MongoDB como almacenamiento. Las especificaciones están en [docs/](docs/) y el contexto completo del proyecto en [.context/proyecto9.md](.context/proyecto9.md).

## Arquitectura

```
CSV EPA (16.9 M filas) ──► [A] Cargador concurrente ──► MongoDB (10.8 M registros)
                              goroutines + channels          │
                                                              ▼
                           [B] Entrenamiento ◄── muestra etiquetada (caché gob)
                               secuencial / paralelo (goroutines)
                               maestro TCP + trabajadores (semana 9: distribuido)
                                                              │ modelo.json
                                                              ▼
                           [C] API REST (/health; resto semanas 9-11)
                           [D] SPA React ◄── JSON exportados (herramientas/cmd/exportar)
```

## Estructura

| Carpeta | Contenido |
|---|---|
| [cargador/cmd/secuencial](cargador/cmd/secuencial/main.go) | Cargador de una sola goroutine (línea base T(1)) |
| [cargador/cmd/concurrente](cargador/cmd/concurrente/main.go) | Worker pool con fan-out/fan-in y agregadoras particionadas (`-goroutines`, `-agregadoras`) |
| [entrenamiento/cmd/secuencial](entrenamiento/cmd/secuencial/main.go) | Regresión logística secuencial, con evaluación en meses no vistos |
| [entrenamiento/cmd/paralelo](entrenamiento/cmd/paralelo/main.go) | Gradiente con N goroutines (`-estrategia reduccion\|mutex\|atomico`) |
| [entrenamiento/cmd/maestro](entrenamiento/cmd/maestro/main.go), [trabajador](entrenamiento/cmd/trabajador/main.go) | Clúster TCP: registro de nodos, particiones y bajas |
| [api/cmd/api](api/cmd/api/main.go) | API REST (por ahora solo `/health`) |
| [herramientas/cmd/descargar](herramientas/cmd/descargar/main.go) | Descarga reproducible de los datos con manifiesto SHA-256 |
| [herramientas/cmd/exportar](herramientas/cmd/exportar/main.go) | Genera los JSON reales para el frontend |
| [experimentos/cmd/analisis](experimentos/cmd/analisis/main.go) | Speedup, eficiencia, Karp-Flatt y Amdahl a partir de las corridas |
| `internal/` | Paquetes compartidos: `aqs` (limpieza), `persistencia`, `particion`, `dataset`, `logistica`, `metricas`, `entrenador`, `protocolo`, `cluster`, `medicion`, `registro` |
| [frontend/](frontend/) | SPA React + Vite |
| [scripts/](scripts/) | Pipeline y experimentos, en PowerShell (`.ps1`) y sh (`.sh`) |
| [resultados/](resultados/) | CSV y tablas de los experimentos |
| [docs/](docs/) | Contratos, experimentos y relación con el temario del curso |

## Requisitos

- Go 1.25 o superior (el `go.mod` declara 1.27.1)
- Docker con Docker Compose para MongoDB. En Windows, Docker Desktop necesita WSL 2.
  - Alternativa sin Docker: cualquier MongoDB 7 escuchando en `localhost:27017`, usando el flag `-SinDocker` del pipeline.
- Node.js 20 o superior para el frontend
- Unos 5 GB libres para los CSV y unos 8 GB de RAM para la carga completa

## Puesta en marcha (un comando)

```powershell
# Windows
powershell -ExecutionPolicy Bypass -File scripts/pipeline.ps1                # sin experimentos
powershell -ExecutionPolicy Bypass -File scripts/pipeline.ps1 -Experimentos  # + experimentos (~1 h)
```

```sh
# Linux / macOS
sh scripts/pipeline.sh
EXPERIMENTOS=1 sh scripts/pipeline.sh
```

El pipeline hace, en orden:
1. descarga y verifica los datos;
2. levanta MongoDB;
3. carga los 16,9 M de filas;
4. entrena el modelo y lo guarda en `data/modelo.json`;
5. corre los experimentos (solo si se pide);
6. exporta los JSON del frontend.

Luego:

```sh
cd frontend && npm install && npm run dev
```

## Comandos por separado

```sh
# Datos: descarga idempotente y verificación contra data/manifiesto.json
go run ./herramientas/cmd/descargar
go run ./herramientas/cmd/descargar -verificar

# Parte A: cargadores
go run ./cargador/cmd/secuencial
go run ./cargador/cmd/concurrente -goroutines 8                  # 1 agregadora (diseño original)
go run ./cargador/cmd/concurrente -goroutines 8 -agregadoras 0   # tantas agregadoras como trabajadoras
#   -sin-mongo mide solo el procesamiento; -resultado archivo.csv guarda los tiempos por fase

# Parte B: entrenamiento
go run ./entrenamiento/cmd/secuencial -modelo data/modelo.json
go run ./entrenamiento/cmd/paralelo -goroutines 8 -estrategia reduccion

# Parte B: clúster TCP (en terminales distintas)
go run ./entrenamiento/cmd/maestro -nodos 2
go run ./entrenamiento/cmd/trabajador -id w1
go run ./entrenamiento/cmd/trabajador -id w2

# Experimentos
powershell -ExecutionPolicy Bypass -File scripts/experimento1.ps1    # o: sh scripts/experimento1.sh
powershell -ExecutionPolicy Bypass -File scripts/experimento1b.ps1   # o: sh scripts/experimento1b.sh

# Pruebas (incluye detector de carreras)
go test -race ./...
```

`go test -race` necesita CGO, y en Windows un compilador de C (por ejemplo MSYS2/MinGW).

## Resultados principales

Medidos en un AMD Ryzen 5 3600 (6 núcleos y 12 hilos), con 16 GB de RAM y SSD. Detalle y análisis en [docs/experimento1.md](docs/experimento1.md).

| Qué | Resultado |
|---|---|
| Datos | 16 972 287 filas crudas → 10 838 667 registros (estación, hora); 1625 filas excluidas por qualifier de evento excepcional |
| Memoria del cargador | 8,7 GB → 4,4 GB al dejar de retener las líneas del CSV |
| Cargador, 8 goroutines | Procesamiento de 27,0 s con 1 agregadora frente a 11,5 s con 8 agregadoras particionadas |
| Entrenamiento, Experimento 1b | Con reducción, S(8) = 6,11 (E = 76 %); con mutex y CAS por ejemplo, más lento que el secuencial |
| Modelo (NO2 > 53 ppb a 1 h) | AUC 0,995 y exhaustividad 0,97 evaluando en oct-dic. La precisión es baja (0,011) porque solo el 0,02 % de las horas cruza el umbral |

Las tablas completas se generan en [resultados/experimento1.md](resultados/experimento1.md) y [resultados/experimento1b.md](resultados/experimento1b.md).

## Documentación

- [docs/contrato_registro_procesado.md](docs/contrato_registro_procesado.md): esquema del documento en MongoDB y reglas de limpieza
- [docs/contrato_protocolo_tcp.md](docs/contrato_protocolo_tcp.md): protocolo maestro-trabajador
- [docs/contrato_api_rest.md](docs/contrato_api_rest.md): API REST y WebSocket
- [docs/experimento1.md](docs/experimento1.md): diseño, resultados y análisis de los experimentos 1 y 1b
- [docs/conceptos_curso.md](docs/conceptos_curso.md): qué concepto del sílabo se aplica en cada parte del código
- [.context/avance_parcial.md](.context/avance_parcial.md): bitácora del avance del parcial (decisiones, hallazgos, pendientes)
