# Guía del maestro (quien coordina el clúster)

**Esta guía es solo para quien maneja el maestro**: la PC que reparte el trabajo, junta los resultados y comparte pantalla en la demo. Hoy es Piero (Ryzen 5 3600, Windows 11). Los demás integrantes siguen la guía de nodo que les toca ([README.md](README.md)).

Como maestro tienes dos tareas (y desde el paso 8, todo se puede manejar desde la página):

1. **Preparar** todo para que los nodos puedan seguir su guía sin ayuda: el código publicado, la muestra en Drive, la invitación de Tailscale y el puerto abierto.
2. **Conducir** el ensayo general y la demo: lanzar el maestro, leer la salida y guardar los resultados.

Tiempo: unos 30 minutos de preparación (una vez) y 30 minutos de ensayo general.

## Antes de empezar: qué necesitas

- [ ] El proyecto en tu PC, con Go 1.27 o mayor (`go version`).
- [ ] `data\muestra.gob`, o los datos y MongoDB para generarla (paso 1).
- [ ] Permisos de administrador (para el firewall, paso 4).
- [ ] Una carpeta de Google Drive del grupo.
- [ ] Ideal: la PC conectada por cable y al cargador si es laptop.

Todos los comandos se pegan en **PowerShell**, desde la carpeta del proyecto. Abre PowerShell y entra a la carpeta (ajusta la ruta si tu proyecto está en otra):

```powershell
cd "C:\Users\piero\OneDrive\Documentos\Universidad\progra paralela\SACA"
```

---

## Paso 1. Preparar y compartir la muestra

**Qué vas a hacer:** dejar `data\muestra.gob` en Drive para que todos los nodos tengan exactamente los mismos datos.

Hay dos muestras:

| Archivo | Contenido | Para qué |
|---|---|---|
| `data\muestra.gob` | **Todos** los registros con NO2 etiquetables: 3 342 599 ejemplos (2 499 636 de entrenamiento), 337 MB, huella `c2c8f8832a30` | **El clúster.** Es la que usan por defecto el maestro, los nodos y los scripts. Con ella el cálculo por época (~214 ms en un hilo) supera con creces a la red (~7 ms): es lo que justifica el clúster |
| `data\muestra_prueba.gob` | 200 mil registros sorteados (146 715 de entrenamiento), 20 MB, huella `5e90db4b1158` | Pruebas rápidas y Experimento 1b. En el clúster se usa solo pasándola explícitamente (`-muestra` en el maestro, `-Muestra` en `nodo.ps1`) |

**1.1 Comprobar que la tienes** y anotar su huella:

```powershell
Test-Path data\muestra.gob
(Get-FileHash data\muestra.gob -Algorithm SHA256).Hash.Substring(0,12).ToLower()
```

**Debes ver** `True` y `c2c8f8832a30`.

**Si ves `False`**, genérala. Necesita MongoDB con los datos ya cargados (`scripts\pipeline.ps1 -MongoLocal`, si no lo hiciste) y tarda **unos 10 minutos**: arma la muestra con todos los registros, la guarda y entrena una vez en secuencial (eso da el T(1)):

```powershell
powershell -ExecutionPolicy Bypass -File scripts\mongo-local.ps1
go run .\entrenamiento\cmd\secuencial -muestra 0 -cache data\muestra.gob
```

**Debes ver**, entre otras líneas, `muestra: 3342599 ejemplos etiquetados desde MongoDB` y al final `T(1) entrenamiento secuencial (200 epocas): ~43s`.

Si regeneras la muestra, la huella **puede cambiar**: anuncia la nueva en el grupo, porque las guías de los nodos dicen `c2c8f8832a30`.

**1.2 Subirla a Drive:**
1. Sube `data\muestra.gob` (337 MB) a la carpeta del grupo en Google Drive. Si ya habías subido la de prueba, **bórrala o renómbrala** en Drive para que nadie la confunda.
2. Clic derecho → **Compartir** → **Acceso general: Cualquier persona con el enlace** → **Copiar enlace**.
3. Publica en el grupo: el enlace y la huella `c2c8f8832a30`, y pide a quien tenga la anterior que la **reemplace**.

- [ ] Muestra en Drive, con el enlace y la huella publicados.

---

## Paso 2. Publicar el código

**Qué vas a hacer:** subir a GitHub la versión que los nodos van a descargar. Los nodos usan **lo que está en GitHub**, no lo que tienes en tu PC.

```powershell
git status --short
git push
git log -1 --oneline
```

**Debes ver** que `git status` no lista nada pendiente y, al final, la versión publicada (por ejemplo `9fce811 ...`). Publica esa versión en el grupo: todos deben tener la misma.

Si el repositorio es **privado**, agrega a cada integrante en GitHub: **Settings → Collaborators → Add people** (requiere ser dueño del repositorio o pedírselo a quien lo sea).

> Antes de **cada** ensayo y de la presentación: `git push` y avisar en el grupo "hagan `git pull`".

- [ ] Código publicado; todos tienen acceso.

---

## Paso 3. Tailscale: la red del grupo

**Qué vas a hacer:** que los nodos puedan llegar a tu PC por internet. Usarás la opción de **compartir tu PC**: cada integrante usa su propia cuenta y solo ve tu máquina. Basta con eso, porque solo los nodos se conectan al maestro, nunca al revés.

1. Instala Tailscale (https://tailscale.com/download/windows) e inicia sesión con tu cuenta.
2. Anota tu IP:

   ```powershell
   tailscale ip -4
   ```

   **Debes ver** algo como `100.101.102.103`. Esta es **la IP del maestro** que usarán todos.
3. Entra a https://login.tailscale.com/admin/machines. En la fila de tu PC:
   - menú **`…`** → **Disable key expiry**, para que tu PC no se desconecte sola antes de la demo;
   - menú **`…`** → **Share…** → **Copy share link** (o escribe el correo de cada integrante). Envía el enlace al grupo: cada uno lo abre y lo acepta con su cuenta.
4. Cuando cada nodo acepte, podrá hacer `tailscale ping <tu-IP>`. Puedes comprobarlo desde tu lado con `tailscale status`.

- [ ] IP del maestro anotada y publicada en el grupo.
- [ ] Enlace para compartir enviado.

---

## Paso 4. Abrir el puerto 9000 en el firewall (una sola vez)

**Qué vas a hacer:** permitir que los nodos se conecten a tu programa maestro.

1. Tecla **Windows** → escribe `PowerShell` → **clic derecho** → **Ejecutar como administrador** → **Sí**.
2. Pega:

```powershell
New-NetFirewallRule -DisplayName "SACA maestro 9000" -Direction Inbound -Protocol TCP -LocalPort 9000 -Action Allow
Get-NetFirewallRule -DisplayName "SACA maestro 9000" | Select-Object DisplayName, Enabled, Direction, Action
```

**Debes ver** al final:

```
DisplayName       Enabled Direction Action
-----------       ------- --------- ------
SACA maestro 9000    True   Inbound  Allow
```

3. Cierra esa PowerShell de administrador. El resto se hace en una PowerShell normal.

La primera vez que ejecutes `maestro.exe`, Windows puede mostrar un aviso del firewall: marca **las dos casillas** (redes privadas y públicas) y **Permitir acceso**.

- [ ] Regla del puerto 9000 creada.

---

## Paso 5. Ensayo en tu PC

**Qué vas a hacer:** comprobar que el maestro funciona antes de involucrar a los demás. Abre el clúster completo en tu PC, con cada nodo en su propia ventana:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\cluster-local.ps1 -Nodos 3 -Ventanas
```

**Debes ver** que se abren 3 ventanas (`trabajador local1`, `local2` y `local3`) y que la tuya termina con:

```
T(3 nodos) = 10.554s, 52.77 ms por época: cálculo del más lento 51.75 ms + red y espera 1.02 ms (2% del tiempo es comunicación)
...
verificación: pérdida final distribuida 0.132576204, secuencial 0.132576204, diferencia 8.6e-16 → OK, mismo modelo
secuencial en esta PC: 42.403s → speedup S(3) = 4.02
```

Tarda unos 2 minutos (la verificación secuencial final dura ~43 s) y usa ~6 GB de RAM: cada proceso carga la muestra (~1,5 GB). Cierra las 3 ventanas de los trabajadores cuando termine.

Si cambiaste el código, actualiza también el Experimento 2 local (unos 2 minutos):

```powershell
powershell -ExecutionPolicy Bypass -File scripts\experimento2.ps1
```

- [ ] Ensayo local con `OK, mismo modelo`.

---

## Paso 6. Ensayo general (todos por videollamada)

**Qué vas a hacer:** la primera vez con todas las PCs conectadas. Unos 30 minutos.

**6.1 Antes de la llamada** debes tener en el grupo el mensaje del paso "Enviar tus datos al grupo" de cada nodo. Completa el inventario de [README.md](README.md) y revisa que:
- todos tengan la huella `c2c8f8832a30`;
- todos tengan la misma `Version` que tu `git log -1 --oneline`;
- todos tengan `Ensayo individual: OK`.

**6.2 Al empezar la llamada**, escribe en el grupo:

```
Hagan:  cd C:\saca ; git pull
IP del maestro: <tu-IP>:9000
Cuando diga "¡ya!", ejecuten el comando del paso "Unirte al clúster" de su guía.
```

**6.3 Compila y lanza el maestro.** Cambia `<N>` por la cantidad de nodos conectados en la llamada, sin contarte a ti:

```powershell
go build -o bin\maestro.exe .\entrenamiento\cmd\maestro
.\bin\maestro.exe -nodos <N> -modo tcp_tailscale -maquina "ensayo: <N> PCs por Tailscale" -resultado resultados\experimento2.csv
```

Di "¡ya!" a los nodos.

**6.4 Qué debes ver y qué significa cada línea:**

| Línea | Significado |
|---|---|
| `muestra data/muestra.gob, huella c2c8f8832a30` | Tu huella: los nodos deben tener la misma |
| `los nodos se conectan con: -maestro 100.101.102.103:9000` | La línea con `100.` es tu IP de Tailscale |
| `nodo ana registrado desde 100.70.12.34:52011 → partición 1/4, motor go (CPU ANA-LAPTOP, 8 goroutines)` | Se conectó un nodo: su IP, la parte de los datos que le tocó y con qué calcula |
| `nodos activos 3/4 [...]` | Faltan nodos: espera |
| `clúster completo con 4 nodos: empieza el entrenamiento` | Están todos; empieza solo |
| `época  20: pérdida=0.325505 \|   48.12 ms (cálculo del más lento 2.97 ms, red y espera 45.15 ms)` | Cada 20 épocas: cuánto tardó la época, cuánto fue cálculo y cuánto red |
| La tabla `nodo part motor dispositivo ...` | Resumen por nodo |
| `T(4 nodos) = ... (95% del tiempo es comunicación)` | El dato clave para la diapositiva de resultados |
| `verificación: ... → OK, mismo modelo` | El resultado distribuido es idéntico al de una sola PC |
| `resultados agregados a resultados\experimento2.csv` | La corrida quedó guardada |

**6.5 Si un nodo no llega** en 2 minutos: presiona `Ctrl` + `C` y relanza con `-nodos` uno menos. Mientras, esa persona revisa los problemas frecuentes de su guía.

**6.6 Anota las latencias**, una por nodo (cambia `<IP-del-nodo>` por la que envió cada uno):

```powershell
tailscale ping <IP-del-nodo>
```

Completa la tabla del ensayo en [../guia_presentacion_cluster.md](../guia_presentacion_cluster.md) (diapositiva 5).

**6.7 Prueba la caída de un nodo** (la misma que se mostrará en la demo):

```powershell
.\bin\maestro.exe -nodos <N> -epocas 2000 -verificar=false
```

Cuando empiece, pide a un nodo sin GPU que presione `Ctrl` + `C` **en SU terminal**. **Tú no presionas nada en tu ventana**: si lo haces, detienes al maestro y verás `maestro detenido con Ctrl+C en ESTA ventana (no fue un nodo)`. **Debes ver:**

```
entrenamiento abortado: época 312: el nodo ana se cayó (baja: cierre_normal); el modo síncrono no puede seguir sin su partición
```

- [ ] Corrida completa con todos los nodos y `OK, mismo modelo`.
- [ ] Latencias anotadas.
- [ ] Caída probada.

---

## Paso 7. Medir el Experimento 2 por Tailscale

**Qué vas a hacer:** repetir el entrenamiento con 1, 2 y todos los nodos, 3 veces cada uno, para la tabla de resultados. Hazlo al final del ensayo general, unos 10 minutos.

En cada corrida:
1. Escribe en el grupo **quiénes** entran. Por ejemplo, "corrida con 2 nodos: ana y luis". Los demás no se conectan en esa corrida.
2. Lanza el maestro con el `-nodos` que toque:

   ```powershell
   .\bin\maestro.exe -nodos <N> -modo tcp_tailscale -maquina "<N> PCs por Tailscale" -resultado resultados\experimento2.csv
   ```

3. Los nodos indicados presionan **flecha ↑** y **Enter** en su ventana.

Orden sugerido: N = 1, 2 y todos, y repetir esa ronda 3 veces.

Al final, regenera la tabla y publícala:

```powershell
go run .\experimentos\cmd\analisis -entrada resultados\experimento2.csv -json frontend\public\datos\experimento2.json -md resultados\experimento2.md -nombre "Experimento 2" -descripcion "Entrenamiento distribuido maestro-trabajador por TCP" -paralelo "tcp_local,tcp_tailscale" "-serial="
Get-Content resultados\experimento2.md -Encoding UTF8
git add resultados\experimento2.csv resultados\experimento2.md frontend\public\datos\experimento2.json
git commit -m "resultados: Experimento 2 por Tailscale (ensayo general)"
git push
```

Copia los números de la serie `tcp_tailscale` a la diapositiva 5.

> `scripts\experimento2.ps1` (corridas locales) **conserva** las filas `tcp_tailscale` del CSV: solo reemplaza sus propias filas `tcp_local`. Con 8 nodos locales y la muestra completa necesita ~13 GB de RAM; en una PC de 16 GB usa `-Nodos 1,2,4`.
>
> Los resultados con la muestra de prueba (incluido el primer ensayo real por Tailscale, del 15/10) quedaron en `resultados\experimento2_prueba.*`.

- [ ] Experimento 2 por Tailscale medido y publicado.

---

## Paso 8. Modo servidor: controlar el clúster desde la página

**Qué vas a hacer:** dejar el maestro encendido como servidor y manejar todo desde la página. Los nodos se conectan **una sola vez** y quedan en una **sala de espera**. Desde la página eliges con casillas qué nodos usar, pulsas **Entrenar** y ves en vivo las épocas, la pérdida, el cálculo frente a la red de cada nodo, el speedup y el concepto del curso que está ocurriendo. Al terminar, los nodos vuelven a la sala y puedes lanzar otra combinación sin pedirle nada a nadie.

**8.1 Arranca el maestro en modo servidor** (PowerShell 1):

```powershell
cd "C:\Users\piero\OneDrive\Documentos\Universidad\progra paralela\SACA"
go build -o bin\maestro.exe .\entrenamiento\cmd\maestro
.\bin\maestro.exe -web 127.0.0.1:8080 -modo tcp_tailscale -resultado resultados\experimento2.csv
```

**Debes ver:**

```
muestra data/muestra.gob, huella c2c8f8832a30 (los nodos deben mostrar la misma)
maestro en modo servidor: nodos en [::]:9000 (sala de hasta 16)
  los nodos se conectan con: -maestro 100.67.177.8:9000
T(1) de referencia: una época en secuencial (1 hilo) en esta PC tarda ~210 ms
API de la página en http://127.0.0.1:8080 (abre la página con: cd frontend; npm run dev)
```

**8.2 Abre la página** (PowerShell 2):

```powershell
cd "C:\Users\piero\OneDrive\Documentos\Universidad\progra paralela\SACA\frontend"
npm install
npm run dev
```

Abre en el navegador la dirección que muestra (`http://localhost:5173`). Pulsa **Presentación** para el recorrido de 10 diapositivas (la demo es la diapositiva 6), o quédate en **Tablero**: el panel "Clúster en vivo" está abajo.

**8.3 Los nodos se conectan** con su comando de siempre, **una sola vez** (el de "Unirte al clúster" de su guía). Ya no se cierran al terminar cada corrida: quedan en la sala y, si se corta la conexión o reinicias el maestro, vuelven solos.

**8.4 En la página:**
- **Sala:** aparece cada nodo con GPU o CPU, su tarjeta, su IP y si está libre. Marca las casillas de los que quieras usar.
- **Épocas** (200 por defecto) y **Verificar contra el secuencial**: añade ~40 s al final para comprobar que el modelo es idéntico. En la demo conviene dejarlo apagado; ya está medido en la diapositiva de resultados.
- **Entrenar con N nodos:** lanza la corrida. Mientras corre verás la barra de época, 5 indicadores (tiempo por época, % de comunicación, speedup en vivo, throughput, pérdida), 4 gráficas que crecen época a época y la franja con el tema del curso que está ocurriendo.
- **Cancelar corrida:** la detiene; los nodos vuelven a la sala.
- **Quitar** (en cada nodo): despide a ese nodo; se cierra en su PC.
- **Terminar sesión:** despide a todos (pide confirmación).
- **Corridas de esta sesión:** al final del panel, una barra por corrida terminada con su tiempo por época (cálculo y red), el speedup y la eficiencia.

**8.5 Guardar los resultados por combinación.** Con `-resultado resultados\experimento2.csv`, cada corrida terminada se guarda con su **combinación exacta de nodos** (p. ej. `tcp_tailscale:augusto-pc+josue`), así que puedes probar todas las combinaciones que quieras sin que se mezclen:
1. **Primera corrida de la sesión con "Verificar contra el secuencial" marcado** (una basta): guarda el T(1) secuencial que necesita la tabla.
2. Lanza cada combinación que quieras comparar, idealmente **3 veces** (la tabla usa la mediana).
3. Genera la tabla (y los datos que muestra la página en "Resultados"):

   ```powershell
   powershell -ExecutionPolicy Bypass -File scripts\experimento2-tabla.ps1
   ```

   Recarga la página (`F5`) para ver las barras nuevas. Las corridas anteriores (agrupadas solo por cantidad de nodos) quedaron archivadas en `resultados\experimento2_anterior.*`; la página las muestra mientras no haya corridas nuevas.

**8.6 Para terminar:** `Ctrl` + `C` en la PowerShell del maestro. Despide a todos los nodos, que esperarán reintentando hasta que lo vuelvas a arrancar o cierren su ventana.

**Ensayo en tu PC (sin compañeros):**

```powershell
powershell -ExecutionPolicy Bypass -File scripts\cluster-local.ps1 -Nodos 2 -Web -Muestra data/muestra_prueba.gob
```

Abre la página en otra PowerShell (`cd frontend; npm run dev`). Usa la muestra de prueba para no cargar la RAM con 3 procesos de la muestra completa.

---

## Paso 9. El día de la presentación

**1 hora antes:**
- [ ] `git push` hecho y "hagan `git pull`" avisado en el grupo.
- [ ] `tailscale status`: tu PC conectada.
- [ ] Ensayo rápido en tu PC: `scripts\cluster-local.ps1 -Nodos 2 -Web -Muestra data/muestra_prueba.gob` y la página respondiendo.
- [ ] Suspensión desactivada (**Configuración → Sistema → Inicio/apagado → Nunca**), notificaciones silenciadas y OneDrive en pausa.
- [ ] Navegador con la página en **Presentación**, en pantalla completa (`F11`).

**15 minutos antes:**
1. Arranca el maestro en modo servidor (paso 8.1) y la página (paso 8.2).
2. Pide a los nodos que ejecuten su comando **una vez**. Comprueba en la página (diapositiva 5 o 6) que aparecen todos en la sala.

**Durante la demo** (diapositiva 6; guion en la sección 4 de [../guia_presentacion_cluster.md](../guia_presentacion_cluster.md)):
1. Marca **solo augusto** (1 GPU) → **Entrenar**. Narra las gráficas mientras corre (~2 s de entrenamiento).
2. Marca **augusto + josue** (2 GPU) → **Entrenar**. Es más lento: la red de un nodo más pesa más que el cálculo que se reparte.
3. Marca los **tres** (+ angelo, CPU) → **Entrenar**. En "Cálculo por época de cada nodo" se ve a angelo arriba: **marca el ritmo**.
4. Muestra "Corridas de esta sesión": las 3 barras comparadas, medidas delante del docente.
5. La caída: lanza una corrida con **2000 épocas** y pide a angelo que presione `Ctrl` + `C` **en su terminal**. La corrida aparece como abortada ("el nodo angelo se cayó"); el resto vuelve a la sala. Angelo relanza su comando y reaparece.

**Plan B** si la red falla: `Ctrl` + `C` al maestro y ejecuta `powershell -ExecutionPolicy Bypass -File scripts\cluster-local.ps1 -Nodos 2 -Web -Muestra data/muestra_prueba.gob`. La página sigue igual y los nodos locales aparecen en la sala. Explica que son procesos que solo se hablan por TCP, igual que entre casas.

**Modo de una corrida** (como en los pasos 6 y 7), por si la página fallara: `.\bin\maestro.exe -nodos <N> -modo tcp_tailscale -resultado resultados\experimento2.csv`; los nodos se conectan igual y la salida se lee en la terminal.

---

## Problemas frecuentes (lado del maestro)

| Lo que ves | Qué hacer |
|---|---|
| La página dice **"maestro sin conexión"** | El maestro no está en modo servidor o no está corriendo: paso 8.1. Si sí corre, revisa que la página se abrió con `npm run dev` (el proxy `/api` solo existe ahí y en `npm run preview`) |
| `no se pudo abrir la API en 127.0.0.1:8080` | Ya hay otro maestro en modo servidor. Ciérralo (`Get-Process maestro \| Stop-Process`) |
| El botón **Entrenar** está gris | No hay nodos libres marcados, o hay una corrida en curso |
| `ya hay una corrida en curso` en la página | Espera a que termine o pulsa **Cancelar corrida** |
| `no se pudo escuchar en :9000: ... Only one usage of each socket address` | Ya hay otro maestro abierto. Ciérralo o pega `Get-Process maestro -ErrorAction SilentlyContinue \| Stop-Process` |
| Un nodo nunca aparece | Que ese nodo revise su guía: Tailscale conectado, invitación aceptada, IP y `:9000` bien escritos. Desde tu lado: `tailscale ping <IP-del-nodo>`. Si falla, revisa la regla del paso 4 |
| `rechazado, su muestra tiene otra huella (...)` | Ese nodo tiene otro `muestra.gob`. Que lo descargue de nuevo (paso "Copiar la muestra" de su guía) |
| `nodos en la sala 3/4` y no avanza (modo de una corrida) | Pusiste más `-nodos` de los presentes. `Ctrl` + `C` y relanza con el número correcto |
| `registro de X rechazado: todas las particiones ya están asignadas` | Entró un nodo de más: no afecta la corrida actual |
| `entrenamiento abortado: ... no enviaron su gradiente en 2m0s` | Un nodo se colgó (suspensión, wifi). Que lo reinicie y repite la corrida |
| `entrenamiento abortado: ... el nodo X se cayó` sin que se lo pidieras | Ese nodo cerró su ventana o perdió internet. Relanza la corrida |
| `resultados\experimento2.csv tiene otras columnas` | El CSV es de otro formato. Usa otro nombre en `-resultado` |
| `verificación: ... → DISTINTO: revisar` | No debería pasar. Guarda toda la salida y revisa que todos los nodos tengan el mismo commit |
