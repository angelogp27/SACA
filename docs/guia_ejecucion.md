# Guía de ejecución y tutoriales

Para cualquier integrante del grupo que quiera ejecutar SACA en su PC, desde cero. Empieza por la sección que necesites:

| Quiero… | Ir a | Tiempo |
|---|---|---|
| Ver el frontend con datos reales, sin instalar nada más | [Tutorial 1](#tutorial-1-ver-el-frontend-en-2-minutos) | 2 min |
| Comprobar que todo el proyecto funciona en mi PC | [Prueba end-to-end](#verificar-que-todo-funcione-prueba-end-to-end) | 5 min + descarga |
| Tener todo el sistema funcionando con los datos completos | [Tutorial 2](#tutorial-2-sistema-completo-de-punta-a-punta) | 15 min + descarga |
| Entender la concurrencia midiendo yo mismo | [Tutorial 3](#tutorial-3-comparar-diseños-y-estrategias-de-sincronización) | 10 min |
| Probar el clúster maestro-trabajador | [Tutorial 4](#tutorial-4-clúster-tcp-en-una-pc-o-en-varias) | 5 min |
| Volver a medir los experimentos | [Tutorial 5](#tutorial-5-repetir-los-experimentos) | 5 min a 1 h |

Si algo falla, ve a [Problemas frecuentes](#problemas-frecuentes). Para entender el diseño, [arquitectura.md](arquitectura.md); para las opciones de cada programa, [funcionalidades.md](funcionalidades.md).

---

## 1. Instalación

### Requisitos

| Herramienta | Versión | Para qué | Cómo comprobar |
|---|---|---|---|
| Git | cualquiera | Clonar el repo | `git --version` |
| Go | 1.25 o superior | Todo el backend | `go version` |
| Node.js | 20 o superior | Frontend | `node --version` |
| MongoDB 7 | — | Base de datos | ver abajo |
| gcc (opcional) | — | `go test -race` y los núcleos en C (Pthreads, OpenMP) | `gcc --version` |

Recursos: unos **5 GB libres** para los datos y **8 GB de RAM** o más para la carga completa (el cargador usa ~6 GB).

### Windows

1. Instalar **Git** (https://git-scm.com), **Go** (https://go.dev/dl), **Node.js LTS** (https://nodejs.org).
2. Opcional, para `go test -race` y los núcleos en C: instalar **MSYS2** (https://www.msys2.org) y en su terminal UCRT64 ejecutar `pacman -S mingw-w64-ucrt-x86_64-gcc`. Luego agregar `C:\msys64\ucrt64\bin` al PATH.
3. **MongoDB**, una de estas opciones:
   - **Sin Docker (recomendado si no tienes WSL 2):**

     ```powershell
     powershell -ExecutionPolicy Bypass -File scripts/mongo-local.ps1
     ```

     Descarga una vez MongoDB portable (~600 MB) a `%LOCALAPPDATA%\saca` y lo deja corriendo en `localhost:27017`. No hace falta instalar nada ni ser administrador. `-Estado` muestra si corre y `-Detener` lo apaga. Hay que volver a ejecutarlo después de reiniciar la PC.
   - **Con Docker Desktop** (necesita WSL 2: `wsl --install` como administrador y reiniciar): `docker compose up -d mongo`.

### Linux / macOS

```sh
# Go, Node y Docker con el gestor de tu sistema; luego:
docker compose up -d mongo
# o MongoDB nativo: https://www.mongodb.com/docs/manual/administration/install-on-linux/
```

### Clonar

```powershell
git clone https://github.com/angelogp27/SACA.git
cd SACA
```

**Todos los comandos de esta guía se ejecutan desde la carpeta raíz `SACA`.**

---

## Tutorial 1: ver el frontend en 2 minutos

Los datos reales ya exportados vienen en el repo (`frontend/public/datos/`), así que no hace falta Mongo ni los CSV.

```powershell
cd frontend
npm install
npm run dev
```

Abre la URL que aparece (normalmente http://localhost:5173):

- **Mapa:** 1447 estaciones reales. El color indica el % de horas de 2025 sobre el umbral AQI 51 del gas dominante. Al pasar el cursor se ve el detalle; con un clic se cambia la serie.
- **Serie:** NO2 observado en el día con más alertas de la red y, debajo, la probabilidad que da el modelo de cruzar 53 ppb en la hora siguiente.
- **Rendimiento:** pestañas Cargador / Entrenamiento con speedup, eficiencia, ideal y Amdahl de los experimentos. "Ver tabla" muestra los valores.

---

## Verificar que todo funcione (prueba end-to-end)

Comprueba **todo el proyecto** en unos minutos:

- compilación, `go vet` y las pruebas con detector de carreras;
- datos, cargadores, entrenamiento, clúster, exportación, análisis y frontend.

Usa un subconjunto de los datos reales y una base temporal (`saca_e2e`) que borra al terminar. **No toca tus datos ni tus resultados, ni repite los experimentos largos.**

Requisitos: los datos descargados (paso 1 del Tutorial 2) y MongoDB corriendo.

```powershell
go run ./herramientas/cmd/e2e
```

Resultado esperado:

```
[ 1/15] Herramientas instaladas                                    OK   0.8s  go1.27.1 windows/amd64, node v24.15.0
[ 2/15] Compilar todos los programas                               OK  30.9s  10 ejecutables
[ 3/15] go vet                                                     OK    1s
[ 4/15] go test (pruebas unitarias)                                OK   9.4s  6 paquetes con pruebas, con -race
[ 5/15] Datos crudos contra el manifiesto                          OK   3.5s  SHA-256 de los 5 CSV coincide
[ 6/15] Subconjunto de datos reales                                OK   0.3s  600000 filas de los 4 CSV
[ 7/15] Conexión a MongoDB                                         OK     0s  mongodb://localhost:27017, base temporal saca_e2e
[ 8/15] Cargador secuencial → MongoDB                              OK   5.3s  600000 filas → 413653 registros en MongoDB
[ 9/15] Cargador concurrente = secuencial (registros y valores)    OK   6.9s  concurrente y concurrente_particionado: mismos registros y mismos valores
[10/15] Entrenamiento secuencial + modelo                          OK   4.2s  pérdida 0.166297, modelo de 12 pesos guardado
[11/15] Entrenamiento paralelo (3 estrategias) = secuencial        OK   1.1s  reducción, mutex y atómico dan la misma pérdida (±1e-6)
[12/15] Clúster TCP: entrenamiento distribuido = secuencial        OK   1.6s  2 nodos: pérdida 0.166297 = secuencial; sobrante y muestra distinta rechazados; caída detectada
[13/15] Exportar datos para el frontend                            OK     3s  56 estaciones, 38 series del 2025-11-15
[14/15] Análisis de experimentos (resultados existentes)           OK   0.3s  Experimento 1b releído: mejor S(8) = 6.11
[15/15] Frontend: lint, build y servidor                           OK     7s  build OK, 6 recursos servidos

RESULTADO: TODO OK
```

Opciones útiles:
- `-sin-race` si no tienes gcc;
- `-sin-frontend` si no tienes Node;
- `-verificar-datos=false` para saltar la verificación de los 4 GB;
- `-mantener` para inspeccionar la carpeta y la base temporales.

**Ejecútala antes de cada commit importante y antes de la demo.**

---

## Tutorial 2: sistema completo de punta a punta

### Opción rápida: un solo comando

```powershell
# Windows sin Docker (levanta MongoDB portable solo)
powershell -ExecutionPolicy Bypass -File scripts/pipeline.ps1 -MongoLocal
# Windows con Docker
powershell -ExecutionPolicy Bypass -File scripts/pipeline.ps1
```

```sh
# Linux / macOS
sh scripts/pipeline.sh
```

El pipeline:
1. descarga y verifica los datos;
2. levanta MongoDB;
3. carga los 16,9 M de filas;
4. entrena y guarda el modelo;
5. exporta los JSON del frontend.

Sin contar la descarga, tarda **~3 minutos**. La primera vez suma ~10 minutos de descarga.

### Paso a paso, para entender qué ocurre

```powershell
# 1. Datos: 4 CSV horarios de la EPA 2025 (~4 GB, ~10 min la primera vez)
go run ./herramientas/cmd/descargar
go run ./herramientas/cmd/descargar -verificar    # comprueba el SHA-256 contra data/manifiesto.json

# 2. MongoDB
powershell -ExecutionPolicy Bypass -File scripts/mongo-local.ps1     # o: docker compose up -d mongo

# 3. Parte A: cargar en Mongo (~40 s, ~6 GB de RAM)
go run ./cargador/cmd/concurrente -goroutines 8 -agregadoras 0
#    → registros unicos (estacion, hora): 10838667 (335331 valores de monitores repetidos resueltos por POC)

# 4. Parte B: entrenar y guardar el modelo (~1 min la primera vez; luego ~3 s con la caché)
go run ./entrenamiento/cmd/secuencial -modelo data/modelo.json
#    → evaluacion en prueba: ... AUC=0.995

# 5. Datos para el frontend (~1,5 min)
go run ./herramientas/cmd/exportar

# 6. Frontend
cd frontend; npm install; npm run dev
```

Qué pasa en cada paso: [arquitectura.md §1 y §8](arquitectura.md).

---

## Tutorial 3: comparar diseños y estrategias de sincronización

Es la forma más directa de *ver* los conceptos del curso. Necesita Mongo cargado (Tutorial 2).

### a) La agregadora única como cuello de botella (ley de Amdahl)

```powershell
go run ./cargador/cmd/concurrente -goroutines 8 -agregadoras 1 -sin-mongo   # diseño original
go run ./cargador/cmd/concurrente -goroutines 8 -agregadoras 0 -sin-mongo   # particionado
```

Compara `procesamiento` e `indexado` en la última línea. Con una agregadora, el indexado (serial) se come la mayor parte del tiempo, y agregar trabajadoras no ayuda. Particionado, el indexado se reparte y el total baja a menos de la mitad. Explicación: [experimento1.md](experimento1.md).

### b) Reducción frente a mutex frente a atómicos

```powershell
go run ./entrenamiento/cmd/secuencial
go run ./entrenamiento/cmd/paralelo -goroutines 8 -estrategia reduccion
go run ./entrenamiento/cmd/paralelo -goroutines 8 -estrategia mutex
go run ./entrenamiento/cmd/paralelo -goroutines 8 -estrategia atomico
```

| Qué mirar | Qué se ve |
|---|---|
| `perdida final` | Es la misma en los cuatro: las cuatro implementaciones son correctas |
| ms por época | La reducción va ~6× más rápido que el secuencial; mutex y atómico van **más lento** que el secuencial |

La razón es que comparten un acumulador por cada ejemplo, y la contención cuesta más que el cálculo. Código: [internal/logistica/paralelo.go](../internal/logistica/paralelo.go).

### c) Detector de carreras

```powershell
go test -race ./...
```

Si alguien introduce un acceso concurrente sin sincronizar, esto falla con un reporte `DATA RACE`.

---

## Tutorial 4: clúster TCP en una PC o en varias

Requisito: `data/muestra.gob`, que se crea con el pipeline o el entrenamiento secuencial, o se copia de otra PC.

**En una PC**, todo con un comando:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/cluster-local.ps1 -Nodos 3
```

O a mano, con 3 terminales:

```powershell
go run ./entrenamiento/cmd/maestro -nodos 2      # terminal 1
go run ./entrenamiento/cmd/trabajador -id w1     # terminal 2
go run ./entrenamiento/cmd/trabajador -id w2     # terminal 3
```

Cuando están los 2 nodos, el maestro entrena: una línea cada 20 épocas con el tiempo de cálculo y de red. Al final muestra una tabla por nodo y la verificación:

```
verificación: pérdida final distribuida 0.125720142, secuencial 0.125720142, diferencia 2.6e-15 → OK, mismo modelo
```

Para ver más comportamiento:

- un tercer trabajador es **rechazado** porque solo hay 2 particiones, y uno con otra muestra es rechazado por `muestra_distinta`;
- al cerrar un trabajador con Ctrl+C a mitad del entrenamiento, el maestro **aborta** con `el nodo w1 se cayó`.

**En varias PCs**, incluso en casas distintas por internet: seguir la guía de cada rol en [cluster/README.md](cluster/README.md), que cubre Tailscale, el firewall, los nodos con GPU NVIDIA (motor CUDA) y los problemas frecuentes. Arquitectura y MPI: [guia_cluster.md](guia_cluster.md).

---

## Tutorial 5: repetir los experimentos

> Los resultados oficiales ya están en `resultados/` y en [experimento1.md](experimento1.md). Repetirlos **sobrescribe** esos archivos. Hazlo solo si quieres medir en tu PC, y en ese caso guarda los resultados con otro nombre o en una rama.

| Experimento | Comando | Duración |
|---|---|---|
| 1b (entrenamiento) | `powershell -ExecutionPolicy Bypass -File scripts/experimento1b.ps1` | ~5 min |
| 1 (cargador con Mongo) | `powershell -ExecutionPolicy Bypass -File scripts/experimento1.ps1` | ~1 hora |
| 1 rápido (sin Mongo) | `powershell -ExecutionPolicy Bypass -File scripts/experimento1.ps1 -SinMongo -Repeticiones 1` | ~8 min |

Para **volver a analizar** sin volver a medir:

```powershell
go run ./experimentos/cmd/analisis -entrada resultados/experimento1.csv -paralelo "concurrente,concurrente_particionado"
```

Durante las mediciones, cierra los programas pesados y pausa la sincronización de OneDrive. Si no, los tiempos salen ruidosos.

---

## Problemas frecuentes

| Síntoma | Causa | Solución |
|---|---|---|
| `server selection error` / `connection refused` en el puerto 27017 | MongoDB no está corriendo | `scripts/mongo-local.ps1` o `docker compose up -d mongo` |
| Docker Desktop no arranca, error 500 | Falta WSL 2 | `wsl --install` como administrador y reiniciar, o usar `scripts/mongo-local.ps1` |
| `falta data\raw\hourly_...csv` | No se descargaron los datos | `go run ./herramientas/cmd/descargar` |
| `SHA-256 distinto` al verificar | La EPA actualizó el archivo o la descarga se cortó | Borra ese CSV y vuelve a ejecutar `descargar` |
| `-race requires cgo` / `gcc not found` | No hay compilador de C | Instala gcc (MSYS2) o usa `go test ./...` / `e2e -sin-race` |
| La carga se cuelga y el log de Mongo muestra `WTEVICT` | Mongo y el cargador se quedan sin RAM | Usar caché de 2 GB: ya viene así en `mongo-local.ps1` y `docker-compose.yml` |
| El entrenamiento usa datos viejos | Está leyendo la caché `data/muestra.gob` | Borra `data/muestra.gob` para sortear la muestra de nuevo |
| `no se encontraron ejemplos etiquetables` | La colección está vacía | Carga los datos (Tutorial 2, paso 3) |
| El frontend muestra "datos de ejemplo" | Falta algún JSON en `frontend/public/datos` | `go run ./herramientas/cmd/exportar` |
| `running scripts is disabled` en PowerShell | Política de ejecución | Usar `powershell -ExecutionPolicy Bypass -File ...` como en esta guía |
| El trabajador repite `maestro ... no disponible todavía` | El maestro no corre, o el firewall bloquea el puerto 9000 | Arrancar el maestro; abrir el puerto en la PC del maestro |
| Los tiempos varían mucho entre corridas | Otros programas, antivirus u OneDrive compiten por la CPU | Cerrarlos; usar la mediana de 3 repeticiones (los scripts ya lo hacen) |

## Antes de subir cambios (checklist)

1. `go run ./herramientas/cmd/e2e` termina en `RESULTADO: TODO OK`.
2. Un commit por cambio pequeño y coherente, con un mensaje en español del estilo `area: qué cambia`.
3. Si cambiaste una regla de datos o un mensaje del protocolo, actualiza también el contrato en `docs/contrato_*.md`.
