# Guía para la presentación del avance: el clúster distribuido de SACA

Guía para que el grupo prepare y dé la presentación **virtual** del avance del clúster. Cómo ejecutar cada cosa está en la guía de cada rol: [maestro](cluster/guia_maestro.md), [nodo sin GPU NVIDIA](cluster/guia_nodo_sin_nvidia.md) y [nodo con GPU NVIDIA](cluster/guia_nodo_con_nvidia.md). Aquí está **qué decir, en qué orden, quién lo hace y qué hacer si algo falla**.

> **Para la exposición de 10 minutos con un solo expositor, usa [guion_exposicion.md](guion_exposicion.md)**: el guion minuto a minuto de las 10 diapositivas de la página (qué decir, qué hacer, la demo y las preguntas probables). Esta guía queda como referencia de la versión con todo el grupo.

## 1. Lo que el docente debe llevarse (3 ideas)

1. **El clúster funciona de verdad y da el mismo resultado.** Varias PCs del grupo, cada una en su casa, entrenan juntas el modelo. La pérdida final es idéntica a la del entrenamiento secuencial (diferencia del orden de 1e-15): la descomposición de datos es correcta.
2. **Es heterogéneo.** Unas PCs calculan con goroutines en la CPU y otras con CUDA en su GPU NVIDIA, bajo el mismo protocolo. Es el modelo jerárquico de HPC: paso de mensajes entre nodos, memoria compartida o GPU dentro de cada nodo.
3. **Medimos cuándo conviene un clúster.** Con una muestra pequeña, el 72 % del tiempo es red y no se gana nada. Con **todos los datos de NO2** (2,5 M ejemplos), una GPU remota por internet entrena **18 veces más rápido** que la CPU local. Pero sumar nodos más lentos frena a todos: en modo síncrono el más lento marca el ritmo. Es Amdahl y balanceo de carga, medidos en un clúster real.

## 2. Roles

| Rol | Quién | Qué hace | Su guía |
|---|---|---|---|
| Presentador principal | ___ | Conduce, muestra las diapositivas y narra la demo | esta |
| Operador del maestro | Piero (tiene la muestra y el puerto abierto) | Comparte su pantalla durante la demo y lanza `maestro.exe` | [guia_maestro.md](cluster/guia_maestro.md), paso 8 |
| Nodo con GPU NVIDIA 1 | augusto (RTX 3060 Ti) | Corre `nodo.ps1`; comparte pantalla cuando se le pide (validación CUDA) | [guia_nodo_con_nvidia.md](cluster/guia_nodo_con_nvidia.md), paso 10 |
| Nodo con GPU NVIDIA 2 | josue (RTX 4060 Laptop) | Ídem | ídem |
| Nodo sin GPU NVIDIA | angelo (Fedora, CPU) | Corren `nodo.ps1`; uno de ellos hace la "caída" del nodo | [guia_nodo_sin_nvidia.md](cluster/guia_nodo_sin_nvidia.md), paso 7 |
| Respaldo | ___ | Tiene listo el plan B (sección 6) y la grabación del ensayo | [guia_maestro.md](cluster/guia_maestro.md), paso 8 |

Todos con la cámara apagada y el micrófono silenciado, salvo quien habla. Las terminales con letra grande: `Ctrl` + rueda del mouse en Windows Terminal.

## 3. Guion (12-15 minutos): se presenta desde la página

Toda la presentación está en la página del proyecto: botón **Presentación**, con 10 diapositivas que se navegan con ← → o la barra espaciadora. Las cifras salen de los resultados reales, y la diapositiva 6 controla el clúster en vivo. El operador del maestro comparte la pantalla del navegador en pantalla completa (`F11`).

| Min | Diapositiva | Quién | Qué decir |
|---|---|---|---|
| 0-1 | 1 · El problema | Presentador | Alerta temprana; 16,97 M mediciones; ¿superará el NO₂ el umbral AQI 51 en la próxima hora? El foco es cómo se paraleliza |
| 1-2 | 2 · Los datos en el mapa | Presentador | El cargador concurrente procesó los 4 CSV; elegir una estación y mostrar su serie con la probabilidad del modelo |
| 2-3 | 3 · Por qué se puede distribuir | Presentador | El gradiente es una suma: scatter, broadcast, reduce (*parameter server*). Mismo modelo que el secuencial |
| 3-4 | 4 · De dónde venimos | Presentador | Memoria compartida: reducción S(8) = 6,11; mutex y CAS más lentos que el secuencial (contención) |
| 4-5 | 5 · El clúster del grupo | Presentador | MIMD de memoria distribuida y heterogéneo; los nodos **conectados ahora** aparecen en el diagrama |
| 5-10 | **6 · Demo en vivo** | Operador del maestro | Sección 4 |
| 10-11 | 7 · Resultados | Presentador | Muestra de prueba frente a completa: con pocos datos 72 % de red; con todos, 1 GPU S = 18,2 |
| 11-12 | 8 · Lecciones | Presentador | Amdahl y tamaño; más nodos ≠ más rápido; el más lento marca el ritmo; correctitud |
| 12-13 | 9 · Temas del curso | Presentador | Recorrer la tabla por unidad: tema, dónde está, evidencia medida y estado |
| 13-14 | 10 · Próximos pasos | Presentador | Reparto proporcional, MPI, CUDA *shuffle*, Pthreads/OpenMP nativos, tolerancia a fallos |
| 14-15 | Preguntas | Todos | Sección 7 |

## 4. La demo, paso a paso (diapositiva 6)

**Antes de empezar a hablar:** el maestro está en modo servidor (`maestro -web`, [guia_maestro.md](cluster/guia_maestro.md), paso 8) y **los tres nodos ya aparecen en la sala** de la página. Cada uno ejecutó su comando una sola vez.

1. **La sala.** Señalar cada nodo: nombre, **GPU** o **CPU**, la tarjeta y la IP `100.x` (PCs en casas distintas). Tema: memoria distribuida, solo mensajes.
2. **1 GPU.** Marcar solo **augusto** → **Entrenar** (200 épocas, sin verificar). En ~2 s:
   - "Tiempo por época" muestra cuánto es cálculo y cuánto red;
   - "Speedup en vivo" sube a ~18× frente al secuencial;
   - la franja inferior narra el concepto: scatter, broadcast + reduce, CUDA.
3. **2 GPUs.** Marcar **augusto + josue** → **Entrenar**. Es **más lento**: con 3 ms de cálculo por época, repartir ahorra poco y la red de un nodo más pesa más.
4. **2 GPUs + 1 CPU.** Marcar los **tres** → **Entrenar**. En "Cálculo por época de cada nodo", la línea de angelo (CPU) queda arriba: la franja dice "barrera síncrona: esperando al más lento (angelo)". Es el balanceo de carga.
5. **Comparación.** Bajar a "Corridas de esta sesión": las 3 barras medidas en vivo, con S y E. Coinciden con la diapositiva 7.
6. **(Opcional) Verificación.** Repetir 1 GPU con **Verificar contra el secuencial**. Al final: "mismo modelo que el secuencial (diferencia ~1e-15)".
7. **Caída de un nodo.** Poner **2000 épocas** con los tres → **Entrenar**, y a mitad pedir a angelo que presione `Ctrl` + `C` **en su terminal**. La corrida aparece como **abortada** con "el nodo angelo se cayó"; augusto y josue vuelven a la sala. Angelo relanza su comando y reaparece solo. La tolerancia a fallos completa es el Experimento 4.

## 5. Contenido de las diapositivas (ya incluido en la página)

Referencia de lo que dice cada diapositiva de la página, por si hay que exportar algo a otro formato.

1. **Problema.** Una frase de impacto social y la pregunta predictiva. Datos: 4 contaminantes, 16 972 287 filas, 1447 estaciones.
2. **Por qué se puede distribuir.** La fórmula ∇L = Σᵢ ∇ℓᵢ partida en Σ_{nodo 1} + Σ_{nodo 2} + …; la figura del *parameter server*. Conceptos del sílabo: **descomposición de datos** (semana 10) y **paso de mensajes** (semanas 3-4).
3. **Arquitectura del clúster.** La figura de [cluster/README.md](cluster/README.md). Clasificación: **multicomputador MIMD de memoria distribuida, heterogéneo** (semana 2); dentro de cada nodo, memoria compartida o GPU SIMT. Protocolo: JSON por TCP (registro, partición, parámetros, gradiente, finalizado, baja).
4. **De dónde venimos.** Experimento 1 (cargador, S(16) = 3,77) y 1b (entrenamiento con goroutines, S(8) = 6,11 con reducción; mutex y CAS más lentos que el secuencial). El clúster reutiliza la misma reducción.
5. **Resultados del Experimento 2: clúster real por internet con todos los datos de NO2.**

   Muestra completa: **2 499 636 ejemplos** de entrenamiento (3,3 M en total, 337 MB). 200 épocas, mediana de 3 a 5 repeticiones (incluye las corridas lanzadas desde la página), 15/10/2026. Maestro en Windows (Ryzen 5 3600); nodos en 3 casas distintas unidos por Tailscale, todos con conexión directa (`tailscale ping` de 5, 6 y 17 ms).

   | Configuración | Nodos | T(p) | **Speedup** | Cálculo del más lento | Red y espera | % comunicación |
   |---|---|---:|---:|---:|---:|---:|
   | Secuencial (1 hilo de CPU, PC del maestro) | — | 41,5 s | 1,0 | 207 ms | — | — |
   | **1 GPU** | augusto (RTX 3060 Ti) | **2,28 s** | **18,2** | 3,1 ms | 8,3 ms | 73 % |
   | 2 GPUs | + josue (RTX 4060 Laptop) | 3,92 s | 10,6 | 5,4 ms | 12,7 ms | 70 % |
   | 2 GPUs + 1 CPU | + angelo (Ryzen 7 7735HS, 16 hilos) | 7,29 s | 5,7 | **18,1 ms (angelo)** | 18,1 ms | 50 % |

   En todas las corridas, la pérdida final fue **idéntica** a la secuencial (`0.132576`, diferencia ~1e-15). Fuente: `resultados/experimento2.md` y `.csv`.

   **Comparación con la muestra de prueba** (146 mil ejemplos, 1 nodo CPU por Tailscale): 2,8 ms de cálculo y 7,4 ms de red por época, 72 % de comunicación y S = 1,3. Con 17 veces más datos, el cálculo por época en una CPU pasa de ~12 ms a ~207 ms mientras la red sigue en ~8 ms. **Ahí el clúster se justifica:** una sola GPU remota, por internet, entrena 18 veces más rápido que la CPU local. Resultados de la muestra de prueba: `resultados/experimento2_prueba.md`.

6. **Lecciones** (cada una sale de una fila de la tabla).
   - **El tamaño del problema decide (Amdahl).** La red cuesta lo mismo con 146 mil que con 2,5 M ejemplos, porque por época solo viajan 13 números. Lo que crece es el cálculo. Con pocos datos, la parte no paralelizable (la comunicación síncrona) domina; con todos los datos, el cálculo distribuido gana.
   - **Más nodos no siempre es más rápido.** Pasar de 1 a 2 GPUs **empeoró** el tiempo (2,3 s → 3,9 s). La GPU es tan rápida (~3 ms por época para 2,5 M ejemplos) que partir el trabajo ahorra apenas 1-2 ms, mientras que cada nodo extra agrega la espera de su red. El maestro siempre espera al nodo con peor conexión.
   - **Heterogeneidad: el más lento marca el ritmo.** Con el nodo CPU, cada época espera a angelo (18 ms), aunque augusto termina su parte en ~1,3 ms. Las particiones son casi iguales (807 mil, 885 mil y 807 mil ejemplos), pero las máquinas no: la RTX 3060 Ti es ~14 veces más rápida que la CPU de angelo.
   - **La solución: reparto proporcional al rendimiento (Experimento 9).** Si cada nodo recibiera ejemplos según su velocidad medida (casi todo a augusto, poco a angelo), el cálculo por época bajaría de ~18 ms a ~3,5 ms. Es el siguiente paso del proyecto y el tema de balanceo de carga de la semana 10 del sílabo.
   - **Correctitud ante todo.** Con CPU, GPU, Windows, Linux, 1, 2 o 3 nodos, el modelo resultante es el mismo bit a bit (salvo redondeo ~1e-15), y la GPU se valida contra la CPU antes de usarse.
   - **No se miden aquí:** Karp-Flatt y Amdahl clásicos. La base T(1) es un hilo de CPU y los nodos son GPUs, así que la "eficiencia" sale mayor al 100 % y esas fórmulas no aplican. Para eso está el Experimento 1b, en memoria compartida (f ≈ 0,31 en local).
7. **Próximos pasos.** MPI (`MPI_Bcast` + `MPI_Allreduce`) frente a este TCP en Go (Experimento 7); ping-pong de latencia y ancho de banda (7a); CUDA con *warp shuffles* y punto de cruce CPU/GPU (8); clúster híbrido con reparto proporcional (9); tolerancia a la caída de nodos.

## 6. Plan B (si algo falla en vivo)

| Falla | Respuesta (sin perder más de 30 s) |
|---|---|
| Un nodo no aparece en la sala | Seguir sin él: en la página se desmarca y se entrena con los demás. Decir "un nodo tiene problemas de red: es justo el tipo de fallo que el sistema detecta" |
| La página dice "maestro sin conexión" | El maestro no corre en modo servidor: relanzarlo (paso 8.1 de la guía del maestro). Los nodos vuelven solos a la sala en unos segundos |
| Tailscale o internet del maestro caídos | **Plan B local:** `Ctrl` + `C` al maestro y `scripts\cluster-local.ps1 -Nodos 2 -Web -Muestra data/muestra_prueba.gob`. La página sigue igual, con 2 nodos locales en la sala. Se explica igual: son procesos que solo se hablan por TCP |
| La GPU no valida o no compila | El nodo sigue con la CPU automáticamente. Mostrar el aviso en el log como prueba de que se valida |
| Falla todo | El respaldo reproduce la **grabación del ensayo general** (grabar la pantalla del maestro con `Win + Alt + R` o con OBS) |

## 7. Preguntas probables del docente

- **¿Cómo saben que el resultado distribuido es correcto?** El maestro entrena también en secuencial y compara la pérdida final: la diferencia es del orden de 1e-15, puro redondeo por el distinto orden de las sumas. Además, un test automático (`go test -race ./internal/distribuido`) lo comprueba con 3 nodos, uno de ellos con un motor externo.
- **¿Dónde está la sección crítica y cómo evitan condiciones de carrera?** El modelo solo lo modifica una goroutine, el bucle de entrenamiento del maestro. Las goroutines que atienden a cada nodo le pasan los gradientes por un **channel**, así que no hay estado compartido. El registro de nodos sí es compartido y está protegido por un `sync.Mutex`. Todo pasa `go test -race`.
- **¿Por qué TCP y no MPI?** Para una demo entre casas por internet, MPI exige el mismo usuario y la misma versión en todas las PCs, más `smpd` y puertos abiertos en ambos sentidos. Es frágil. TCP en Go solo necesita que los nodos lleguen al maestro. MPI viene después y se comparará con este TCP en el Experimento 7.
- **¿Por qué el speedup por internet es bajo, o menor que 1?** Por Amdahl: en modo síncrono, cada época paga una ida y vuelta de red de unos 30-80 ms, y el cálculo por nodo dura unos pocos ms. La fracción no paralelizable domina. Es el resultado esperado con esta muestra (147 mil ejemplos). Con el dataset completo el cálculo por época crece unas 100 veces y la red pesa mucho menos.
- **¿Qué hace exactamente la GPU?** Cada hilo CUDA procesa ejemplos (*grid-stride loop*) y acumula su gradiente. Cada bloque reduce en **memoria compartida con un árbol** (`__syncthreads`), y la CPU suma los resultados de los bloques. Los datos se copian una sola vez; por época solo viajan 13 números.
- **¿Qué pasa si un nodo se cae?** En modo síncrono, el maestro lo detecta (por la desconexión o tras 2 min sin gradiente) y aborta limpio. Redistribuir su partición o usar el modo asíncrono es el Experimento 4.
- **¿Por qué partir por estación y no por filas?** Para que cada nodo tenga las series completas de sus estaciones. Hará falta cuando se agreguen los rezagos t-1..t-24, que se calculan dentro de cada estación sin comunicarse.

## 8. Listas de verificación

**El día anterior (ensayo general)**
- [ ] Todos completaron el "Ensayo individual" de su guía (`cluster-local.ps1` con OK) y enviaron sus datos al grupo
- [ ] Inventario lleno con las IPs de Tailscale
- [ ] Ensayo general hecho, tabla de Tailscale llena y CSV subido al repo
- [ ] Recorrido completo de las 10 diapositivas de la página, con la demo de la diapositiva 6 funcionando con los tres nodos
- [ ] Grabación del ensayo guardada por el respaldo

**1 hora antes**
- [ ] `git pull` y el mismo commit en todas las PCs
- [ ] Huella `c2c8f8832a30` en todas (la muestra completa de 337 MB)
- [ ] Tailscale conectado; `tailscale ping` al maestro desde cada nodo
- [ ] Suspensión desactivada, notificaciones silenciadas, OneDrive en pausa
- [ ] Nodos GPU: `bin\gradiente_cuda.exe --info` funciona

**5 minutos antes**
- [ ] Maestro en modo servidor corriendo y la página abierta en **Presentación**, diapositiva 1, en pantalla completa
- [ ] Los tres nodos ya aparecen en la sala (diapositiva 5 o 6)
- [ ] El respaldo con `cluster-local.ps1 -Web` y la grabación a mano
