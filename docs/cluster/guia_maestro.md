# Guía del maestro (quien coordina el clúster)

**Esta guía es solo para quien maneja el maestro**: la PC que reparte el trabajo, junta los resultados y comparte pantalla en la demo. Hoy es Piero (Ryzen 5 3600, Windows 11). Los demás integrantes siguen la guía de nodo que les toca ([README.md](README.md)).

Como maestro tienes dos tareas:

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

**1.1 Comprobar que la tienes** y anotar su huella:

```powershell
Test-Path data\muestra.gob
(Get-FileHash data\muestra.gob -Algorithm SHA256).Hash.Substring(0,12).ToLower()
```

**Debes ver** `True` y `5e90db4b1158`.

**Si ves `False`**, genérala (necesita los CSV de la EPA y MongoDB, unos 3 minutos con la descarga ya hecha):

```powershell
powershell -ExecutionPolicy Bypass -File scripts\pipeline.ps1 -MongoLocal
```

Si regeneras la muestra, la huella **cambia**: anuncia la nueva en el grupo, porque las guías de los nodos dicen `5e90db4b1158`.

**1.2 Subirla a Drive:**
1. Sube `data\muestra.gob` a la carpeta del grupo en Google Drive.
2. Clic derecho → **Compartir** → **Acceso general: Cualquier persona con el enlace** → **Copiar enlace**.
3. Publica en el grupo: el enlace y la huella `5e90db4b1158`.

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
T(3 nodos) = 897ms, 4.48 ms por época: cálculo del más lento 4.25 ms + red y espera 0.24 ms (5% del tiempo es comunicación)
...
verificación: pérdida final distribuida 0.125720142, secuencial 0.125720142, diferencia 2.6e-15 → OK, mismo modelo
```

Cierra las 3 ventanas de los trabajadores cuando termine.

Si cambiaste el código, actualiza también el Experimento 2 local (unos 2 minutos):

```powershell
powershell -ExecutionPolicy Bypass -File scripts\experimento2.ps1
```

- [ ] Ensayo local con `OK, mismo modelo`.

---

## Paso 6. Ensayo general (todos por videollamada)

**Qué vas a hacer:** la primera vez con todas las PCs conectadas. Unos 30 minutos.

**6.1 Antes de la llamada** debes tener en el grupo el mensaje del paso "Enviar tus datos al grupo" de cada nodo. Completa el inventario de [README.md](README.md) y revisa que:
- todos tengan la huella `5e90db4b1158`;
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
| `muestra data/muestra.gob, huella 5e90db4b1158` | Tu huella: los nodos deben tener la misma |
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

> No vuelvas a correr `scripts\experimento2.ps1` después de esto: **borra** `resultados\experimento2.csv` y lo regenera solo con las corridas locales. Si necesitas rehacer las corridas locales, primero guarda una copia del CSV.

- [ ] Experimento 2 por Tailscale medido y publicado.

---

## Paso 8. El día de la presentación

**1 hora antes:**
- [ ] `git push` hecho y "hagan `git pull`" avisado en el grupo.
- [ ] `tailscale status`: tu PC conectada.
- [ ] Ensayo rápido en tu PC: `scripts\cluster-local.ps1 -Nodos 2` con `OK`.
- [ ] Suspensión desactivada (**Configuración → Sistema → Inicio/apagado → Nunca**), notificaciones silenciadas y OneDrive en pausa.
- [ ] PowerShell con letra grande (`Ctrl` + rueda del mouse) y la carpeta del proyecto abierta.

**5 minutos antes:** los nodos ejecutan su comando de "Unirte al clúster" y quedan reintentando. Tú tienes escrito, **sin ejecutar**:

```powershell
.\bin\maestro.exe -nodos <N> -modo tcp_tailscale -maquina "demo: <N> PCs por Tailscale" -resultado resultados\experimento2.csv
```

**Durante la demo** (detalle en la sección 4 de [../guia_presentacion_cluster.md](../guia_presentacion_cluster.md)):
1. Comparte tu pantalla y ejecuta el comando: los nodos entran solos.
2. Señala las líneas `registrado desde 100.x... motor go/cuda`.
3. Deja correr el entrenamiento y lee una línea de época.
4. Pide al nodo con GPU que comparta su pantalla unos segundos (`motor cuda validado`).
5. Muestra la tabla final y `OK, mismo modelo`.
6. La caída: relanza con `.\bin\maestro.exe -nodos <N> -epocas 2000 -verificar=false`, los nodos repiten su comando (flecha ↑ y Enter) y el integrante acordado presiona `Ctrl` + `C` **en su terminal**. Tú no tocas tu ventana.

**Plan B** si la red falla: ejecuta `powershell -ExecutionPolicy Bypass -File scripts\cluster-local.ps1 -Nodos 4 -Ventanas`, que hace el clúster completo en tu PC, y explica que son procesos que solo se hablan por TCP, igual que entre casas.

---

## Problemas frecuentes (lado del maestro)

| Lo que ves | Qué hacer |
|---|---|
| `no se pudo escuchar en :9000: ... Only one usage of each socket address` | Ya hay otro maestro abierto. Ciérralo o pega `Get-Process maestro -ErrorAction SilentlyContinue \| Stop-Process` |
| Un nodo nunca aparece | Que ese nodo revise su guía: Tailscale conectado, invitación aceptada, IP y `:9000` bien escritos. Desde tu lado: `tailscale ping <IP-del-nodo>`. Si falla, revisa la regla del paso 4 |
| `rechazado, su muestra tiene otra huella (...)` | Ese nodo tiene otro `muestra.gob`. Que lo descargue de nuevo (paso "Copiar la muestra" de su guía) |
| `nodos activos 3/4` y no avanza | Pusiste más `-nodos` de los presentes. `Ctrl` + `C` y relanza con el número correcto |
| `registro de X rechazado: todas las particiones ya están asignadas` | Entró un nodo de más: no afecta la corrida actual |
| `entrenamiento abortado: ... no enviaron su gradiente en 2m0s` | Un nodo se colgó (suspensión, wifi). Que lo reinicie y repite la corrida |
| `entrenamiento abortado: ... el nodo X se cayó` sin que se lo pidieras | Ese nodo cerró su ventana o perdió internet. Relanza la corrida |
| `resultados\experimento2.csv tiene otras columnas` | El CSV es de otro formato. Usa otro nombre en `-resultado` |
| `verificación: ... → DISTINTO: revisar` | No debería pasar. Guarda toda la salida y revisa que todos los nodos tengan el mismo commit |
