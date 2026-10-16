# Guion de la exposición: SACA en 10 minutos

Expone: Piero Carlinho Vargas Chota, que maneja la página y el maestro.

**Grupo** (orden por apellido): Cerna Sifuentes, Augusto Nicolas · Goitia Peves, Angelo Diego · Grijalva De La Cruz, Rony · Leon Garcia, Angel Gabriel · Martines Cancho, Josue · Vargas Chota, Piero Carlinho · Yanyachi Perez, Pedro · Zarate Vilchez, Axhell. Docente: Javier Antonio Prudencio Vidal.

## Cómo usar este guion

- **No lo aprendas de memoria.** En cada diapositiva tienes la idea que no puede faltar, las cifras que hay que decir y un ejemplo de cómo sonaría. Léelo en voz alta dos o tres veces y después cuéntalo a tu manera. Si te sale con otras palabras, mejor: así suena tuyo.
- **Habla como si se lo explicaras a un compañero.** Frases cortas. Si te trabas, sigue; nadie tiene tu guion.
- **No leas la pantalla.** Tiene más detalle del que vas a decir (tablas, código, cifras). Señala, di la idea y sigue. Si el profe pregunta, la respuesta ya está ahí.
- **Haz pausas.** Después de cada cifra importante (18 veces, 0,46, el 73 %) quédate callado un segundo. Eso hace que se queden con el número.
- **Mira a la cámara**, no a tus notas, al empezar y al terminar.

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
| 9 | Conclusiones y cierre | 1:20 | 10:00 |

**Si vas atrasado al llegar a la demo** (más de 5:00 en el cronómetro): haz solo dos corridas (una GPU y luego los tres). En la 9 di solo las tres conclusiones y despídete.

---

## Antes de empezar (15 minutos antes)

1. **Maestro en modo servidor** (PowerShell 1):
   ```powershell
   cd "C:\Users\piero\OneDrive\Documentos\Universidad\progra paralela\SACA"
   .\bin\maestro.exe -web 127.0.0.1:8080 -modo tcp_tailscale -resultado resultados\experimento2.csv
   ```
2. **Página** (PowerShell 2): `cd frontend; npm run dev`, abrir `http://localhost:5173` y pulsar **Presentación**.
3. **Nodos:** avisa por WhatsApp a Augusto, Josué y Angelo que ejecuten su comando. Comprueba en la diapositiva 6 que salen los tres "conectados ahora".
4. **Prueba rápida:** en la diapositiva 7, entrena con augusto y 20 épocas para ver que todo responde. Esa corrida queda en el historial; no pasa nada.
5. Vuelve a la diapositiva 1, pulsa `F` (pantalla completa) y `T` (cronómetro a cero).
6. Cierra las otras pestañas, silencia las notificaciones y pon la PC en "no suspender".

**Plan B** (si se cae internet o Tailscale):
- `Ctrl` + `C` al maestro y ejecuta `powershell -ExecutionPolicy Bypass -File scripts\cluster-local.ps1 -Nodos 2 -Web -Muestra data/muestra_prueba.gob`.
- La demo sigue igual, con 2 nodos en tu PC.
- Qué decir: "Se nos cayó la conexión, así que lo corro acá mismo. Son procesos que solo se hablan por TCP, igual que entre casas; lo único que cambia es la latencia."

---

## Diapositiva 1 · Portada (0:20)

**Idea:** qué es SACA y a dónde vamos a llegar.
**Cifra:** 18 veces más rápido con una GPU.
**Qué hacer:** mira a la cámara; al decir "el grupo", señala la lista de integrantes.

> Buenos días. Les voy a presentar el avance de SACA, el proyecto que venimos trabajando con el grupo.
>
> La idea es simple: queremos saber, con una hora de anticipación, si el aire en una estación va a pasar el nivel de alerta de NO₂. Pero lo que nos interesa para el curso es cómo lo entrenamos: en paralelo, con hilos, con varias computadoras conectadas por internet y con GPUs.
>
> Para que vean a dónde vamos: con una sola GPU, en la casa de un compañero, entrenamos unas 18 veces más rápido que con una CPU. Y el modelo sale exactamente igual.

**Pase:** "Primero, ¿con qué datos trabajamos?"

## Diapositiva 2 · El problema (0:40)

**Idea:** son muchos datos, y ya en la carga apareció Amdahl.
**Cifras:** 17 millones de mediciones · de 132 a 35 segundos · se quedaba en 1 y subió a 3,8.
**Qué hacer:** señala el mapa, luego la gráfica chiquita (la verde sube, la naranja se queda plana) y pasa la mano por la tubería de abajo, sin leerla.

> Usamos datos reales de la agencia ambiental de Estados Unidos, todo el 2025. Son casi 17 millones de mediciones, hora por hora, de unas mil cuatrocientas estaciones.
>
> El modelo es una regresión logística, nada del otro mundo, y aun así acierta bastante: el AUC sale 0,995. Pero acá el modelo es lo de menos; lo que nos importa es cómo movemos todo ese volumen.
>
> Miren la tubería de abajo. Solo cargar los datos tomaba 132 segundos; con un cargador concurrente lo bajamos a 35. Y en la gráfica se ve algo que nos pasó: al inicio había una sola goroutine juntando los resultados, y por más hilos que pusiéramos, no pasábamos de 1. Ese era el cuello de botella; Amdahl puro. Cuando repartimos esa parte, subió a 3,8.

**Pase:** "Y para entrenar usamos paralelismo en tres niveles."

## Diapositiva 3 · Tres niveles de paralelismo (0:50)

**Idea:** distribuida, compartida y GPU; cada nivel con su propio problema.
**Qué hacer:** recorre el diagrama de arriba abajo; al final señala la columna "Riesgo típico" de la tabla.

> Si lo vemos con la clasificación de Flynn, el clúster completo es MIMD de memoria distribuida: son cuatro PCs, cada una en una casa distinta, y no comparten nada; solo se mandan mensajes por TCP.
>
> Si bajamos un nivel, dentro de cada PC sí hay memoria compartida: varias goroutines trabajando sobre las mismas variables.
>
> Y abajo del todo están las GPUs, que son SIMT: miles de hilos haciendo la misma instrucción con datos distintos.
>
> Cada nivel trae su propio problema, y los tienen en la tabla: con memoria compartida, las condiciones de carrera; entre PCs, la latencia de la red; y en la GPU, aprovechar bien su memoria. Les cuento cómo resolvimos cada uno.

**Pase:** "Empiezo por la memoria compartida."

## Diapositiva 4 · Memoria compartida (1:00)

**Idea:** el mutex es correcto pero lento; la reducción es la que escala.
**Cifras:** mutex 0,46 con 8 hilos · reducción 6,1.
**Qué hacer:** señala las curvas (la verde sube, las otras caen); luego `mu.Lock()` en el código rojo y `wg.Wait()` en el verde.

> Entrenar, al final, es sumar el gradiente de cada ejemplo. Si pongo varias goroutines a sumar en la misma variable, tengo una condición de carrera: dos leen el mismo valor, cada una suma lo suyo, y una de las dos escrituras se pierde.
>
> Lo primero que a uno se le ocurre es ponerle un mutex, o una operación atómica como CAS. Y sí, el resultado sale bien. Pero miren la curva: con 8 hilos, el mutex va a 0,46. O sea, es más lento que no paralelizar, porque todos hacen cola para entrar.
>
> Lo que sí funciona es la reducción: cada goroutine suma en su propia variable, sin compartir nada, y recién al final se juntan. Es lo mismo que el reduction de OpenMP. Así llegamos a 6,1 con 8 hilos.
>
> En el código se nota la diferencia: a la izquierda, el lock está dentro del bucle, se bloquea en cada ejemplo. A la derecha no hay ningún lock; solo se esperan una vez, al final.

**Pase:** "Esa misma idea la llevamos a varias computadoras."

## Diapositiva 5 · Distribución: colectivas (1:00)

**Idea:** scatter, broadcast y reduce con barrera, y el modelo sale igual al secuencial.
**Cifras:** 13 pesos por época · coincide hasta el decimal 15.
**Qué hacer:** sigue con el puntero los pasos 1, 2 y 3 y la línea de la barrera; luego señala en la tabla `parametros` y `gradiente_parcial`.

> Como el gradiente es una suma, se puede partir: cada nodo suma su pedazo y el maestro junta todo. Son las colectivas que vimos en las semanas 3 y 4.
>
> Primero hay un scatter: a cada nodo le toca un grupo de estaciones. Después, en cada época, un broadcast: el maestro manda los 13 pesos del modelo. Cada nodo calcula y hace el reduce: devuelve su suma y el maestro las junta. Y hay una barrera: la época no avanza hasta que llegan todos. Viene a ser un MPI_Bcast con un Allreduce.
>
> Algo que para nosotros era clave: el modelo sale igual que el secuencial. La pérdida final coincide hasta el decimal quince, y esa diferencia es solo redondeo por sumar en otro orden.
>
> Abajo están los mensajes del protocolo. En cada época solo viajan dos: los pesos de ida y el gradiente de vuelta.

**Pase:** "Y así se ve el clúster de verdad."

## Diapositiva 6 · El clúster y CUDA (0:50)

**Idea:** las máquinas no son iguales (y eso se va a notar en la demo); en la GPU se reduce en árbol.
**Cifras:** GPU unos 4 ms por época · CPU de Angelo 18 ms con un tercio de los datos.
**Qué hacer:** señala las tarjetas de los nodos (los ms) y luego el árbol.

> Estas son las PCs que están conectadas ahorita. La de Augusto y la de Josué tienen GPU; la de Angelo va con CPU. Las une Tailscale, que es una red privada encima de internet.
>
> Fíjense en los tiempos. La 3060 Ti procesa sus dos millones y medio de ejemplos en unos 4 milisegundos. La CPU de Angelo, con un tercio de los datos, se demora 18. Acuérdense de eso, porque lo van a ver en la demo.
>
> En la GPU, el gradiente lo calcula un kernel CUDA. Cada hilo procesa sus ejemplos, y los hilos de un mismo bloque juntan sus resultados en memoria compartida, en forma de árbol: en cada paso trabaja la mitad, con un syncthreads entre paso y paso.
>
> Y un detalle: antes de usar la GPU, el nodo compara su resultado con el de la CPU. Si no cuadra, no la usa.

**Pase:** "Ya, vamos a verlo en vivo."

## Diapositiva 7 · Demo en vivo (3:00)

Es el panel de control del clúster. Mientras corre, no te quedes callado, pero tampoco hables sin parar: di la frase, señala y deja que vean.

**Corrida 1 · una GPU** (unos 50 s)
- **Qué hacer:** marca solo **augusto-pc** → **Entrenar** (200 épocas, sin verificar).

> Arranco con una sola GPU, la de Augusto. Miren el tiempo por época: más o menos 3 milisegundos calculando y unos 8 en la red. El speedup va por 18. Y abajo la página va marcando lo que está pasando: el broadcast, el reduce, CUDA en el nodo.

**Corrida 2 · dos GPUs** (unos 50 s)
- **Qué hacer:** marca **augusto-pc + josue** → **Entrenar**.

> Ahora le sumo la GPU de Josué. Uno pensaría que va a ir el doble de rápido… y no, sale más lento.
>
> ¿Por qué? Porque el cálculo ya era de 3 milisegundos. Partirlo en dos me ahorra uno, pero ahora tengo que esperar la red del que tiene peor conexión. La red es la parte que no se paraleliza. Eso es Amdahl.

**Corrida 3 · los tres** (unos 60 s)
- **Qué hacer:** marca **los tres** → **Entrenar**. Señala la gráfica "Cálculo por época de cada nodo".

> Y ahora los tres, con la CPU de Angelo. Miren esta gráfica: su línea está allá arriba, en 18 milisegundos, y las GPUs en 1 o 3. Como hay barrera, todos se quedan esperándolo.
>
> Los datos están repartidos parejo, pero las máquinas no son iguales. Eso es un problema de balanceo de carga.

**Para cerrar la demo** (10 s)
- **Qué hacer:** baja a "Corridas de esta sesión".

> Y acá quedan las tres corridas que acaban de ver, con su speedup y su eficiencia.

**Solo si te sobra tiempo: la caída.** Marca los tres con 2000 épocas → Entrenar y dile a Angelo "¡ahora!" para que presione `Ctrl` + `C`.

> El maestro se da cuenta de que se cayó, corta la corrida y los demás vuelven a la sala. Que siga entrenando sin él es algo que todavía nos falta.

**Pase:** "¿Y por qué pasa todo esto? Para eso armamos un modelo de costo."

## Diapositiva 8 · Resultados: Amdahl (1:00)

**Idea:** la red es la parte serial; con más datos pesa menos (Gustafson).
**Cifras:** muestra 13 ms contra 7 de red, techo menor a 3 · completo 230 ms, techo de unas 30 · 73 % era red · una GPU, 18 veces.
**Qué hacer:** señala la curva naranja, luego la verde, luego las barras de la derecha.

> La cuenta es sencilla. Cada época cuesta el cálculo, que se reparte entre los nodos, más la red, que no se reparte. Esa red es la parte serial de Amdahl.
>
> Con la muestra de prueba teníamos 13 milisegundos de cálculo contra 7 de red. Con eso, ponga los nodos que ponga, no paso de 3 veces; es la curva naranja.
>
> Con todos los datos, el cálculo sube a 230 y la red sigue en 7, y el techo se va a unas 30 veces; es la verde. Eso es Gustafson: si el problema crece, la parte serial pesa cada vez menos.
>
> Y a la derecha está lo que medimos: con la muestra chica, casi tres cuartos del tiempo se iba en la red. Con los datos completos, una GPU llega a 18.

**Pase:** "Para cerrar, tres cosas que nos llevamos."

## Diapositiva 9 · Conclusiones y cierre (1:20)

**Idea:** las tres conclusiones, lo que sigue y la despedida.
**Cifras:** 73 % · 18 veces · de 18 a 6.
**Qué hacer:** señala las tres tarjetas de izquierda a derecha mientras dices cada conclusión; después pasa por las de próximos pasos. Para despedirte, mira a la cámara.

> Primero: con pocos datos no conviene distribuir, porque la red se come todo.
>
> Segundo: con los datos completos el clúster sí gana, y el modelo sale idéntico.
>
> Y tercero: en un esquema síncrono manda el más lento. Meter una CPU entre las GPUs nos bajó el speedup de 18 a 6.
>
> Por eso lo que sigue es repartir según lo que rinde cada máquina: medir cuánto aguanta cada una y darle más datos a la GPU. Después vienen MPI, los warp shuffles en CUDA y las versiones en Pthreads y OpenMP.

**Despedida** (con calma, mirando a la cámara):

> Si me quedo con una sola idea, es esta: lo que vimos en el curso, Amdahl, Gustafson, la barrera, la reducción, no lo dejamos en la teoría; lo medimos en un clúster de verdad, con nuestras propias máquinas.
>
> Eso sería todo de parte del grupo. Muchas gracias, y si tienen alguna pregunta, con gusto.

**Después:** deja la diapositiva 9 en pantalla. Si preguntan por algo puntual, vuelve con `←` a la diapositiva que corresponda.

---

## Preguntas probables

Responde corto: primero la respuesta directa en una frase, y después, si hace falta, el porqué. Si no sabes algo, dilo tal cual ("eso no lo medimos, pero lo que esperaríamos es…"); queda mucho mejor que inventar.

**¿Qué es el speedup y la eficiencia? ¿Por qué la eficiencia pasa del 100 %?**
> El speedup es el tiempo secuencial entre el tiempo en paralelo, y la eficiencia es eso dividido entre el número de nodos. Pasa del 100 % porque comparamos contra un hilo de CPU, y los nodos son GPUs; no es el mismo procesador, así que no es un superlineal de verdad. Si quiere una eficiencia justa, está el experimento con goroutines en la misma CPU: ahí da 76 % con 8 hilos.

**¿Cuál es la diferencia entre Amdahl y Gustafson?**
> Amdahl deja el problema fijo: la parte serial pone un techo, uno entre f. Gustafson dice que si crece el problema junto con los recursos, la parte serial pesa menos. Nosotros lo vimos: con la muestra, el techo era menos de 3; con 17 veces más datos, unas 30.

**¿Qué es Karp-Flatt y para qué sirve?**
> Es la fracción serial pero medida, sacada del speedup real. Si se queda constante cuando agrego hilos, el límite es la parte serial; si sube, es sobrecarga de sincronización o de comunicación. En el cargador se mantuvo en 0,21, más o menos.

**¿Qué es una condición de carrera? ¿Encontraron alguna?**
> Es cuando dos hilos leen, modifican y escriben la misma variable a la vez y se pierden actualizaciones. Pasamos `go test -race` por todo el código. Pero encontramos una más tramposa, una lógica: había estaciones con varios monitores para el mismo gas, y el resultado dependía de qué goroutine llegaba primero. El detector no la ve porque no hay acceso a memoria al mismo tiempo. La arreglamos con una regla fija: gana el monitor de número más bajo.

**¿Mutex o CAS? ¿Cuál es mejor?**
> El mutex bloquea al que espera; CAS intenta y, si alguien escribió antes, reintenta. Cuando todos pelean por la misma variable, los dos terminan haciendo cola, y CAS encima gasta reintentos. Lo mejor es no compartir: una reducción con variables privadas.

**¿Por qué síncrono y no asíncrono?**
> Porque el síncrono da exactamente el mismo modelo que el secuencial. El precio es que espera al más lento. El asíncrono no espera, pero trabaja con pesos un poco viejos y ya no da siempre el mismo resultado. Eso lo vamos a comparar en el experimento 4.

**¿Su reduce es igual a MPI_Allreduce?**
> El resultado sí, la forma no. El nuestro es centralizado: el maestro recibe de todos y les reenvía a todos. MPI usa anillo o árbol, en log de p pasos, sin un cuello de botella en el centro. Con tres nodos no se nota; con muchos, sí.

**¿Cuánto cuesta la red? ¿Latencia o ancho de banda?**
> Latencia. Por época mandamos 13 números, unos cien bytes; eso no es nada de ancho de banda. Lo que pesa es la latencia de internet, entre 5 y 17 milisegundos. Por eso, aunque metamos más datos, la red cuesta lo mismo; solo crece el cálculo.

**¿Cómo usan la memoria de la GPU?**
> Los datos se copian una vez a la memoria global. Cada hilo acumula en sus registros, y el bloque reduce en memoria compartida, que es la rápida, con un syncthreads entre nivel y nivel. A la CPU solo vuelven unos pocos números por bloque. Con warp shuffles nos ahorraríamos la memoria compartida al final; está en los próximos pasos.

**¿Hay divergencia en el kernel?**
> Un poco: el peso depende de si la etiqueta es 1 o 0, y en el árbol, en cada nivel, la mitad de los hilos se queda sin hacer nada. Es controlada; con shuffles bajaría.

**¿Cómo saben que la GPU calcula bien?**
> Antes de entrenar, el nodo calcula el gradiente en la GPU y en la CPU, en dos puntos distintos, y los compara. Si no coinciden, usa la CPU y lo avisa. Y además, la pérdida final de cada corrida sale igual a la secuencial.

**¿Por qué no da exactamente igual, bit por bit?**
> Porque sumar decimales en otro orden cambia el redondeo; la suma en punto flotante no es asociativa. La diferencia está en el decimal quince.

**¿El balanceo es estático o dinámico?**
> Hoy es estático: cada nodo recibe la misma cantidad de estaciones. Los datos quedan parejos, pero el tiempo no, porque las máquinas son distintas. Lo siguiente es repartir según lo que rinde cada una. Uno dinámico, con robo de trabajo, se adaptaría en el camino, pero mandaría más mensajes.

**¿Por qué TCP en Go y no MPI?**
> Porque las PCs están en casas distintas. MPI pide el mismo sistema y la misma versión en todas, más SSH y puertos abiertos. Con nuestro TCP basta con que cada nodo llegue al maestro. Igual vamos a comparar contra MPI en el experimento 7.

**Muéstrenme el código de la reducción.**
> Vuelve a la diapositiva 4, al código verde. "Cada goroutine calcula sobre su bloque de ejemplos y guarda el resultado en su propia casilla. Este `wg.Wait()` es la barrera, y recién después se suman las casillas. Dentro del bucle no hay ningún lock." (Está en `internal/logistica/paralelo.go`, `gradienteReduccion`.)

**¿Qué viaja exactamente en cada mensaje?**
> Vuelve a la diapositiva 5, a la tabla. "Por época van dos mensajes: de ida, los 13 pesos; de vuelta, las sumas del gradiente, la pérdida, cuántos ejemplos procesó y cuánto se demoró. Es JSON de una línea por TCP, unos cientos de bytes."

**¿Qué pasa si un nodo se cae?**
> El maestro se da cuenta, porque se cierra la conexión o pasan dos minutos sin respuesta, corta la corrida y devuelve a los demás a la sala. Repartir su parte entre los que quedan es lo que sigue.
