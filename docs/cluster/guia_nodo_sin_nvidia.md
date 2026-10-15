# Guía del nodo SIN GPU NVIDIA (tu PC calcula con el procesador)

**Esta guía es para ti si tu computadora NO tiene tarjeta gráfica NVIDIA.** Eso incluye las PCs con tarjeta **AMD (Radeon)**, con gráficos **Intel** o integrados, y **todas las Mac**. Tu PC aportará su **procesador (CPU)** al clúster.

> ¿Tienes NVIDIA? Entonces tu guía es [guia_nodo_con_nvidia.md](guia_nodo_con_nvidia.md). ¿No sabes? Revisa la sección 1 de [README.md](README.md).

No necesitas saber programar. Copia y pega cada comando **tal cual** y compara lo que ves con lo que dice "Debes ver". Lo único que cambias son los textos entre `<` `>`, por ejemplo `<tu-nombre>`, y siempre se explica qué poner.

- **Tiempo total:** unos 45 minutos la primera vez. Después, 2 minutos.
- **Windows:** sigue los pasos 0 a 8.
- **Linux o Mac:** sigue la sección [Linux y macOS](#linux-y-macos) al final; los pasos tienen los mismos nombres.

## Antes de empezar: qué necesitas

- [ ] Internet estable. Para la demo, mejor por cable.
- [ ] Unos 2 GB libres en el disco `C:`.
- [ ] Una cuenta de Google, Microsoft o GitHub, para Tailscale.
- [ ] Del maestro, por el grupo:
  - la invitación de Tailscale (un enlace o un correo);
  - el enlace de Drive con el archivo `muestra.gob`;
  - el día de la demo, su dirección, del tipo `100.101.102.103`.

---

## Paso 0. Abrir PowerShell (la terminal)

**Qué vas a hacer:** abrir la ventana donde se pegan los comandos.

1. Presiona la tecla **Windows**, escribe `PowerShell` y presiona **Enter**. Se abre una ventana azul o negra que termina en algo como `PS C:\Users\ana>`.
2. Para **pegar** un comando: cópialo de esta guía (`Ctrl` + `C`) y en PowerShell presiona `Ctrl` + `V` o **clic derecho**. Luego **Enter**.
3. Un comando **terminó** cuando vuelve a aparecer la línea `PS C:\...>` y puedes escribir de nuevo.
4. Si se te pide "abrir una PowerShell **nueva**", cierra todas las ventanas de PowerShell y abre otra como en el punto 1.

- [ ] Sé abrir PowerShell y pegar un comando.

---

## Paso 1. Instalar Git, Go y Tailscale

**Qué vas a hacer:** instalar los 3 programas que necesita tu nodo. Unos 15 minutos.

| Programa | Para qué sirve | Dónde descargarlo | Qué elegir en el instalador |
|---|---|---|---|
| **Git** | Descargar el proyecto | https://git-scm.com/download/win, botón "Click here to download" | **Next** en todas las pantallas, sin cambiar nada |
| **Go** | Compilar el programa del nodo | https://go.dev/dl/, el primer archivo que termina en **`windows-amd64.msi`** | **Next** en todo |
| **Tailscale** | Conectarte con el maestro por internet | https://tailscale.com/download/windows | **Install**. Al terminar, aparece un ícono en la barra de tareas, junto al reloj |

Cuando termines las 3 instalaciones, **abre una PowerShell nueva** (paso 0, punto 4) y pega:

```powershell
git --version; go version; tailscale version
```

**Debes ver** 3 respuestas, por ejemplo:

```
git version 2.49.0.windows.1
go version go1.27.1 windows/amd64
1.88.1
  tailscale commit: ...
```

**Si en cambio ves** `... no se reconoce como nombre de un cmdlet` en alguno: no abriste una PowerShell nueva, o ese programa no se instaló. Cierra todas las PowerShell, abre otra y repite. Si sigue igual, vuelve a instalar ese programa.

> La versión de Go debe ser **1.27 o mayor**. Si es menor, Go descargará la correcta solo la primera vez que compiles (tarda un poco más, pero funciona).

- [ ] Los 3 programas responden.

---

## Paso 2. Conectarte a la red del grupo (Tailscale)

**Qué vas a hacer:** entrar a la red privada para poder llegar a la PC del maestro.

1. Haz clic en el ícono de **Tailscale** junto al reloj (si no lo ves, haz clic en la flechita `^`) → **Log in**. Se abre el navegador: entra con tu cuenta de Google, Microsoft o GitHub.
2. Abre la **invitación del maestro** (el enlace o el correo que te pasó) y acéptala con **la misma cuenta**. Así aparecerá la PC del maestro en tu red.
3. En PowerShell, pega (cambia `<IP-del-maestro>` por la IP `100.x.y.z` que te pasó el maestro, por ejemplo `100.101.102.103`):

```powershell
tailscale ip -4
tailscale ping <IP-del-maestro>
```

**Debes ver:**

```
100.70.12.34                                            ← tu IP: anótala para el paso 6
pong from piero-pc (100.101.102.103) via DERP(mia) in 85ms
pong from piero-pc (100.101.102.103) via 190.12.34.56:41641 in 32ms
```

- `via DERP(...)`: la conexión pasa por un servidor intermedio. Funciona, solo que más lento.
- `via 190...:41641`: la conexión es directa, la mejor.
- Las dos sirven.

**Si en cambio ves:**
- `no matching peer`: todavía no aceptaste la invitación del maestro, o la aceptaste con otra cuenta. Repite el punto 2.
- Que no responde: puede que la PC del maestro esté apagada en ese momento. Pruébalo el día del ensayo general.

- [ ] Anoté mi IP de Tailscale.
- [ ] El ping al maestro responde (o lo pruebo en el ensayo general).

---

## Paso 3. Descargar el proyecto

**Qué vas a hacer:** copiar el proyecto en la carpeta `C:\saca`. Usa **exactamente** esa ruta: no lo pongas en Escritorio, Documentos ni OneDrive, porque las rutas con espacios o sincronizadas dan problemas.

```powershell
cd C:\
git clone https://github.com/angelogp27/SACA.git saca
cd C:\saca
git log -1 --oneline
```

**Debes ver** al final una línea como `9fce811 scripts: mensajes que...`: es la versión del proyecto que tienes.

**Si en cambio ves:**
- Una ventana que pide iniciar sesión en GitHub: el repositorio es privado. Inicia sesión con tu cuenta de GitHub y, si dice que no tienes acceso, pídele al maestro que te agregue.
- `destination path 'saca' already exists`: ya lo descargaste antes. Pega `cd C:\saca; git pull`.

**Las veces siguientes** (antes de cada ensayo y de la presentación) solo actualiza:

```powershell
cd C:\saca
git pull
```

- [ ] Tengo el proyecto en `C:\saca`.

---

## Paso 4. Copiar la muestra

**Qué vas a hacer:** poner el archivo de datos `muestra.gob` (unos 20 MB) dentro del proyecto. No viene con el proyecto: lo comparte el maestro por Drive.

1. Abre el enlace de Drive del maestro y descarga `muestra.gob`. Queda en tu carpeta **Descargas**.
2. Pega:

```powershell
Copy-Item "$env:USERPROFILE\Downloads\muestra.gob" C:\saca\data\
(Get-FileHash C:\saca\data\muestra.gob -Algorithm SHA256).Hash.Substring(0,12).ToLower()
```

**Debes ver:**

```
5e90db4b1158
```

Esa es la **huella**. Debe ser **exactamente** esta, o la que el maestro anuncie si regeneró los datos.

**Si en cambio ves:**
- `No se encuentra la ruta de acceso ... muestra.gob`: el archivo se descargó con otro nombre (por ejemplo `muestra (1).gob`) o en otra carpeta. Renómbralo a `muestra.gob` y repite.
- Otra huella: el archivo está incompleto o es de otra versión. Bórralo, descárgalo de nuevo y repite.

- [ ] Mi huella es `5e90db4b1158`.

---

## Paso 5. Ensayo individual

**Qué vas a hacer:** comprobar que tu PC puede hacer todo. El script simula en tu PC un clúster completo (un maestro y 2 nodos). Unos 2-3 minutos la primera vez, porque compila.

```powershell
cd C:\saca
powershell -ExecutionPolicy Bypass -File scripts\cluster-local.ps1 -Nodos 2
```

Si Windows muestra un aviso de **Firewall de Windows Defender** para `maestro.exe`, haz clic en **Permitir** (o **Cancelar**: para este ensayo da igual).

**Debes ver** muchas líneas. Las importantes, casi al final, son:

```
nodo           part  motor dispositivo                           ejemplos  cálculo/ép.  desde
local1            0  go    CPU ANA-LAPTOP, 4 goroutines             73413      2.42 ms  [::1]:60727
local2            1  go    CPU ANA-LAPTOP, 4 goroutines             73302      2.80 ms  [::1]:60728
...
verificación: pérdida final distribuida 0.125720142, secuencial 0.125720142, diferencia 2.5e-15 → OK, mismo modelo
```

Lo que importa es que diga **`OK, mismo modelo`**. Los números de tiempo cambian según la PC.

**Si en cambio ves:**
- `falta data/muestra.gob`: repite el paso 4.
- `go: ... dial tcp ... timeout` mientras compila: tu internet se cortó al descargar componentes. Repite el comando.
- Cualquier otro error: copia las **últimas 15 líneas** y pégalas en el grupo.

- [ ] Mi ensayo dice `OK, mismo modelo`.

---

## Paso 6. Enviar tus datos al grupo

**Qué vas a hacer:** darle al maestro lo que necesita para el inventario y para saber que estás listo.

```powershell
cd C:\saca
"CPU: " + (Get-CimInstance Win32_Processor).Name.Trim()
"RAM: " + [math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory/1GB) + " GB"
"GPU: " + ((Get-CimInstance Win32_VideoController).Name -join ", ")
"Sistema: " + (Get-CimInstance Win32_OperatingSystem).Caption
"Huella: " + (Get-FileHash data\muestra.gob -Algorithm SHA256).Hash.Substring(0,12).ToLower()
"Version: " + (git rev-parse --short HEAD)
try { "IP Tailscale: " + (tailscale ip -4) } catch { "IP Tailscale: (Tailscale no instalado)" }
```

**Debes ver** algo como:

```
CPU: Intel(R) Core(TM) i5-1135G7 @ 2.40GHz
RAM: 8 GB
GPU: Intel(R) Iris(R) Xe Graphics
Sistema: Microsoft Windows 11 Home
Huella: 5e90db4b1158
Version: 9fce811
IP Tailscale: 100.70.12.34
```

Copia esas líneas y pégalas en el grupo con este mensaje:

```
NODO SIN NVIDIA — <tu-nombre>
<pega aquí las 7 líneas>
Ensayo individual: OK, mismo modelo
```

- [ ] Envié mis datos al grupo.

---

## Paso 7. Unirte al clúster (ensayo general y presentación)

**Qué vas a hacer:** conectar tu PC al maestro de verdad. Se hace igual en el ensayo general y en la presentación.

**7.1 Antes de empezar** (5 minutos antes): actualiza el proyecto y verifica Tailscale.

```powershell
cd C:\saca
git pull
tailscale ping <IP-del-maestro>
```

**7.2 Conéctate.** Cambia `<IP-del-maestro>` por la IP que el maestro escribe en el grupo y `<tu-nombre>` por tu nombre en minúsculas, sin tildes ni espacios (por ejemplo `ana`). **No borres el `:9000`**.

```powershell
cd C:\saca
powershell -ExecutionPolicy Bypass -File scripts\nodo.ps1 -Maestro <IP-del-maestro>:9000 -Id <tu-nombre>
```

Ejemplo ya completo: `powershell -ExecutionPolicy Bypass -File scripts\nodo.ps1 -Maestro 100.101.102.103:9000 -Id ana`

**Debes ver**, en este orden:

```
== 1/4 programa del trabajador
   compilado bin/trabajador.exe (commit 9fce811)
== 2/4 muestra de datos
   huella 5e90db4b1158 (debe ser igual a la que muestra el maestro)
== 3/4 motor de cálculo
   sin GPU NVIDIA: se calcula con goroutines en la CPU
== 4/4 conexión con el maestro 100.101.102.103:9000
   OK
== nodo ana en marcha (Ctrl+C para salir)
... muestra data/muestra.gob cargada, huella 5e90db4b1158
... motor auto: no hay bin/gradiente_cuda.exe, se calcula con goroutines
... registrado como ana: partición 1 de 4 → 38975 de 146715 ejemplos de entrenamiento (26.6%)
... listo: calcula con el motor go (CPU ANA-LAPTOP, 8 goroutines); esperando los parámetros del maestro
... época 0: gradiente enviado (2.13 ms de cálculo)
... época 50: gradiente enviado (2.40 ms de cálculo)
...
... entrenamiento finalizado: 200 épocas, pérdida final 0.125720; este nodo calculó 2.42 ms por época en promedio
```

- Si el maestro **todavía no arrancó**, verás `AVISO: no se pudo conectar todavía` y luego `maestro ... no disponible todavía ... reintentando...` cada pocos segundos. **Es normal: espera.** Se conecta solo cuando el maestro arranca.
- Mientras entrena, **no cierres la ventana** y no suspendas la PC.

**7.3 Otra corrida.** Cuando aparece `entrenamiento finalizado`, tu nodo termina. Si el maestro pide **"otra corrida"**, presiona la **flecha ↑** del teclado (vuelve el último comando) y **Enter**.

**7.4 La caída (solo si el maestro te lo pide).** En la presentación se muestra qué pasa si un nodo se cae. Solo el integrante al que el maestro se lo pida presiona **`Ctrl` + `C`** en su ventana cuando se lo indiquen.

- [ ] Llegué a `entrenamiento finalizado` en el ensayo general.

---

## Paso 8. Al terminar

- No hay que desinstalar nada. Tailscale puede quedar encendido o puedes desconectarlo desde su ícono (**Disconnect**).
- Si el maestro vuelve a convocar, empieza directamente en el paso 7.

---

## Problemas frecuentes

| Lo que ves | Qué hacer |
|---|---|
| `no se puede cargar ... porque la ejecución de scripts está deshabilitada` | Escribiste `scripts\nodo.ps1` sin el comienzo. Copia el comando completo, que empieza con `powershell -ExecutionPolicy Bypass -File` |
| `... no se reconoce como nombre de un cmdlet` (git, go o tailscale) | Abre una PowerShell **nueva** (paso 0). Si sigue, reinstala ese programa (paso 1) |
| `maestro ... no disponible todavía ... reintentando...` durante mucho rato | ¿El maestro ya arrancó? Pregunta en el grupo. ¿Está conectado tu Tailscale? (`tailscale status` debe listar la PC del maestro). ¿Escribiste bien la IP y el `:9000`? |
| `el maestro rechazó el registro: muestra_distinta` | Tu `muestra.gob` no es la del maestro. Repite el paso 4 y compara la huella |
| `el maestro rechazó el registro: registro_rechazado: todas las particiones ya están asignadas` | Entraste después de que el maestro completó su lista de nodos. Avisa en el grupo para que te cuente en la siguiente corrida |
| `registro_rechazado: ya hay un nodo registrado con ese worker_id` | Otra persona usa tu mismo nombre. Cambia el `-Id`, por ejemplo `ana2` |
| `el maestro pidió la baja: maestro_cerrando` o `el maestro cerró la conexión` | El maestro terminó o se reinició. Espera su indicación y repite el paso 7.2 |
| `fatal: not a git repository` | No estás en la carpeta del proyecto. Pega `cd C:\saca` primero |
| `huella` distinta de `5e90db4b1158` en el paso 1 o 2 del script | Paso 4 |
| La ventana se cerró sola | Abre PowerShell y repite el paso 7.2 |

Si nada de esto aplica, copia las **últimas 15 líneas** de tu ventana y pégalas en el grupo.

---

## Linux y macOS

Los mismos pasos, con los comandos de tu sistema. La terminal es la aplicación **Terminal**. Para pegar se usa `Ctrl` + `Shift` + `V` en Linux y `Cmd` + `V` en Mac.

### Paso 1 (Linux/Mac). Instalar Git, Go y Tailscale

**Ubuntu / Debian:**

```sh
sudo apt update && sudo apt install -y git curl
sudo snap install go --classic
curl -fsSL https://tailscale.com/install.sh | sh
```

**Mac:**

1. Git: en Terminal, `xcode-select --install` → **Instalar**.
2. Go: https://go.dev/dl/, el archivo **`darwin-arm64.pkg`** (Mac con chip M1/M2/M3/M4) o **`darwin-amd64.pkg`** (Mac con Intel). Abrir e instalar.
3. Tailscale: https://tailscale.com/download/mac o desde la App Store. Queda un ícono en la barra de menú, arriba a la derecha.

Verifica en una Terminal **nueva**: `git --version; go version`. Debe mostrar Go 1.27 o mayor.

### Paso 2 (Linux/Mac). Tailscale

- Linux: `sudo tailscale up`, que muestra un enlace: ábrelo e inicia sesión. Después acepta la invitación del maestro y comprueba con `tailscale ip -4` y `tailscale ping <IP-del-maestro>`.
- Mac: ícono de Tailscale → **Log in**, y acepta la invitación del maestro. Tu IP aparece al hacer clic en el ícono (`This device: ... 100.x.y.z`).

### Paso 3 (Linux/Mac). Descargar el proyecto

```sh
cd ~
git clone https://github.com/angelogp27/SACA.git saca
cd ~/saca
git log -1 --oneline
```

Las veces siguientes: `cd ~/saca && git pull`.

### Paso 4 (Linux/Mac). Copiar la muestra

```sh
cp ~/Downloads/muestra.gob ~/saca/data/
shasum -a 256 ~/saca/data/muestra.gob | cut -c1-12
```

(Si tu carpeta de descargas se llama `~/Descargas`, usa esa.) **Debes ver** `5e90db4b1158`.

### Paso 5 (Linux/Mac). Ensayo individual

```sh
cd ~/saca
bash scripts/cluster-local.sh 2
```

**Debes ver** al final `OK, mismo modelo`.

### Paso 6 (Linux/Mac). Enviar tus datos

Linux:

```sh
cd ~/saca
lscpu | grep "Model name"; free -h | grep Mem; lspci | grep -iE "vga|3d"
echo "Huella: $(shasum -a 256 data/muestra.gob | cut -c1-12)"; echo "Version: $(git rev-parse --short HEAD)"; tailscale ip -4
```

Mac: menú  → **Acerca de esta Mac** (chip y memoria), más:

```sh
cd ~/saca
echo "Huella: $(shasum -a 256 data/muestra.gob | cut -c1-12)"; echo "Version: $(git rev-parse --short HEAD)"
```

Envía todo al grupo con la plantilla del paso 6, junto con tu IP de Tailscale.

### Paso 7 (Linux/Mac). Unirte al clúster

```sh
cd ~/saca
git pull
bash scripts/nodo.sh <IP-del-maestro>:9000 <tu-nombre>
```

Ejemplo: `bash scripts/nodo.sh 100.101.102.103:9000 luis`. La salida es la misma que en Windows. Para otra corrida: **flecha ↑** y **Enter**. Para la caída (solo si te lo piden): `Ctrl` + `C`.
