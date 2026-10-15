# Guía para la presentación del avance: el clúster distribuido de SACA

Guía para que el grupo prepare y dé la presentación **virtual** del avance del clúster. Cómo ejecutar cada cosa está en la guía de cada rol: [maestro](cluster/guia_maestro.md), [nodo sin GPU NVIDIA](cluster/guia_nodo_sin_nvidia.md) y [nodo con GPU NVIDIA](cluster/guia_nodo_con_nvidia.md). Aquí está **qué decir, en qué orden, quién lo hace y qué hacer si algo falla**.

## 1. Lo que el docente debe llevarse (3 ideas)

1. **El clúster funciona de verdad y da el mismo resultado.** Varias PCs del grupo, cada una en su casa, entrenan juntas el modelo. La pérdida final es idéntica a la del entrenamiento secuencial (diferencia del orden de 1e-15): la descomposición de datos es correcta.
2. **Es heterogéneo.** Unas PCs calculan con goroutines en la CPU y otras con CUDA en su GPU NVIDIA, bajo el mismo protocolo. Es el modelo jerárquico de HPC: paso de mensajes entre nodos, memoria compartida o GPU dentro de cada nodo.
3. **Medimos el costo de la comunicación.** En una PC la red cuesta un 3-5 % por época. Por internet domina, porque cada época calcula unos pocos ms y espera decenas de ms de red. Es Amdahl en vivo: el speedup lo limita la parte que no se paraleliza (la comunicación síncrona), no la cantidad de nodos.

## 2. Roles

| Rol | Quién | Qué hace | Su guía |
|---|---|---|---|
| Presentador principal | ___ | Conduce, muestra las diapositivas y narra la demo | esta |
| Operador del maestro | ___ (quien tiene la muestra y el puerto abierto) | Comparte su pantalla durante la demo y lanza `maestro.exe` | [guia_maestro.md](cluster/guia_maestro.md), paso 8 |
| Nodo con GPU NVIDIA 1 | ___ | Corre `nodo.ps1`; comparte pantalla cuando se le pide (validación CUDA) | [guia_nodo_con_nvidia.md](cluster/guia_nodo_con_nvidia.md), paso 10 |
| Nodo con GPU NVIDIA 2 | ___ | Ídem | ídem |
| Nodos sin GPU NVIDIA | ___, ___ | Corren `nodo.ps1`; uno de ellos hace la "caída" del nodo | [guia_nodo_sin_nvidia.md](cluster/guia_nodo_sin_nvidia.md), paso 7 |
| Respaldo | ___ | Tiene listo el plan B (sección 6) y la grabación del ensayo | [guia_maestro.md](cluster/guia_maestro.md), paso 8 |

Todos con la cámara apagada y el micrófono silenciado, salvo quien habla. Las terminales con letra grande: `Ctrl` + rueda del mouse en Windows Terminal.

## 3. Guion (12-15 minutos)

| Min | Bloque | Quién | Qué se muestra |
|---|---|---|---|
| 0-1 | Problema | Presentador | Diapositiva 1: alerta temprana de contaminación con 16,97 M mediciones horarias de la EPA. Regresión logística: ¿superará el NO2 el umbral AQI 51 en la próxima hora? |
| 1-3 | Por qué distribuir y cómo | Presentador | Diapositiva 2: el gradiente es una suma sobre ejemplos, así que cada nodo suma su parte y el maestro las combina (*parameter server* síncrono). Diapositiva 3: arquitectura (maestro, nodos CPU y GPU, Tailscale) |
| 3-4 | Lo que ya teníamos | Presentador | Diapositiva 4: Experimentos 1 y 1b en memoria compartida (S(8) = 6,11 con reducción). El clúster es el siguiente nivel |
| 4-10 | **Demo en vivo** | Operador del maestro + nodos | Sección 4 |
| 10-12 | Resultados | Presentador | Diapositiva 5: Experimento 2 local frente a Tailscale; desglose cálculo/comunicación |
| 12-13 | Lecciones | Presentador | Diapositiva 6: Amdahl, latencia, desbalance, heterogeneidad |
| 13-14 | Próximos pasos | Presentador | Diapositiva 7: MPI (Experimentos 7 y 7a), kernels CUDA con *shuffle* (Experimento 8), reparto proporcional al rendimiento (Experimento 9) |
| 14-15 | Preguntas | Todos | Sección 7 |

## 4. La demo, paso a paso

Antes de empezar a hablar, **todos los nodos ya están con `nodo.ps1` abierto y reintentando**, y el maestro todavía no arrancó.

1. **Arranque del maestro.** El operador comparte pantalla y ejecuta:

   ```powershell
   .\bin\maestro.exe -nodos 4 -modo tcp_tailscale -maquina "demo: 4 PCs por Tailscale" -resultado resultados\experimento2.csv
   ```

   Narrar: "el maestro carga la muestra, calcula su huella y espera 4 nodos".
2. **Registro de los nodos.** En segundos aparecen las líneas `nodo luis registrado desde 100.x.y.z → partición 2/4, motor cuda (NVIDIA ...)`. Señalar:
   - las **IPs 100.x**: son PCs en casas distintas;
   - el **motor** de cada nodo: go o cuda;
   - la **partición**: cada nodo tiene estaciones distintas, sin solaparse.
3. **(Opcional, 30 s) Rechazo por datos distintos.** Si se preparó un nodo con una muestra alterada, mostrar el `rechazado, su muestra tiene otra huella`. Mensaje: el maestro protege la corrección del resultado.
4. **Entrenamiento.** Corre solo, en unos 10 s por internet. Leer una línea: "por época, el nodo más lento calcula X ms y la red se lleva Y ms".
5. **Nodo GPU.** El nodo GPU 1 comparte pantalla 20 s: `motor cuda validado contra el de Go: mismo gradiente`. Mensaje: antes de usar la GPU se comprueba que da lo mismo que la CPU.
6. **Tabla final y verificación.** Volver al maestro:
   - tabla por nodo: quién calculó qué y cuánto tardó;
   - `verificación: ... diferencia 2.6e-15 → OK, mismo modelo`;
   - el speedup (o *slowdown*) frente al secuencial, que conecta con las lecciones.
7. **Caída de un nodo.** El maestro relanza con `-epocas 2000`. A mitad, un nodo CPU cierra su ventana. El maestro aborta con `el nodo X se cayó; el modo síncrono no puede seguir sin su partición`. Mensaje: se detecta y no se cuelga. La tolerancia a fallos (redistribuir la partición o el modo asíncrono) es el siguiente paso, en el Experimento 4.

## 5. Contenido sugerido de las diapositivas

1. **Problema.** Una frase de impacto social y la pregunta predictiva. Datos: 4 contaminantes, 16 972 287 filas, 1447 estaciones.
2. **Por qué se puede distribuir.** La fórmula ∇L = Σᵢ ∇ℓᵢ partida en Σ_{nodo 1} + Σ_{nodo 2} + …; la figura del *parameter server*. Conceptos del sílabo: **descomposición de datos** (semana 10) y **paso de mensajes** (semanas 3-4).
3. **Arquitectura del clúster.** La figura de [cluster/README.md](cluster/README.md). Clasificación: **multicomputador MIMD de memoria distribuida, heterogéneo** (semana 2); dentro de cada nodo, memoria compartida o GPU SIMT. Protocolo: JSON por TCP (registro, partición, parámetros, gradiente, finalizado, baja).
4. **De dónde venimos.** Experimento 1 (cargador, S(16) = 3,77) y 1b (entrenamiento con goroutines, S(8) = 6,11 con reducción; mutex y CAS más lentos que el secuencial). El clúster reutiliza la misma reducción.
5. **Resultados del Experimento 2.**

   | Configuración | p | T(p) | S(p) | % comunicación |
   |---|---:|---:|---:|---:|
   | Secuencial (1 PC) | 1 | 2,68 s | 1,00 | — |
   | TCP local (procesos, 1 goroutine por nodo) | 2 | 1,65 s | 1,62 | 3 % |
   | TCP local | 4 | 1,37 s | 1,96 | 4 % |
   | TCP local | 8 | 1,07 s | 2,51 | 4-5 % |
   | **Tailscale: 2 casas por internet** (15/10, 1 nodo Fedora de 16 hilos) | 1 | 2,06 s | 1,30* | **72 %** |

   Mediana de 3 repeticiones, 200 épocas; maestro en un Ryzen 5 3600. Fuente: `resultados/experimento2.md`.

   **Por Tailscale** (maestro en Windows y nodo en Fedora, conexión directa, `tailscale ping` de 29 ms), cada época dura unos 10 ms:

   | | ms por época |
   |---|---:|
   | Cálculo del nodo | 2,8 |
   | Red (ida y vuelta) | 7,4 |
   | **Total** | **10,3** |

   El **72 % del tiempo es comunicación**, y en una corrida con más jitter de internet llegó al 83 %. Pérdida final idéntica a la secuencial (diferencia 2,5e-15). La caída del nodo se detectó y el maestro abortó limpio en la época 239.

   \* El S(1) = 1,30 **no** viene de distribuir. El nodo calcula con 16 hilos y el secuencial con 1: su CPU hace en 2,8 ms lo que el secuencial hace en ~12 ms, y la red se come casi toda esa ganancia. No se compara con la serie local, que usa 1 hilo por nodo.
6. **Lecciones.**
   - **Amdahl y Karp-Flatt:** localmente f ≈ 0,31, y el speedup máximo ronda 3,2.
   - **Latencia:** por internet, la comunicación síncrona por época (decenas de ms) supera al cálculo (pocos ms). El clúster gana solo si el trabajo por época crece: todos los registros en lugar de una muestra, más variables con los rezagos.
   - **Desbalance:** la partición por hash de estación no da partes iguales (con 4 nodos, entre 34 mil y 39 mil ejemplos). El nodo con más datos, o el más lento, marca el ritmo.
   - **Heterogeneidad:** una GPU es mucho más rápida calculando, pero en modo síncrono espera al nodo CPU más lento. De ahí el **reparto proporcional al rendimiento** del Experimento 9.
7. **Próximos pasos.** MPI (`MPI_Bcast` + `MPI_Allreduce`) frente a este TCP en Go (Experimento 7); ping-pong de latencia y ancho de banda (7a); CUDA con *warp shuffles* y punto de cruce CPU/GPU (8); clúster híbrido con reparto proporcional (9); tolerancia a la caída de nodos.

## 6. Plan B (si algo falla en vivo)

| Falla | Respuesta (sin perder más de 30 s) |
|---|---|
| Un nodo no conecta | Seguir sin él: el operador reinicia el maestro con `-nodos` uno menor. Decir "un nodo tiene problemas de red: es justo el tipo de fallo que el sistema detecta" |
| Tailscale o internet del maestro caídos | **Plan B local:** el operador ejecuta `scripts\cluster-local.ps1 -Nodos 4 -Ventanas`, que abre el maestro y 4 trabajadores en su PC. Se explica igual: son procesos que solo se hablan por TCP |
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
- [ ] Diapositiva 5 actualizada con los números del ensayo
- [ ] Grabación del ensayo guardada por el respaldo

**1 hora antes**
- [ ] `git pull` y el mismo commit en todas las PCs
- [ ] Huella `5e90db4b1158` en todas
- [ ] Tailscale conectado; `tailscale ping` al maestro desde cada nodo
- [ ] Suspensión desactivada, notificaciones silenciadas, OneDrive en pausa
- [ ] Nodos GPU: `bin\gradiente_cuda.exe --info` funciona

**5 minutos antes**
- [ ] Todos los nodos con `nodo.ps1` abierto y reintentando
- [ ] El operador con el comando del maestro escrito, sin ejecutar
- [ ] El respaldo con `cluster-local.ps1` y la grabación a mano
