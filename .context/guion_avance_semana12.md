# Guion de la exposición: avance de SACA (semana 12)

Expone: Angelo Diego Goitia Peves, que maneja el maestro (Fedora) y la página. Duración prevista: **8 minutos** más preguntas. Las diapositivas son las 4 de «Avance de SACA» (artefacto de Claude) y la demo es la página `http://localhost:5173`.

## Cómo usar este guion

- **No lo memorices.** En cada parte tienes la idea que no puede faltar, las cifras y cómo podría sonar. Cuéntalo con tus palabras.
- **Habla como si se lo explicaras a alguien que no ha visto el proyecto.** El profe no necesita saber de machine learning: el modelo solo da una probabilidad entre 0 y 1.
- **Pausa después de cada cifra importante.**
- **Si algo falla en la demo, sigue hablando.** El plan B está al final.

## Cronograma

| Parte | Tema | Tiempo | Acumulado |
|---|---|---:|---:|
| Diapositiva 1 | El problema | 0:50 | 0:50 |
| Diapositiva 2 | Cómo está armado | 1:10 | 2:00 |
| Diapositiva 3 | Dónde entra cada tecnología | 1:30 | 3:30 |
| **Demo** | Panel de tecnologías y clúster en vivo | 3:00 | 6:30 |
| Diapositiva 4 | Dónde estamos y qué sigue | 1:30 | 8:00 |

## Antes de empezar (checklist, 5 minutos antes)

- [ ] Maestro corriendo: `./bin/maestro -web 127.0.0.1:8080 -modo tcp_tailscale -resultado resultados/experimento2_fedora.csv`
- [ ] Página abierta en `localhost:5173`, y en la sala aparecen **angelo, nicolas y josue** (si falta uno, ver plan B).
- [ ] No hay ningún experimento corriendo en tu laptop y la videollamada no está compartiendo desde la misma pestaña pesada.
- [ ] La laptop está en el cargador.
- [ ] Los datos del panel «Las tecnologías en C» están cargados (Pthreads, OpenMP y MPI con punto verde). Si los vas a correr en vivo: `EPOCAS=50 HILOS="1 2 4 8" REPETICIONES=1 sh scripts/nativo.sh exp6` (unos 40 s).

---

## Diapositiva 1 · El problema (0:50)

**Idea que no puede faltar:** SACA responde una pregunta simple: ¿habrá aire contaminado en la próxima hora?

**Qué decir:**
> "SACA es un sistema de alerta de calidad del aire. Toma las mediciones de las estaciones de la agencia ambiental de Estados Unidos, un modelo que aprendió de todo 2025, y nos da la probabilidad de que en la próxima hora se pase el umbral de alerta.
> Lo que evalúa este curso es que son casi 17 millones de filas de datos. Leerlas, limpiarlas y entrenar el modelo es mucho trabajo, así que lo hacemos en paralelo."

**Cifras (di solo una o dos):** 16 972 287 filas · 1 447 estaciones · 10,8 millones de registros después de limpiar.

---

## Diapositiva 2 · Cómo está armado (1:10)

**Idea que no puede faltar:** hay dos partes. A: preparar los datos. B: entrenar el modelo repartido entre varias computadoras.

**Qué decir:**
> "El sistema tiene dos partes. La primera prepara los datos: varios trabajadores leen y limpian los archivos a la vez, otros juntan las filas de cada estación y hora, y todo queda en una base de datos.
> La segunda entrena el modelo. El modelo es una fórmula con números ajustables. Entrenar es ajustarlos muchas veces para que se equivoque menos. Ese cálculo se puede repartir: cada computadora mide el error sobre su parte de los datos, y el maestro suma los resultados y corrige.
> En nuestro clúster hay un maestro y tres nodos: mi laptop con procesador, y dos laptops de mis compañeros con tarjeta gráfica NVIDIA."

**Señala en la diapositiva:** la flecha de ida y vuelta entre el maestro y los nodos.

---

## Diapositiva 3 · Dónde entra cada tecnología (1:30)

**Idea que no puede faltar:** cada tecnología del curso resuelve una parte del recorrido, y las versiones en C se validan contra Go.

**Qué decir (siguiendo las tarjetas de izquierda a derecha):**
> "Esta es la ruta de los datos y dónde entra cada tecnología del curso.
> Pthreads se usa al preparar los datos: varios hilos leen y limpian bloques de los archivos al mismo tiempo.
> MPI es el reparto entre computadoras: un maestro manda a cada una su parte y junta lo que calculan.
> Dentro de cada computadora, el cálculo lo hace OpenMP si tiene procesador, con varios hilos, o CUDA si tiene tarjeta gráfica, con miles de hilos.
> Y Go es el sistema completo que une todo, incluida la red entre las computadoras.
> Lo nuevo de este avance, lo que dice NUEVO, son las versiones reales en C de Pthreads, OpenMP y MPI. Cada una se valida contra Go con los mismos datos: tiene que llegar al mismo error final del modelo, que es 0,132576. Si no coincide, no la medimos."

**Si preguntan por MPI:** en el clúster que corre hoy, el reparto entre PCs usa el protocolo TCP de Go; el MPI real es el programa en C, probado en una sola laptop. Entre PCs queda para la semana del switch.

---

## Demo en vivo (3:00)

### Parte 1 · Panel «Las tecnologías en C» (1:15)

> "Estos son resultados reales, medidos en esta laptop. Cada pestaña es una tecnología. La línea punteada es lo ideal: el doble de hilos, el doble de rápido."

- **Pthreads:** "Aquí comparamos el cargador en Go con el de C. Los dos mejoran al agregar hilos y se aplanan cerca de 8, porque la laptop tiene 8 núcleos reales. El de C es más rápido por sí mismo: con 8 hilos tarda unos 4 segundos y el de Go unos 24."
- **OpenMP:** "Con 8 hilos va unas 5 veces más rápido que con uno. Las tres variantes de reparto salen casi iguales, porque cada muestra cuesta lo mismo de procesar." *(No digas 11×: esa gráfica tiene la base inflada, ver preguntas.)*
- **MPI:** "Dos formas de juntar los resultados. La colectiva escala mejor que mandar mensaje por mensaje al primer proceso, que se vuelve cuello de botella."
- **CUDA:** "Esta pestaña muestra las corridas reales con las GPU del clúster. La comparación de los tres kernels falta medirla en una tarjeta."

### Parte 2 · Clúster en vivo (1:45)

1. **Muestra la sala:** pasa el ratón por el dispositivo de un nodo.
   > "Cada nodo avisa al conectarse qué hardware tiene: procesador, memoria y tarjeta. Así comprobamos que cada máquina trabaja de verdad."
2. **Marca solo `josue` y `nicolas` (GPU), deja «Verificar» sin marcar y pulsa Entrenar.** Dura unos 7 segundos.
   > "Cada época, el maestro manda el modelo, cada GPU calcula su mitad de los datos y devuelve el resultado. Las curvas se dibujan época por época."
3. **Al terminar, señala los números:** unos 35 ms por época; la pérdida llega a 0,1326, la misma que en Go.
4. **Opcional, si hay tiempo:** marca también `angelo` (CPU).
   > "Con tres nodos tarda más que con las dos GPU, porque mi procesador es el más lento y las GPU esperan. Por eso implementamos el reparto según la velocidad de cada PC."

**Dato que sí puedes decir:** una corrida verificada con las dos GPU tardó 7,1 segundos; el mismo entrenamiento en una sola laptop, en secuencial, tardó 50,7 segundos (unas 7 veces más rápido).

---

## Diapositiva 4 · Dónde estamos (1:30)

**Idea que no puede faltar:** funciona de punta a punta; falta medir CUDA en GPU y hacer las pruebas finales.

**Qué decir:**
> "En resumen, el sistema ya funciona de punta a punta: los datos se limpian en paralelo, el entrenamiento se reparte entre varias PCs con procesador y con GPU, y las versiones en C dan el mismo modelo que Go. Además, cada resultado guarda el hardware de la máquina que lo corrió, y el maestro puede repartir los datos según la velocidad de cada PC.
> Falta medir los kernels de CUDA en una tarjeta real, repetir los experimentos finales en las cuatro laptops, probar el reparto por velocidad con las PCs reales, y las pruebas finales en red local con switch.
> La lección que nos llevamos: repartir el trabajo no basta. Hay que repartirlo bien y sincronizar poco."

**Cierra mirando a la cámara:** "Gracias. ¿Preguntas?"

---

## Preguntas probables y cómo contestarlas

**¿Cómo saben que las versiones en C dan el mismo resultado que Go?**
Se entrena una vez en Go con los datos reales y se guarda el error final. Cada programa en C entrena con los mismos datos y compara. Si la diferencia es menor a una millonésima imprime CORRECTO; si no, no se mide. OpenMP y MPI dieron una diferencia de 1e-16. Pthreads se comprueba contando: las mismas 16 972 287 filas y 10 838 667 registros.

**¿Son datos simulados?**
No. Todo sale de los datos reales de la EPA. La única excepción es el Experimento 8, que puede repetir filas reales para probar tamaños de problema más grandes, y lo avisa en su descripción.

**¿Por qué Pthreads sale 30 veces más rápido?**
Porque se compara contra el Go secuencial, y el programa en C es más rápido por sí solo. Para ver lo que aporta agregar hilos, hay que comparar C con C: de 15,8 s con un hilo a 3,9 s con ocho, unas 4 veces.

**¿Por qué el panel de OpenMP marca 11× si dicen 5×?**
La base de esa gráfica se midió en un momento en que la laptop estaba ocupada y quedó inflada. Comparado con su propio 1 hilo (4,4 s contra 0,8 s con 8 hilos) son unas 5,5 veces. Se repite con la laptop tranquila para la entrega.

**¿MPI corre entre varias computadoras?**
Todavía no. Probamos MPI con varios procesos en una sola laptop. En el clúster entre PCs, el reparto usa TCP de Go. MPI entre PCs queda para la semana del switch.

**¿Y CUDA?**
El motor CUDA ya trabaja en el clúster con las dos GPU. Los tres kernels nuevos (atomic, shared y shuffle) están escritos y se probaron en emulación; falta compilarlos y medirlos en una tarjeta real.

**¿Por qué con tres nodos tarda más que con dos?**
Con reparto igual, la computadora más lenta (mi procesador) marca el ritmo y las GPU esperan. El reparto proporcional lo corrige y está implementado en el maestro de una corrida; falta medirlo con las PCs reales y llevarlo a la página.

**¿Qué es la pérdida? ¿Y el F1 bajo (0,015)?**
La pérdida es una cifra que dice qué tan equivocado está el modelo. El F1 es bajo porque solo cerca del 0,02 % de las horas son de alerta; la métrica que importa es el AUC, 0,994.

---

## Plan B (si algo falla en vivo)

| Problema | Qué hacer |
|---|---|
| Un nodo no aparece en la sala | Sigue con los que estén. Pide que repita su comando de conexión. Si sigue "ya hay un nodo registrado", el maestro ya lo reemplaza solo en unos segundos. |
| La corrida se queda en la época 0 | Pulsa **Cancelar corrida**, luego **Quitar** en el nodo trabado, y vuelve a lanzar. |
| Error «ya hay una corrida en curso» | Recarga con F5 y vuelve a pulsar Entrenar. |
| La red está lenta (latencia alta) | Dilo con naturalidad: "la red por VPN pesa más que el cálculo de la GPU" y muestra la corrida ya guardada en el historial. |
| Los paneles de tecnologías están vacíos | Corre `EPOCAS=50 HILOS="1 2 4 8" REPETICIONES=1 sh scripts/nativo.sh exp6` y habla de otra cosa mientras aparecen los puntos (40 s). |
| Cae todo | Enseña `resultados/experimento2_fedora.csv` y las diapositivas. Lo medido ya está guardado. |

## Lo que NO hay que decir

- «CUDA está validado en GPU»: los kernels nuevos solo se probaron en emulación.
- «MPI funciona entre PCs»: solo se probó en una laptop.
- «El speedup es 11×» (OpenMP) ni «30×» como mejora por paralelismo (Pthreads): son efectos de la base de comparación.
- «La eficiencia es 231 %»: no tiene sentido; la página ya muestra «E n/d» en esos casos.
