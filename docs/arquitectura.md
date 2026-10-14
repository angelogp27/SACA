# Arquitectura y flujos de SACA

Este documento explica **cómo está construido el sistema y por dónde pasan los datos**, con diagramas. Los diagramas usan [Mermaid](https://mermaid.js.org/), que GitHub y VS Code (con la extensión "Markdown Preview Mermaid Support") muestran como imágenes.

Para **ejecutar** el proyecto ver [guia_ejecucion.md](guia_ejecucion.md). Para el detalle de cada programa y sus opciones, [funcionalidades.md](funcionalidades.md).

## 1. Vista general

```mermaid
flowchart LR
    EPA[("Portal EPA<br/>4 ZIP horarios<br/>+ resumen anual")]
    subgraph Datos
        DESC["herramientas/cmd/descargar<br/>(goroutines + SHA-256)"]
        RAW[("data/raw/*.csv<br/>16,97 M filas")]
        MAN["data/manifiesto.json"]
    end
    subgraph A["Parte A: cargador"]
        CS["cargador/cmd/secuencial"]
        CC["cargador/cmd/concurrente<br/>worker pool + agregadoras"]
    end
    MONGO[("MongoDB<br/>registros_procesados<br/>10,8 M docs")]
    subgraph B["Parte B: entrenamiento"]
        ES["entrenamiento/cmd/secuencial"]
        EP["entrenamiento/cmd/paralelo<br/>reducción · mutex · CAS"]
        MAE["maestro TCP"]
        TRA["trabajadores TCP"]
        MOD["data/modelo.json"]
    end
    subgraph Exp["Experimentos"]
        CSV[("resultados/*.csv")]
        AN["experimentos/cmd/analisis<br/>S(p), E(p), Amdahl"]
    end
    EXPO["herramientas/cmd/exportar"]
    subgraph D["Parte D: frontend"]
        JSON[("frontend/public/datos/*.json")]
        SPA["SPA React<br/>mapa · serie · rendimiento"]
    end
    API["Parte C: API REST<br/>(/health; resto semanas 9-11)"]
    NAT["nativo/ (en desarrollo)<br/>Pthreads · OpenMP · MPI · CUDA"]

    EPA --> DESC --> RAW
    DESC --> MAN
    RAW --> CS --> MONGO
    RAW --> CC --> MONGO
    MONGO --> ES --> MOD
    MONGO --> EP
    MAE <-->|JSON por línea| TRA
    CS & CC & ES & EP -->|-resultado| CSV --> AN --> JSON
    MONGO --> EXPO
    MOD --> EXPO --> JSON --> SPA
    MOD -.-> API
    MONGO -.->|muestra.bin| NAT -.-> CSV
```

| Parte | Responsabilidad | Modelo de paralelismo | Estado |
|---|---|---|---|
| A: cargador | Limpiar y unir los 4 CSV por (estación, hora) y guardarlos en MongoDB | Memoria compartida: goroutines + channels | ✅ |
| B: entrenamiento | Regresión logística que predice si el NO2 cruzará 53 ppb en 1 hora | Memoria compartida (goroutines) y paso de mensajes (TCP) | ✅ secuencial y paralelo, maestro y registro de nodos; ⏳ entrenamiento distribuido (semana 9) |
| C: API | Predicciones y control de entrenamientos para el frontend | Una goroutine por petición (la pone Go) | ⏳ solo `/health` |
| D: frontend | Mapa, serie con la predicción y panel de rendimiento | — | ✅ con datos reales exportados |
| Núcleos nativos | Las mismas piezas en Pthreads, OpenMP, MPI y CUDA | Los del sílabo | ⏳ semanas 8-13 |

## 2. Organización del código

Cada ejecutable vive en un `cmd/` y toda la lógica reutilizable en `internal/`. Las flechas indican "usa".

```mermaid
flowchart TB
    subgraph Ejecutables
        cs[cargador/cmd/secuencial]
        cc[cargador/cmd/concurrente]
        es[entrenamiento/cmd/secuencial]
        ep[entrenamiento/cmd/paralelo]
        ma[entrenamiento/cmd/maestro]
        tr[entrenamiento/cmd/trabajador]
        de[herramientas/cmd/descargar]
        ex[herramientas/cmd/exportar]
        e2e[herramientas/cmd/e2e]
        an[experimentos/cmd/analisis]
        api[api/cmd/api]
    end
    subgraph internal
        aqs[aqs<br/>limpieza CSV]
        reg[registro<br/>esquema]
        per[persistencia<br/>Mongo por lotes]
        med[medicion<br/>tiempos por fase]
        par[particion<br/>hash FNV]
        ds[dataset<br/>muestra, features, modelo]
        log[logistica<br/>gradiente secuencial y paralelo]
        met[metricas<br/>F1, AUC]
        ent[entrenador<br/>flujo común]
        pro[protocolo<br/>mensajes TCP]
        clu[cluster<br/>registro de nodos]
    end
    cs --> aqs & per & med
    cc --> aqs & per & med & par
    aqs --> reg
    per --> reg
    es & ep --> ent
    ent --> ds & log & met & per
    ds --> reg
    ma --> clu & pro
    clu --> pro
    tr --> pro
    ex --> ds & per
    e2e --> aqs & per
```

## 3. Modelo de datos

```mermaid
classDiagram
    class RegistroProcesado {
        string estacion_id   "SS-CCC-NNNN"
        string timestamp_utc "RFC3339"
        float64 latitud
        float64 longitud
        float64 o3_ppm  "nullable"
        float64 no2_ppb "nullable"
        float64 so2_ppb "nullable"
        float64 co_ppm  "nullable"
        string estado
        string condado
    }
    class Muestra {
        float64[][] X  "12 features"
        float64[] Y    "1 si NO2(t+1h) > 53 ppb"
        string[] EstacionID
        int[] Mes
    }
    class ModeloGuardado {
        float64[] pesos
        float64 sesgo
        Normalizador normalizador
        float64 f1, auc
    }
    RegistroProcesado "muchos" --> "1" Muestra : dataset.Construir
    Muestra --> ModeloGuardado : entrenar + guardar
```

- Contrato del documento en Mongo: [contrato_registro_procesado.md](contrato_registro_procesado.md).
- Las 12 variables: NO2, O3, SO2 y CO actuales, latitud, longitud, y seno/coseno de la hora, el día de la semana y el mes (`dataset.NombresFeatures`).

## 4. Flujo de una fila: del CSV a MongoDB

```mermaid
flowchart LR
    F["Fila CSV de la EPA<br/>(24 columnas)"] --> P{"procesarFila"}
    P -->|medición vacía| D1[descartada: sin_medicion]
    P -->|qualifier R*| D2[descartada: qualifier_excluido]
    P -->|unidad no ppm/ppb| D3[descartada: unidad_desconocida]
    P -->|número o fecha ilegible| D4[descartada: formato_invalido]
    P -->|ppm↔ppb| CONV[convierte ×1000 o ÷1000]
    P -->|válida| V["ValorLeido<br/>(estación, hora UTC, gas, valor)"]
    CONV --> V
    V --> IX["Indice.Agregar<br/>clave = estación + hora"]
    IX -->|"ese gas ya tiene valor<br/>de otro monitor"| POC{"¿POC menor?"}
    POC -->|sí| R
    POC -->|no| IGN[se conserva el del POC menor]
    IX --> R["RegistroProcesado<br/>(une los 4 gases)"]
    R --> M[("InsertMany por lotes de 5000")]
```

En 2025: 16 972 287 filas leídas → 1625 descartadas por qualifier → 335 331 valores de monitores repetidos resueltos por POC → 10 838 667 registros únicos.

La regla del POC más bajo hace que el resultado **no dependa del orden** en que lleguen las filas. Sin ella, el cargador concurrente guardaba en esos 53 sitios un valor distinto en cada corrida. Esta condición de carrera *lógica* no la detecta `-race`, porque no hay accesos simultáneos a memoria; la descubrió la prueba end-to-end al comparar dos cargas.

## 5. Cargador concurrente: worker pool, fan-out y fan-in

```mermaid
flowchart LR
    subgraph Despacho
        LB["LimitesBloques<br/>corta cada CSV en bloques<br/>ajustados al salto de línea"]
    end
    LB --> CH1[["chan bloque"]]
    subgraph Trabajadoras["N goroutines trabajadoras (fan-out)"]
        T1[trabajadora 1]
        T2[trabajadora 2]
        TN[trabajadora N]
    end
    CH1 --> T1 & T2 & TN
    T1 & T2 & TN -->|"hash(estación) mod K"| R{reparto}
    R --> C1[["chan agregadora 1"]]
    R --> C2[["chan agregadora 2"]]
    R --> CK[["chan agregadora K"]]
    subgraph Agregadoras["K agregadoras (fan-in), cada una dueña de su índice"]
        A1[índice 1]
        A2[índice 2]
        AK[índice K]
    end
    C1 --> A1
    C2 --> A2
    CK --> AK
    A1 --> W1[escritor Mongo 1]
    A2 --> W2[escritor Mongo 2]
    AK --> WK[escritor Mongo K]
    W1 & W2 & WK --> DB[(MongoDB)]
```

Puntos de sincronización:

- `sync.WaitGroup` de las trabajadoras. Cuando terminan todas, se cierran los channels de las agregadoras, y así los `for range` terminan sin goroutines huérfanas.
- Conteo de filas con `sync.Mutex`, una vez por bloque.
- **Ningún lock en los índices**: cada clave tiene una sola agregadora dueña. Con `-agregadoras 1` se obtiene el diseño original, con una agregadora y un escritor; es la línea de comparación del Experimento 1.

## 6. Entrenamiento: una época en paralelo

```mermaid
flowchart TB
    W["pesos y sesgo actuales"] --> SPLIT["dividir X en N bloques"]
    SPLIT --> G1["goroutine 1<br/>GradienteParcial(bloque 1)"]
    SPLIT --> G2["goroutine 2<br/>GradienteParcial(bloque 2)"]
    SPLIT --> GN["goroutine N<br/>GradienteParcial(bloque N)"]
    G1 --> S1["casilla 1"]
    G2 --> S2["casilla 2"]
    GN --> SN["casilla N"]
    S1 & S2 & SN --> B{{"barrera: WaitGroup.Wait()"}}
    B --> RED["reducción: suma de las casillas"]
    RED --> UPD["actualizar pesos (solo la goroutine principal)"]
    UPD --> W
```

Esa es la estrategia `reduccion`. Las estrategias `mutex` y `atomico` reemplazan las casillas privadas por **un acumulador compartido**, actualizado por cada ejemplo con `sync.Mutex` o con CAS. Dan el mismo resultado, pero son más lentas que el secuencial (Experimento 1b, [experimento1.md](experimento1.md)).

## 7. Protocolo maestro-trabajador (TCP)

```mermaid
sequenceDiagram
    participant W1 as trabajador w1
    participant M as maestro (-nodos 2)
    participant W2 as trabajador w2
    participant W3 as trabajador w3
    W1->>M: {"tipo":"registro","worker_id":"w1"}
    M-->>W1: {"tipo":"particion_asignada","filtro_mongo":{mod:2, resto:0}}
    W2->>M: registro w2
    M-->>W2: particion_asignada resto 1
    Note over M: "clúster completo, listo para entrenar"
    W3->>M: registro w3
    M-->>W3: {"tipo":"baja","motivo":"registro_rechazado: ..."}
    Note over W1: se cae o se cierra
    M->>M: DarDeBaja(w1) → partición 0 libre
    Note over M,W2: Semana 9: parametros → gradiente_parcial → agregación por época
```

- Cada conexión la atiende su propia goroutine. El registro de nodos (`internal/cluster`) es estado compartido protegido con `sync.Mutex`.
- Contrato completo: [contrato_protocolo_tcp.md](contrato_protocolo_tcp.md).
- En varias PCs: [guia_cluster.md](guia_cluster.md).

## 8. Pipeline completo (`scripts/pipeline.ps1` / `.sh`)

```mermaid
flowchart TB
    P1["1. descargar<br/>(idempotente, verifica SHA-256)"] --> P2["2. MongoDB<br/>docker compose o mongo-local.ps1"]
    P2 --> P3["3. cargador concurrente<br/>-agregadoras 0 (~40 s)"]
    P3 --> P4["4. entrenamiento secuencial<br/>→ data/modelo.json (~1 min con la muestra)"]
    P4 --> P5{"-Experimentos?"}
    P5 -->|sí, ~1 h| P6["5. Experimentos 1b y 1<br/>→ resultados/ y JSON"]
    P5 -->|no| P7
    P6 --> P7["6. exportar<br/>→ frontend/public/datos"]
    P7 --> P8["npm run dev"]
```

## 9. Prueba end-to-end (`herramientas/cmd/e2e`)

Recorre todo lo anterior con un **subconjunto de los datos reales**, en una base de datos temporal (`saca_e2e`). No toca `data/`, `resultados/` ni la base principal, y **no repite los experimentos largos**.

```mermaid
flowchart LR
    T1[herramientas] --> T2[compilar 10 programas] --> T3[go vet] --> T4["go test -race"]
    T4 --> T5[verificar CSV] --> T6["subconjunto<br/>150 mil filas × 4"] --> T7[ping Mongo]
    T7 --> T8[cargador secuencial] --> T9["concurrente 1 y N agregadoras<br/>= mismos registros"]
    T9 --> T10[entrenamiento + modelo] --> T11["paralelo × 3 estrategias<br/>= misma pérdida"]
    T11 --> T12["clúster: 2 nodos OK,<br/>1 rechazado, 1 caída"] --> T13[exportar JSON]
    T13 --> T14["análisis del Exp. 1b<br/>(CSV existente)"] --> T15["frontend: lint, build,<br/>servir JSON"]
```

## 10. Despliegue

| Escenario | Cómo |
|---|---|
| Una PC con Docker | `docker compose up -d mongo` y luego los programas con `go run` |
| Una PC Windows sin Docker | `scripts/mongo-local.ps1` (MongoDB portable) y luego los programas |
| Clúster del grupo | Maestro y Mongo en la PC principal; trabajadores y procesos MPI en las demás. Ver [guia_cluster.md](guia_cluster.md) |
