# Experimentos 1 y 1b — escalabilidad del cargador y del entrenamiento

Resultados medidos el 13 y 14 de octubre de 2026. Las tablas completas las genera `experimentos/cmd/analisis` en [resultados/experimento1.md](../resultados/experimento1.md) y [resultados/experimento1b.md](../resultados/experimento1b.md). Los datos crudos de cada corrida están en los CSV de la misma carpeta.

## Entorno

| | |
|---|---|
| CPU | AMD Ryzen 5 3600: **6 núcleos físicos, 12 hilos** |
| RAM | 16 GB |
| Sistema | Windows 11, Go 1.27.1 |
| MongoDB | 7.0.14 local, caché WiredTiger limitada a 2 GB (ver "Incidente" al final) |
| Datos | 4 CSV horarios EPA 2025: 16 972 287 filas (4 GB) → 10 838 667 registros (estación, hora) |
| Método | 3 repeticiones de cada configuración, **mediana**. T(1) = cargador o entrenamiento **secuencial** (una goroutine), no la versión concurrente con p = 1 |

Métricas (ver `experimentos/cmd/analisis`):

- S(p) = T(1)/T(p) y E(p) = S(p)/p.
- **Karp-Flatt** e = (1/S − 1/p)/(1 − 1/p): la fracción serial "experimental". Si se mantiene constante al crecer p, la pérdida se explica por la parte serial. Si crece, hay además sobrecarga de sincronización.
- **Amdahl**: S_max(p) = 1/(f + (1−f)/p), con f **medida** en el T(1): el tiempo de las fases que en el diseño original solo puede hacer una goroutine (indexado + escritura en Mongo).

## Experimento 1: cargador (ingesta completa, con escritura en MongoDB)

Se comparan dos diseños del cargador concurrente:

- **1 agregadora** (`-agregadoras 1`, el diseño original de la semana 5): N trabajadoras leen y limpian bloques en paralelo, pero **una sola goroutine** indexa y luego **una sola** escribe en Mongo.
- **Particionado** (`-agregadoras 0`, mejora propia): N trabajadoras y N agregadoras, cada una dueña de las estaciones de su partición (hash de `estacion_id`), con **un escritor a Mongo por agregadora**. No hay ningún lock, porque dos agregadoras nunca tocan la misma clave.

### Tiempo total (mediana, segundos)

| p | 1 agregadora | S(p) | E(p) | Particionado | S(p) | E(p) |
|---:|---:|---:|---:|---:|---:|---:|
| secuencial | 132,4 | 1,00 | — | 132,4 | 1,00 | — |
| 1 | 144,0 | 0,92 | 92 % | 140,5 | 0,94 | 94 % |
| 2 | 117,4 | 1,13 | 56 % | 86,1 | 1,54 | 77 % |
| 4 | 121,3 | 1,09 | 27 % | 57,6 | 2,30 | 57 % |
| 8 | 123,1 | 1,08 | 13 % | 41,3 | 3,21 | 40 % |
| 12 | 124,5 | 1,06 | 9 % | 37,3 | 3,55 | 30 % |
| 16 | 128,1 | 1,03 | 6 % | **35,1** | **3,77** | 24 % |

### Tiempo por fase (mediana, segundos)

| Configuración | Lectura + limpieza + indexado | de ello, indexado | Escritura Mongo | Total | Memoria |
|---|---:|---:|---:|---:|---:|
| Secuencial | 39,0 | 11,3 | 94,2 | 132,4 | 3,7 GB |
| 1 agregadora, p = 8 | 29,1 | 17,8 | 94,5 | 123,1 | 4,9 GB |
| Particionado, p = 8 | 13,5 | 3,2 | 27,8 | 41,3 | 5,8 GB |
| Particionado, p = 16 | 11,1 | 2,2 | 24,2 | 35,1 | 5,9 GB |

### Análisis

1. **Amdahl explica exactamente el diseño original.**
   - En el T(1), el indexado (11,3 s) y la escritura en Mongo (94,2 s) son el 80 % del tiempo: f = 0,797.
   - La ley de Amdahl predice entonces un speedup máximo de 1/f = **1,25×**, con infinitas goroutines.
   - Con 1 agregadora se mide entre 1,03× y 1,13×, siempre por debajo de esa curva.
   - Karp-Flatt da e = 0,77-0,97, aproximadamente igual a f: la pérdida es la parte serial y no otra cosa.

   Agregar goroutines trabajadoras no sirve, porque solo aceleran el 20 % paralelo.
2. **Para superar el límite de Amdahl hay que achicar la parte serial, no agregar hilos.**
   - El diseño particionado convierte en paralelo lo que antes era serial: el indexado baja de 17,8 a 3,2 s (p = 8) y Mongo de 94,5 a 27,8 s.
   - Por eso supera la curva de Amdahl del diseño original (3,77× frente a 1,25× máximo teórico).
   - No es una contradicción: Amdahl se aplica a un programa fijo, y aquí se cambió el programa.
   - En el diseño nuevo, Karp-Flatt se estabiliza en e ≈ 0,21. Lo que sigue siendo serial es el despacho de bloques, el arranque de la conexión y la escritura de MongoDB, que su propio motor serializa.
3. **p = 1 es más lento que el secuencial** (0,92-0,94×). Es la sobrecarga pura de la concurrencia: enviar 16,9 M valores por channels, sin nadie que trabaje en paralelo. Por eso T(1) se toma del programa secuencial y no del concurrente con p = 1.
4. **Por encima de p = 6 la ganancia se aplana** (3,21× con p = 8 frente a 3,77× con p = 16, con eficiencia en caída). Hay dos causas:
   - la máquina tiene **6 núcleos físicos**: los 12 hilos lógicos comparten unidades de ejecución;
   - MongoDB, en la misma PC, compite por la CPU y el disco con el cargador.
5. **Memoria.** El particionado usa más memoria (5,9 GB frente a 3,7 GB del secuencial): son N índices y N buffers de escritura a la vez. Es el intercambio clásico de memoria por tiempo. Antes de la corrección del Paso 1, el cargador original llegaba a 8,7 GB.

### Sin MongoDB: solo el procesamiento

Con `-sin-mongo` (medido aparte con p = 8 sobre los mismos datos), la lectura + limpieza + indexado tarda:

- **27,0 s** con 1 agregadora;
- **13,1 s** con 4 agregadoras;
- **11,5 s** con 8 agregadoras.

Confirma que la agregadora única era el cuello de botella del procesamiento, y no solo de la escritura.

## Experimento 1b: entrenamiento en memoria compartida

Se entrena la regresión logística: 146 715 ejemplos, 12 variables, 200 épocas. El gradiente de cada época se reparte entre p goroutines con tres estrategias de sincronización ([internal/logistica/paralelo.go](../internal/logistica/paralelo.go)). T(1) = 2,396 s (secuencial).

| p | Reducción (casillas privadas) | | Mutex por ejemplo | | Atómico (CAS) por ejemplo | |
|---:|---:|---:|---:|---:|---:|---:|
| | T(p) [s] | S(p) | T(p) [s] | S(p) | T(p) [s] | S(p) |
| 1 | 2,393 | 1,00 | 2,376 | 1,01 | 2,406 | 1,00 |
| 2 | 1,204 | 1,99 | 2,058 | 1,16 | 3,414 | 0,70 |
| 4 | 0,662 | 3,62 | 3,799 | 0,63 | 3,883 | 0,62 |
| 8 | **0,392** | **6,11** | 5,214 | 0,46 | 4,192 | 0,57 |
| 12 | 0,448 | 5,35 | 5,254 | 0,46 | 4,101 | 0,58 |

### Análisis

1. **Las tres estrategias son correctas**: dan la misma pérdida final (los tests con `-race` lo verifican). Solo cambia el costo de sincronizar.
2. **Reducción ≈ lineal hasta 6-8 goroutines** (E = 99 % con p = 2 y 76 % con p = 8). Es el equivalente de `#pragma omp parallel for reduction`: cada goroutine acumula en privado y solo hay una barrera y una suma por época.
3. **Mutex y CAS de grano fino empeoran al agregar goroutines.**
   - Con p = 8 son 2× **más lentos que el secuencial**, con Karp-Flatt > 1: la sincronización cuesta más que el trabajo que protege (13 sumas por ejemplo).
   - El CAS evita el bloqueo, pero no la **contención en la misma línea de caché**: los 8 núcleos se disputan los mismos 14 float64, y los reintentos crecen con p.
   - Es el adelanto del Experimento 3 (mutex frente a atómicos sobre el vector de parámetros del maestro).
4. **p = 12 rinde menos que p = 8** con la reducción. Con 6 núcleos físicos, los hilos hermanos de hyper-threading compiten por las mismas unidades de punto flotante, y la barrera por época pesa más con bloques más pequeños. El Karp-Flatt sube de 0,044 a 0,113.

## Conclusiones para el informe

- La ley de Amdahl **predijo** el estancamiento del diseño original con la fracción serial medida. El rediseño (particionar el estado compartido y no protegerlo) multiplicó el speedup por 3, de 1,13× a 3,77×.
- En los dos experimentos, la lección es la misma: **la mejor sincronización es no necesitarla**. Hay que repartir la propiedad de los datos (agregadoras por partición, acumuladores privados) y sincronizar una sola vez al final.
- Los límites que quedan son de hardware (6 núcleos físicos) y de I/O (MongoDB en la misma máquina). En el clúster ([guia_cluster.md](guia_cluster.md)) Mongo podrá ir en otra PC.

## Incidente durante la medición

El primer intento del Experimento 1 se trabó en la repetición 2: una escritura a MongoDB quedó bloqueada 35 minutos, con avisos `WTEVICT` en el log.

- **Causa:** la caché de WiredTiger usa por defecto ~50 % de la RAM (~7,5 GB). Sumada a los ~5 GB del cargador, en una PC de 16 GB dejó sin memoria al desalojo de caché.
- **Solución:** `--wiredTigerCacheSizeGB 2`, ya incluida en `docker-compose.yml`.
- Se descartaron las 17 corridas de ese intento y el experimento se repitió completo con la nueva configuración, para no mezclar condiciones. Por eso los tiempos de Mongo de este documento (~94 s en el secuencial) son mayores que los de las primeras pruebas (~85 s): la caché es menor.
