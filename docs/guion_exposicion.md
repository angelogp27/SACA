# Guion de la exposición: SACA en 10 minutos

Guion para **un solo expositor** (Piero Carlinho Vargas Chota), que maneja la página y el maestro.

**Grupo** (orden por apellido): Cerna Sifuentes, Augusto Nicolas · Goitia Peves, Angelo Diego · Grijalva De La Cruz, Rony · Leon Garcia, Angel Gabriel · Martines Cancho, Josue · Vargas Chota, Piero Carlinho · Yanyachi Perez, Pedro · Zarate Vilchez, Axhell. Docente: Javier Antonio Prudencio Vidal.

Las diapositivas tienen **más contenido del que se dice**, a propósito: tablas, código y cifras de respaldo. **No los leas.** Di la idea central, señala el elemento y deja que la pantalla sostenga el detalle. Si el docente pregunta, la respuesta ya está a la vista. La presentación es la página del proyecto en modo **Presentación**: 10 diapositivas y la demo del clúster en vivo.

En cada diapositiva tienes tres cosas:
- **Qué hacer:** clics y qué señalar en la pantalla.
- **Qué decir:** texto casi literal, para leerlo en voz alta en los ensayos y luego decirlo con tus palabras.
- **Transición:** la frase que lleva a la siguiente diapositiva.

Atajos en la página:

| Tecla | Acción |
|---|---|
| `→` / espacio | Siguiente |
| `←` | Anterior |
| `F` | Pantalla completa |
| `N` | Notas del orador (resumen de este guion, debajo de la diapositiva) |
| `T` | Reiniciar el cronómetro (abajo a la derecha; se pone rojo al pasar de 10:00) |

---

## Cronograma

| Diapositiva | Tema | Tiempo | Acumulado |
|---|---|---:|---:|
| 1 | Portada | 0:20 | 0:20 |
| 2 | El problema | 0:40 | 1:00 |
| 3 | Tres niveles de paralelismo | 0:50 | 1:50 |
| 4 | Memoria compartida | 1:00 | 2:50 |
| 5 | Distribución: colectivas | 1:00 | 3:50 |
| 6 | El clúster y CUDA | 0:50 | 4:40 |
| 7 | **Demo en vivo** | 3:00 | 7:40 |
| 8 | Resultados: Amdahl | 1:00 | 8:40 |
| 9 | Conclusiones | 0:50 | 9:30 |
| 10 | Temas del curso | 0:30 | 10:00 |

**Si vas atrasado al llegar a la demo** (más de 5:00 en el cronómetro), haz solo 2 corridas (1 GPU y los tres), y en la diapositiva 10 di solo la frase final.

---

## Antes de empezar (15 minutos antes)

1. **Maestro en modo servidor** (PowerShell 1):
   ```powershell
   cd "C:\Users\piero\OneDrive\Documentos\Universidad\progra paralela\SACA"
   .\bin\maestro.exe -web 127.0.0.1:8080 -modo tcp_tailscale -resultado resultados\experimento2.csv
   ```
2. **Página** (PowerShell 2): `cd frontend; npm run dev`, abrir `http://localhost:5173` y pulsar **Presentación**.
3. **Nodos:** avisa por WhatsApp a augusto, josue y angelo que ejecuten su comando una vez. Comprueba en la diapositiva 6 que aparecen los tres "conectados ahora".
4. **Ensayo de 10 s:** en la diapositiva 7, entrena con augusto y 20 épocas para confirmar que todo responde. Esa corrida queda en el historial de la sesión; no importa.
5. Vuelve a la diapositiva 1, pulsa `F` (pantalla completa) y `T` (cronómetro a cero).
6. Cierra el navegador de otras pestañas, silencia las notificaciones y pon la PC en "no suspender".

**Plan B** (si internet o Tailscale fallan):
- `Ctrl` + `C` al maestro y ejecutar `powershell -ExecutionPolicy Bypass -File scripts\cluster-local.ps1 -Nodos 2 -Web -Muestra data/muestra_prueba.gob`.
- La demo sigue igual, con 2 nodos locales.
- Frase: "Son procesos que solo se hablan por TCP, exactamente como entre casas; lo único que cambia es la latencia."

---

## Diapositiva 1 · Portada (0:20)

**En pantalla:** título, 3 cifras, los 8 integrantes y el docente.

**Qué hacer:** mira al público; al decir "el grupo", señala la lista de integrantes.

**Qué decir:**
> "Buenos días. En nombre del grupo, presento el avance de SACA, un sistema de alerta temprana de contaminación del aire. Predice si el NO₂ de una estación superará el umbral de alerta en la próxima hora, y lo entrena en paralelo en tres niveles: hilos en una PC, varias PCs en red y GPUs. El resultado central: con una GPU remota, por internet, el entrenamiento es unas 18 veces más rápido que en una CPU, con exactamente el mismo modelo."

**Transición:** "Empiezo por el problema."

## Diapositiva 2 · El problema (0:40)

**En pantalla:** mapa, pregunta predictiva, AUC, una **mini gráfica del Experimento 1** (cargador: N agregadoras frente a 1 agregadora) y, abajo, la **tubería de datos** en 6 pasos.

**Qué hacer:** señala el mapa; luego la mini gráfica (la curva verde sube, la naranja se aplana) y recorre la tubería de izquierda a derecha, sin leerla.

**Qué decir:**
> "Usamos los datos reales de la red de monitoreo de Estados Unidos en 2025: casi 17 millones de mediciones horarias de cuatro contaminantes, en 1447 estaciones. La pregunta es de clasificación: ¿el NO₂ superará 53 partes por mil millones en la próxima hora? El modelo es una regresión logística, simple a propósito: alcanza un AUC de 0,995. El interés del curso está en cómo procesamos y entrenamos ese volumen en paralelo. Abajo está la tubería: el cargador concurrente, un worker pool con fan-out y fan-in, bajó la ingesta de 132 a 35 segundos. La gráfica muestra por qué: con una sola agregadora el speedup se aplana cerca de 1, por Amdahl; al particionar las agregadoras llega a 3,8. Al final de la tubería está el entrenamiento distribuido."

**Transición:** "Para eso usamos tres niveles de paralelismo."

## Diapositiva 3 · Tres niveles de paralelismo (0:50)

**En pantalla:** diagrama de 3 niveles, tarjeta de Flynn y, abajo, la tabla **nivel → tecnología → riesgo → solución**.

**Qué hacer:** recorre el diagrama de arriba abajo; al final señala la columna "Riesgo típico" de la tabla.

**Qué decir:**
> "Según la taxonomía de Flynn, el clúster completo es un multicomputador **MIMD de memoria distribuida**: cuatro PCs en cuatro casas que no comparten memoria y solo se envían mensajes por TCP. Dentro de cada PC hay **memoria compartida**: goroutines que trabajan sobre las mismas variables. Y en dos PCs hay GPUs NVIDIA, que son **SIMT**: miles de hilos ejecutando la misma instrucción sobre datos distintos. Cada nivel tiene su riesgo propio, y la tabla de abajo resume cómo lo resolvemos: la condición de carrera con una reducción, la latencia con una barrera síncrona y mensajes mínimos, y la jerarquía de memoria de la GPU con una reducción en memoria compartida."

**Transición:** "Empiezo por la memoria compartida."

## Diapositiva 4 · Memoria compartida (1:00)

**En pantalla:** curvas de speedup, 3 cifras (reducción, mutex, Karp-Flatt) y, abajo, **el código real lado a lado**: mutex (rojo) frente a reducción (verde).

**Qué hacer:** señala las curvas (la verde sube, la naranja y la morada caen); luego señala `mu.Lock()` en el código rojo y `wg.Wait()` en el verde.

**Qué decir:**
> "Entrenar es sumar el gradiente de cada ejemplo. Si varias goroutines suman en el mismo acumulador hay una **condición de carrera**: dos leen el mismo valor, cada una suma su parte y una de las escrituras se pierde. La solución clásica es una sección crítica, con un **mutex** o con **CAS**, una operación atómica. Funciona, pero serializa: con 8 hilos el mutex es 0,46 veces el secuencial, más lento que no paralelizar. La solución correcta es una **reducción**: cada goroutine suma en su variable privada, y al final, tras una barrera, se combinan. Es el `reduction(+)` de OpenMP, y llega a 6,1 de speedup con 8 hilos. En el código se ve la diferencia: a la izquierda, el `Lock` dentro del bucle, por cada ejemplo; a la derecha, cada goroutine trabaja sola y solo se sincroniza una vez, en la barrera."

**Transición:** "Esa misma idea de reducción la llevamos a varias computadoras."

## Diapositiva 5 · Distribución: colectivas (1:00)

**En pantalla:** diagrama de colectivas, 2 cifras y, abajo, la **tabla de los 6 mensajes** del protocolo.

**Qué hacer:** sigue con el puntero los pasos 1 (scatter), 2 (broadcast) y 3 (reduce) y la línea de la barrera; luego señala en la tabla `parametros` y `gradiente_parcial`.

**Qué decir:**
> "El gradiente es una suma sobre ejemplos independientes, así que se puede partir: cada nodo suma sobre su grupo de estaciones y el maestro suma los resultados. Son las **operaciones colectivas** de las semanas 3 y 4. **Scatter**: cada nodo recibe su partición, por hash de la estación. **Broadcast**: en cada época, el maestro envía los 13 pesos. **Reduce**: cada nodo devuelve su suma parcial y el maestro las suma, tras una **barrera síncrona**. Es equivalente a MPI_Bcast más MPI_Allreduce. Lo importante: el modelo es idéntico al secuencial; la pérdida final coincide hasta 10 a la menos 15, que es solo el redondeo por sumar en otro orden. Abajo están los seis mensajes del protocolo: los que importan en cada época son `parametros`, el broadcast, y `gradiente_parcial`, el reduce."

**Transición:** "Así se ve el clúster real."

## Diapositiva 6 · El clúster y CUDA (0:50)

**En pantalla:** tarjetas de los nodos con su **tiempo de cálculo por época medido**, el árbol de reducción CUDA y los 4 pasos del kernel.

**Qué hacer:** señala las tarjetas de los nodos (las cifras en ms) y luego el árbol de reducción.

**Qué decir:**
> "Estas son las PCs conectadas en este momento: augusto y josue con GPU, angelo con CPU, unidas por Tailscale, una red privada sobre internet. Cada tarjeta muestra lo que tarda en calcular una época: la RTX 3060 Ti procesa los 2,5 millones de ejemplos en unos 4 ms; la CPU de angelo tarda 18 ms con solo un tercio. Esa diferencia la van a ver en la demo. En las GPUs, el gradiente se calcula con un kernel CUDA: cada hilo procesa ejemplos; los hilos de un **bloque** acumulan en **memoria compartida** y la reducen en **árbol**, la mitad de hilos en cada nivel, sincronizados con `__syncthreads`, en log₂ n pasos. La CPU suma los resultados de los bloques en orden fijo, así el resultado es determinista. Y antes de usar una GPU, el nodo compara su gradiente con el de la CPU: si no coincide, no la usa."

**Transición:** "Lo muestro en vivo."

## Diapositiva 7 · Demo en vivo (3:00)

Es el panel de control del clúster. **Corrida 1** (unos 50 s):
- **Qué hacer:** marca solo **augusto-pc** → **Entrenar** (200 épocas, sin verificar).
- **Qué decir mientras corre** (2-3 s):
  > "Una sola GPU. Miren 'Tiempo por época': unos 3 ms de cálculo y unos 8 de red. El speedup en vivo ronda 18 frente al secuencial. Abajo, la página va diciendo qué concepto ocurre: broadcast y reduce en cada época, CUDA en el nodo."

**Corrida 2** (unos 50 s):
- **Qué hacer:** marca **augusto-pc + josue** → **Entrenar**.
- **Qué decir:**
  > "Con dos GPUs uno esperaría el doble. Pero es más lento: el cálculo ya era de 3 ms, repartirlo ahorra 1 ms, y ahora la barrera espera la red del nodo con peor conexión. Es Amdahl: la comunicación es la parte serial."

**Corrida 3** (unos 60 s):
- **Qué hacer:** marca **los tres** → **Entrenar**. Señala la gráfica "Cálculo por época de cada nodo".
- **Qué decir:**
  > "Agrego la CPU de angelo. Su línea queda arriba: unos 18 ms por época, contra 1 a 3 de las GPUs. En modo síncrono, la barrera hace que todos esperen al más lento. Es un problema de **balanceo de carga**: los datos están repartidos en partes iguales, pero las máquinas no son iguales."

**Cierre de la demo** (10 s):
- **Qué hacer:** baja a "Corridas de esta sesión".
- **Qué decir:**
  > "Aquí quedan las tres corridas medidas delante de ustedes, con su speedup y su eficiencia."

**Opcional, solo si vas sobrado de tiempo: la caída.** Con los tres y 2000 épocas → Entrenar, y pides a angelo "¡ahora!" para que presione `Ctrl` + `C`.
> "El maestro detecta la caída y aborta limpio; los demás vuelven a la sala. Tolerar la falla sin abortar es un siguiente paso."

**Transición:** "¿Por qué pasa esto? El modelo de costo lo explica."

## Diapositiva 8 · Resultados: Amdahl (1:00)

**Qué hacer:** señala las dos curvas del modelo y luego las barras medidas a la derecha.

**Qué decir:**
> "Cada época cuesta el cálculo dividido entre los nodos más la red, que no se divide. La red es la **fracción serial** de Amdahl. Con la muestra de prueba, 13 ms de cálculo frente a 7 de red: el techo es menos de 3 veces, por muchos nodos que agregue; la curva naranja. Con todos los datos, unos 230 ms de cálculo frente a los mismos 7 de red: el techo sube a unas 30 veces; la curva verde. Es lo que dice **Gustafson**: crecer el problema reduce el peso de la parte serial. A la derecha, lo medido: con la muestra pequeña el 73 % del tiempo era red; con todos los datos, una GPU llega a 18 veces."

**Transición:** "Cierro con lo aprendido."

## Diapositiva 9 · Conclusiones (0:50)

**En pantalla:** **3 tarjetas grandes** numeradas con su cifra (73 %, 18×, 18 → 6), la tarjeta de balanceo junto a la tabla **"teoría del curso → lo que medimos"** y, abajo, 4 tarjetas de próximos pasos con su semana.

**Qué hacer:** señala las tres tarjetas de izquierda a derecha mientras dices cada conclusión; al final, recorre las tarjetas de próximos pasos.

**Qué decir:**
> "Tres conclusiones. Primera: con pocos datos, la red domina y no conviene distribuir. Segunda: con todos los datos, el clúster sí gana, y el modelo es idéntico. Tercera: en un algoritmo síncrono, el nodo más lento marca el ritmo; agregar una CPU baja el speedup de 18 a 6. El siguiente paso es un **reparto proporcional al rendimiento**: medir cuánto rinde cada nodo y darle más datos a la GPU. Después vienen MPI, warp shuffles en CUDA y los núcleos nativos en Pthreads y OpenMP. La tabla de la derecha resume la idea de toda la exposición: cada ley del curso, Amdahl, Gustafson y la barrera síncrona, la medimos en el clúster real."

**Transición:** "Y para cerrar, cómo se conecta esto con el curso."

## Diapositiva 10 · Temas del curso (0:30)

**En pantalla:** 3 contadores (temas hechos, en Go, pendientes), la matriz de temas y el cierre "Gracias · ¿Preguntas?" con los nombres del grupo.

**Qué hacer:** señala los contadores; recorre la tabla con la vista, sin leerla.

**Qué decir:**
> "Arriba, el resumen: la mayoría de los temas están hechos y medidos. Cada tema del curso que aplicamos tiene aquí una evidencia **medida**, no solo implementada: Flynn, speedup y Amdahl, colectivas, mutex frente a CAS, reducción, balanceo de carga y CUDA. Lo pendiente, MPI y los núcleos nativos, ya tiene el contrato de datos listo. Muchas gracias en nombre de todo el grupo; quedo atento a sus preguntas."

---

## Preguntas probables (respuestas cortas que muestran dominio)

**¿Qué es el speedup y la eficiencia? ¿Por qué la eficiencia supera el 100 %?**
S(p) = T₁ / Tₚ y E = S / p. Aquí T₁ es un hilo de CPU y los nodos son GPUs: no es el mismo procesador, así que E > 100 % no es un speedup superlineal "clásico". Mide cuánto gana el sistema frente a la línea base. Para una eficiencia comparable, el Experimento 1b compara goroutines contra un hilo en la misma CPU (E = 76 % con 8).

**¿Cuál es la diferencia entre Amdahl y Gustafson?**
Amdahl fija el tamaño del problema: S ≤ 1 / (f + (1 − f)/p), y el techo es 1/f. Gustafson escala el problema con los recursos: S = p − f(p − 1), y la parte serial pesa menos. Lo vimos: con la muestra de prueba el techo era menos de 3×; con 17 veces más datos, ~30×.

**¿Qué es Karp-Flatt y para qué sirve?**
Es la fracción serial experimental: e = (1/S − 1/p) / (1 − 1/p). Si e se mantiene constante al crecer p, el límite es la parte serial; si crece, hay sobrecarga de sincronización o de comunicación. En el cargador con agregadoras particionadas se mantuvo en ~0,21.

**¿Qué es una condición de carrera? ¿La encontraron?**
Dos hilos hacen leer-modificar-escribir sobre la misma variable sin exclusión mutua, y se pierden actualizaciones. Usamos `go test -race` en todo el código. Además encontramos una **condición de carrera lógica**: estaciones con varios monitores para el mismo gas. El resultado dependía del orden en que llegaban las goroutines. `-race` no la detecta, porque no hay acceso simultáneo a memoria; la resolvimos con una regla determinista (gana el monitor de POC más bajo).

**¿Mutex o CAS? ¿Cuál es mejor?**
El mutex bloquea al hilo que espera. CAS es optimista: lee, calcula y reintenta si otro escribió antes. Con alta contención, como un acumulador por ejemplo, los dos serializan, y CAS además desperdicia reintentos. La respuesta correcta es evitar compartir: una reducción con variables privadas.

**¿Por qué síncrono y no asíncrono?**
El síncrono tiene una barrera por época y da exactamente el modelo secuencial, pero espera al más lento. El asíncrono no espera, aunque usa gradientes calculados con pesos viejos (*stale*) y el resultado ya no es determinista. Es el Experimento 4.

**¿Su reduce es igual a MPI_Allreduce?**
El resultado es el mismo. La implementación es centralizada: el maestro recibe p mensajes y difunde p, O(p) en el maestro. MPI_Allreduce usa algoritmos en anillo o en árbol, O(log p) pasos, sin un cuello de botella central. Con 3 nodos no importa; con muchos, sí.

**¿Cuánto cuesta la red? ¿Latencia o ancho de banda?**
Modelo de Hockney: T = α + β·n. Por época viajan 13 números (~100 bytes), así que β·n es despreciable y domina la **latencia** α (~5-17 ms por internet, medida con `tailscale ping`). Por eso más datos no aumentan el costo de red: solo el cálculo.

**¿Cómo usan la jerarquía de memoria de la GPU?**
Los datos se copian una vez a la memoria global. Cada hilo acumula en registros. El bloque reduce en memoria compartida (`__shared__`, rápida y por bloque) con `__syncthreads` entre niveles. Solo d + 2 números vuelven a la CPU por bloque. Una versión con *warp shuffles* evitaría la memoria compartida en los últimos 32 valores: es el Experimento 8.

**¿Hay divergencia en el kernel?**
Poca. El peso de la clase depende de la etiqueta (`y == 1`), y en la reducción los hilos con `threadIdx.x ≥ s` quedan inactivos en cada nivel. Es divergencia controlada; los shuffles la reducen.

**¿Cómo saben que la GPU calcula bien?**
Antes de entrenar, el nodo compara el gradiente de la GPU con el de la CPU en dos puntos (pesos en cero y al azar), con tolerancia relativa 1e-8. Si no coincide, calcula con la CPU y lo avisa. Además, la pérdida final de cada corrida es idéntica a la secuencial.

**¿Por qué no da exactamente igual, bit a bit?**
La suma de punto flotante no es asociativa: sumar en otro orden cambia el redondeo, del orden de 1e-15. El resto es idéntico.

**¿Balanceo estático o dinámico?**
Hoy es estático: una partición por hash de la estación, igual para todos. Los datos quedan balanceados, pero el tiempo no, porque las máquinas son distintas. El reparto proporcional al rendimiento medido sigue siendo estático pero informado. Uno dinámico, con trozos pequeños y *work stealing*, se adapta durante la ejecución a cambio de más mensajes.

**¿Por qué TCP en Go y no MPI?**
Las PCs están en casas distintas. MPI exige el mismo sistema operativo y la misma versión en todas, más SSH o `smpd` y puertos abiertos en todas. Nuestro TCP solo necesita que los nodos lleguen al maestro. MPI se compara contra este TCP en el Experimento 7.

**Muéstrenme el código de la reducción.**
Diapositiva 4, abajo a la derecha (`internal/logistica/paralelo.go`, `gradienteReduccion`): cada goroutine llama a `GradienteParcial` sobre su bloque de ejemplos y guarda el resultado en su casilla de `parciales`. `wg.Wait()` es la barrera, y después la goroutine principal suma las casillas. No hay ningún lock en el bucle.

**¿Qué viaja exactamente en cada mensaje?**
Diapositiva 5, tabla de abajo. Por época: `parametros` lleva la corrida, la época y 13 números (12 pesos + sesgo); `gradiente_parcial` lleva 12 + 1 sumas de gradiente, la suma de la pérdida, n y los ms de cálculo. Son JSON de una línea por TCP: unos cientos de bytes, por eso domina la latencia y no el ancho de banda.

**¿Qué pasa si un nodo se cae?**
El maestro lo detecta (conexión cerrada o 2 minutos sin gradiente), aborta la corrida limpio y devuelve los demás nodos a la sala. Redistribuir su partición, o el modo asíncrono, es el siguiente paso.
