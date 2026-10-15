# Guía del nodo CON GPU NVIDIA (tu PC calcula con la tarjeta gráfica)

**Esta guía es para ti si tu computadora tiene una tarjeta gráfica NVIDIA** (GeForce, RTX, GTX, Quadro). Tu PC aportará su **GPU con CUDA** al clúster.

> ¿No tienes NVIDIA (tienes AMD, Intel o una Mac)? Entonces tu guía es [guia_nodo_sin_nvidia.md](guia_nodo_sin_nvidia.md). ¿No sabes? Revisa la sección 1 de [README.md](README.md).

No necesitas saber programar. Copia y pega cada comando **tal cual** y compara lo que ves con lo que dice "Debes ver". Lo único que cambias son los textos entre `<` `>`, y siempre se explica qué poner.

- **Tiempo total:** entre 1 y 2 horas la primera vez, porque las herramientas de NVIDIA y Microsoft pesan varios GB. Después, 2 minutos.
- **Windows:** sigue los pasos 0 a 11.
- **Linux:** sigue la sección [Linux](#linux) al final.

> **Tranquilidad:** si la parte de CUDA (pasos 5 a 7) no te sale en unos 30 minutos, **no pasa nada**. Tu PC funcionará igual en el clúster, calculando con el procesador. Sigue con el paso 8 y avisa al maestro.

## Antes de empezar: qué necesitas

- [ ] Internet estable. Para la demo, mejor por cable.
- [ ] Unos **15 GB libres** en el disco `C:`, por las herramientas de NVIDIA y Microsoft.
- [ ] Permisos de **administrador** en tu PC, para instalar.
- [ ] Una cuenta de Google, Microsoft o GitHub, para Tailscale.
- [ ] Del maestro, por el grupo:
  - la invitación de Tailscale;
  - el enlace de Drive con `muestra.gob`;
  - el día de la demo, su dirección `100.x.y.z`.
- [ ] Si es una laptop: **conectada al cargador** durante todo el proceso.

---

## Paso 0. Abrir PowerShell (la terminal)

**Qué vas a hacer:** abrir la ventana donde se pegan los comandos.

1. Presiona la tecla **Windows**, escribe `PowerShell` y presiona **Enter**. Se abre una ventana que termina en algo como `PS C:\Users\luis>`.
2. Para **pegar** un comando: cópialo (`Ctrl` + `C`) y en PowerShell presiona `Ctrl` + `V` o **clic derecho**. Luego **Enter**.
3. Un comando **terminó** cuando vuelve a aparecer `PS C:\...>`.
4. Si se te pide "abrir una PowerShell **nueva**", cierra todas las ventanas de PowerShell y abre otra.

- [ ] Sé abrir PowerShell y pegar un comando.

---

## Paso 1. Instalar Git, Go y Tailscale

| Programa | Para qué sirve | Dónde descargarlo | Qué elegir en el instalador |
|---|---|---|---|
| **Git** | Descargar el proyecto | https://git-scm.com/download/win | **Next** en todo |
| **Go** | Compilar el programa del nodo | https://go.dev/dl/, el primer archivo que termina en **`windows-amd64.msi`** | **Next** en todo |
| **Tailscale** | Conectarte con el maestro | https://tailscale.com/download/windows | **Install** |

Abre una PowerShell **nueva** y pega:

```powershell
git --version; go version; tailscale version
```

**Debes ver** 3 respuestas (versión de Git, `go version go1.27...` o mayor, y la versión de Tailscale).

**Si en cambio ves** `... no se reconoce como nombre de un cmdlet`: abre una PowerShell nueva; si sigue, reinstala ese programa.

- [ ] Los 3 programas responden.

---

## Paso 2. Conectarte a la red del grupo (Tailscale)

1. Ícono de **Tailscale** junto al reloj (o en la flechita `^`) → **Log in**, e inicia sesión en el navegador.
2. Abre la **invitación del maestro** y acéptala con **la misma cuenta**.
3. En PowerShell (cambia `<IP-del-maestro>` por la IP `100.x.y.z` que te pasó el maestro):

```powershell
tailscale ip -4
tailscale ping <IP-del-maestro>
```

**Debes ver** tu IP (por ejemplo `100.70.12.35`; anótala) y líneas `pong from ... in 40ms`. Las respuestas `via DERP(...)` también sirven.

**Si en cambio ves** `no matching peer`: no aceptaste la invitación o usaste otra cuenta.

- [ ] Anoté mi IP de Tailscale.

---

## Paso 3. Descargar el proyecto

Usa **exactamente** la carpeta `C:\saca`, sin espacios y fuera de OneDrive:

```powershell
cd C:\
git clone https://github.com/angelogp27/SACA.git saca
cd C:\saca
git log -1 --oneline
```

**Debes ver** una línea como `9fce811 scripts: mensajes que...`.

**Si en cambio ves:**
- Que pide iniciar sesión en GitHub: inicia sesión; si dice que no tienes acceso, pide al maestro que te agregue.
- `already exists`: pega `cd C:\saca; git pull`.

Las veces siguientes: `cd C:\saca; git pull`.

- [ ] Tengo el proyecto en `C:\saca`.

---

## Paso 4. Copiar la muestra

1. Descarga `muestra.gob` del enlace de Drive del maestro (queda en **Descargas**).
2. Pega:

```powershell
Copy-Item "$env:USERPROFILE\Downloads\muestra.gob" C:\saca\data\
(Get-FileHash C:\saca\data\muestra.gob -Algorithm SHA256).Hash.Substring(0,12).ToLower()
```

**Debes ver** `5e90db4b1158` (o la huella que anuncie el maestro).

**Si en cambio ves:**
- No encuentra el archivo: revisa si se descargó como `muestra (1).gob`. Renómbralo.
- Otra huella: descárgalo de nuevo.

- [ ] Mi huella es `5e90db4b1158`.

---

## Paso 5. Revisar la tarjeta NVIDIA y su driver

**Qué vas a hacer:** confirmar que Windows reconoce la tarjeta y que el driver está al día.

```powershell
nvidia-smi
```

**Debes ver** una tabla con el nombre de tu tarjeta. Arriba a la derecha dice `CUDA Version: 12.x` o mayor:

```
+-----------------------------------------------------------------------------------------+
| NVIDIA-SMI 560.94                 Driver Version: 560.94         CUDA Version: 12.6     |
|-------------------------------+------------------------+-------------------------------+
|   0  NVIDIA GeForce RTX 3060 ...
```

**Si en cambio ves:**
- `nvidia-smi no se reconoce`, o `CUDA Version` menor que 12: instala el driver más reciente desde https://www.nvidia.com/es-la/drivers/ (elige tu modelo) o desde la app **NVIDIA App** / **GeForce Experience**. Reinicia la PC y repite.

- [ ] `nvidia-smi` muestra mi tarjeta y `CUDA Version` 12 o mayor.

---

## Paso 6. Instalar las herramientas para compilar CUDA

**Qué vas a hacer:** instalar dos programas, **en este orden**. Primero Microsoft y después NVIDIA, porque el instalador de NVIDIA necesita encontrar el de Microsoft.

**6.1 Visual Studio Build Tools 2022** (compilador de Microsoft, unos 7 GB, 20-40 min)

1. Entra a https://visualstudio.microsoft.com/es/downloads/, baja hasta **"Todas las descargas"** → **"Herramientas para Visual Studio"** → **"Build Tools para Visual Studio 2022"** → **Descargar**.
2. Abre el instalador. En la pestaña **Cargas de trabajo**, marca **una sola casilla**: **"Desarrollo para el escritorio con C++"** (en inglés: *Desktop development with C++*).
3. Clic en **Instalar** (abajo a la derecha) y espera a que termine.

**6.2 CUDA Toolkit** (herramientas de NVIDIA, unos 3 GB, 10-20 min)

1. Entra a https://developer.nvidia.com/cuda-downloads y elige **Windows** → **x86_64** → tu versión de Windows (**10** u **11**) → **exe (local)** → **Download**.
2. Abre el instalador → **Aceptar** → elige **Rápida (Express)** → **Siguiente** hasta terminar.

**6.3 Comprobar.** Abre una PowerShell **nueva**:

```powershell
nvcc --version
```

**Debes ver** algo que termina en `Cuda compilation tools, release 12.x, V12.x.xx`.

**Si en cambio ves** `nvcc no se reconoce`: reinicia la PC y vuelve a probar. Si sigue igual, reinstala el CUDA Toolkit.

- [ ] Instalé Build Tools con "Desarrollo para el escritorio con C++".
- [ ] `nvcc --version` responde.

---

## Paso 7. Compilar el motor CUDA

**Qué vas a hacer:** convertir el código CUDA del proyecto en un programa (`gradiente_cuda.exe`) que usa tu tarjeta.

> **Importante:** este paso **no** se hace en PowerShell, sino en una consola especial de Visual Studio que sabe dónde está el compilador de Microsoft.

1. Presiona la tecla **Windows**, escribe **`x64 Native`** y abre **"x64 Native Tools Command Prompt for VS 2022"**. Es una ventana negra que termina en `C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools>`.
2. Pega estos comandos de uno en uno:

```bat
cd /d C:\saca
if not exist bin mkdir bin
nvcc -O3 -o bin\gradiente_cuda.exe nativo\cuda\gradiente_servidor.cu
bin\gradiente_cuda.exe --info
```

**Debes ver**, tras el comando `nvcc` (tarda unos 30 s), que vuelve el cursor **sin la palabra `error`**; puede mostrar el nombre del archivo o alguna advertencia. Después, el último comando muestra tu tarjeta:

```
GPU: NVIDIA GeForce RTX 3060, sm_86, 28 SM, 12.0 GB
```

**Si en cambio ves:**

| Mensaje | Qué hacer |
|---|---|
| `nvcc fatal : Cannot find compiler 'cl.exe' in PATH` | Estás en PowerShell normal. Abre la **x64 Native Tools Command Prompt** (punto 1) |
| `unsupported Microsoft Visual Studio version` | Tu Visual Studio es más nuevo de lo que admite tu CUDA. Usa este comando en su lugar: `nvcc -O3 -allow-unsupported-compiler -o bin\gradiente_cuda.exe nativo\cuda\gradiente_servidor.cu` |
| `'nvcc' no se reconoce` | Reinicia la PC (paso 6.3) |
| `--info` dice `no hay GPU CUDA disponible` o `CUDA driver version is insufficient` | Actualiza el driver (paso 5) y reinicia |

> Cada vez que el maestro avise que **cambió el motor CUDA**, haz `git pull` y repite este paso.

- [ ] `bin\gradiente_cuda.exe --info` muestra mi tarjeta.

---

## Paso 8. Ensayo individual

**Qué vas a hacer:** comprobar que todo funciona, incluida tu GPU. El script simula en tu PC un clúster completo (un maestro y 2 nodos). Vuelve a **PowerShell** (no la consola de Visual Studio):

```powershell
cd C:\saca
powershell -ExecutionPolicy Bypass -File scripts\cluster-local.ps1 -Nodos 2
```

Si aparece un aviso del **Firewall de Windows** para `maestro.exe`, elige **Permitir** o **Cancelar** (para este ensayo da igual).

**Debes ver** casi al final la tabla con **`cuda`** en la columna `motor` y tu tarjeta en `dispositivo`, y la verificación:

```
nodo           part  motor dispositivo                           ejemplos  cálculo/ép.  desde
local1            0  cuda  NVIDIA GeForce RTX 3060, sm_86, 28 …     73413      0.85 ms  [::1]:60727
local2            1  cuda  NVIDIA GeForce RTX 3060, sm_86, 28 …     73302      0.83 ms  [::1]:60728
...
verificación: pérdida final distribuida 0.125720142, secuencial 0.125720142, diferencia ... → OK, mismo modelo
```

Para confirmar que la GPU pasó la prueba de corrección, pega:

```powershell
Select-String -Path bin\logs\trabajador-local1.log -Pattern "motor cuda"
```

**Debes ver:**

```
... motor cuda iniciado: NVIDIA GeForce RTX 3060, sm_86, 28 SM, 12.0 GB
... motor cuda validado contra el de Go: mismo gradiente (tolerancia relativa 1e-8)
```

**Si en cambio ves:**
- `go` en la columna `motor`: no se encontró o no arrancó `bin\gradiente_cuda.exe`. Repite el paso 7. El ensayo igual vale: tu nodo funciona con la CPU.
- `AVISO: el motor cuda no da el mismo gradiente que el de Go`: copia ese mensaje y el modelo de tu tarjeta, y envíalos al grupo. Tu nodo seguirá con la CPU.
- `falta data/muestra.gob`: paso 4.

- [ ] Mi ensayo dice `OK, mismo modelo`.
- [ ] La columna `motor` dice `cuda` (o avisé al maestro que la mía calcula con CPU).

---

## Paso 9. Enviar tus datos al grupo

```powershell
cd C:\saca
"CPU: " + (Get-CimInstance Win32_Processor).Name.Trim()
"RAM: " + [math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory/1GB) + " GB"
"GPU: " + ((Get-CimInstance Win32_VideoController).Name -join ", ")
"Sistema: " + (Get-CimInstance Win32_OperatingSystem).Caption
"Huella: " + (Get-FileHash data\muestra.gob -Algorithm SHA256).Hash.Substring(0,12).ToLower()
"Version: " + (git rev-parse --short HEAD)
try { "IP Tailscale: " + (tailscale ip -4) } catch { "IP Tailscale: (Tailscale no instalado)" }
& .\bin\gradiente_cuda.exe --info
```

Copia las líneas y pégalas en el grupo con este mensaje:

```
NODO CON NVIDIA — <tu-nombre>
<pega aquí las 8 líneas>
Ensayo individual: OK, mismo modelo — motor: cuda   (o: motor: go, no pude compilar CUDA)
```

- [ ] Envié mis datos al grupo.

---

## Paso 10. Unirte al clúster (ensayo general y presentación)

**10.1 Antes de empezar** (5 minutos antes):

```powershell
cd C:\saca
git pull
tailscale ping <IP-del-maestro>
```

Si el maestro avisó que cambió el motor CUDA, repite también el paso 7.

**10.2 Conéctate.** Cambia `<IP-del-maestro>` y `<tu-nombre>` (en minúsculas, sin tildes ni espacios). **No borres el `:9000`**.

```powershell
cd C:\saca
powershell -ExecutionPolicy Bypass -File scripts\nodo.ps1 -Maestro <IP-del-maestro>:9000 -Id <tu-nombre>
```

Ejemplo: `powershell -ExecutionPolicy Bypass -File scripts\nodo.ps1 -Maestro 100.101.102.103:9000 -Id luis`

**Debes ver:**

```
== 1/4 programa del trabajador
   compilado bin/trabajador.exe (commit 9fce811)
== 2/4 muestra de datos
   huella 5e90db4b1158 (debe ser igual a la que muestra el maestro)
== 3/4 motor de cálculo
   GPU NVIDIA: NVIDIA GeForce RTX 3060
== 4/4 conexión con el maestro 100.101.102.103:9000
   OK
== nodo luis en marcha (Ctrl+C para salir)
... motor cuda iniciado: NVIDIA GeForce RTX 3060, sm_86, 28 SM, 12.0 GB
... registrado como luis: partición 2 de 4 → 38653 de 146715 ejemplos de entrenamiento (26.3%)
gradiente_cuda: 38653 ejemplos x 12 variables en la GPU (224 bloques x 256 hilos)
... motor cuda validado contra el de Go: mismo gradiente (tolerancia relativa 1e-8)
... listo: calcula con el motor cuda (NVIDIA GeForce RTX 3060, ...); esperando los parámetros del maestro
... época 0: gradiente enviado (0.91 ms de cálculo)
...
... entrenamiento finalizado: 200 épocas, pérdida final 0.125720; este nodo calculó 0.85 ms por época en promedio
```

- Si en 3/4 aparece `AVISO: falta bin/gradiente_cuda.exe`, tu nodo funcionará con la CPU. Si hay tiempo, repite el paso 7.
- Si el maestro **todavía no arrancó**, verás `maestro ... no disponible todavía ... reintentando...`. **Es normal: espera.**
- Mientras entrena, **no cierres la ventana** y no suspendas la PC.
- En la presentación, el maestro puede pedirte que **compartas tu pantalla** unos segundos para mostrar la línea `motor cuda validado`.

**10.3 Otra corrida.** Cuando aparece `entrenamiento finalizado`, el nodo termina. Si el maestro pide otra corrida: **flecha ↑** y **Enter**.

**10.4 La caída (solo si el maestro te lo pide).** Presiona **`Ctrl` + `C`** en tu ventana cuando te lo indiquen.

- [ ] Llegué a `entrenamiento finalizado` en el ensayo general.

---

## Paso 11. Al terminar

No hay que desinstalar nada. Puedes desconectar Tailscale desde su ícono (**Disconnect**). La próxima vez, empieza directamente en el paso 10.

---

## Problemas frecuentes

| Lo que ves | Qué hacer |
|---|---|
| `no se puede cargar ... la ejecución de scripts está deshabilitada` | Copia el comando completo, que empieza con `powershell -ExecutionPolicy Bypass -File` |
| `... no se reconoce como nombre de un cmdlet` | Abre una PowerShell nueva; si sigue, reinstala ese programa |
| `maestro ... no disponible todavía` durante mucho rato | ¿Arrancó el maestro? ¿`tailscale status` lista su PC? ¿Escribiste bien la IP y el `:9000`? |
| `el maestro rechazó el registro: muestra_distinta` | Paso 4 |
| `registro_rechazado: todas las particiones ya están asignadas` | Avisa en el grupo para que te cuente en la siguiente corrida |
| `registro_rechazado: ya hay un nodo registrado con ese worker_id` | Cambia tu `-Id` |
| `el maestro cerró la conexión` | El maestro terminó o se reinició. Espera su indicación y repite el paso 10.2 |
| `AVISO: el motor cuda no da el mismo gradiente que el de Go` | Tu nodo sigue con la CPU. Avisa el mensaje y tu modelo de tarjeta |
| `gradiente_cuda: no se pudieron copiar los datos a la GPU` | Cierra juegos o programas que usen la GPU y repite |
| Laptop: `--info` dice que no hay GPU | Conecta el cargador, actualiza el driver (paso 5) y reinicia |

Si nada de esto aplica, copia las **últimas 15 líneas** de tu ventana al grupo.

---

## Linux

Para PCs con Linux (Ubuntu o Debian) y tarjeta NVIDIA. La terminal es **Terminal**; para pegar, `Ctrl` + `Shift` + `V`.

1. **Programas:**

   ```sh
   sudo apt update && sudo apt install -y git curl
   sudo snap install go --classic
   curl -fsSL https://tailscale.com/install.sh | sh
   ```

2. **Tailscale:** `sudo tailscale up`, que muestra un enlace: ábrelo e inicia sesión. Acepta la invitación del maestro y comprueba con `tailscale ip -4` y `tailscale ping <IP-del-maestro>`.
3. **Proyecto y muestra:**

   ```sh
   cd ~ && git clone https://github.com/angelogp27/SACA.git saca
   cp ~/Downloads/muestra.gob ~/saca/data/
   shasum -a 256 ~/saca/data/muestra.gob | cut -c1-12      # debe dar 5e90db4b1158
   ```

4. **Driver y CUDA:** `nvidia-smi` debe mostrar tu tarjeta; si no, instala el driver con `sudo ubuntu-drivers install` y reinicia. Luego:

   ```sh
   sudo apt install -y nvidia-cuda-toolkit
   nvcc --version
   ```

5. **Compilar el motor:**

   ```sh
   cd ~/saca
   mkdir -p bin
   nvcc -O3 -o bin/gradiente_cuda nativo/cuda/gradiente_servidor.cu
   ./bin/gradiente_cuda --info
   ```

6. **Ensayo individual:** `bash scripts/cluster-local.sh 2`. Debe terminar en `OK, mismo modelo`, con `cuda` en la columna `motor`. Para confirmar: `grep "motor cuda" bin/logs/trabajador-local1.log`.
7. **Datos para el grupo:**

   ```sh
   lscpu | grep "Model name"; free -h | grep Mem; ./bin/gradiente_cuda --info
   echo "Huella: $(shasum -a 256 data/muestra.gob | cut -c1-12)"; echo "Version: $(git rev-parse --short HEAD)"; tailscale ip -4
   ```

8. **Unirte al clúster:**

   ```sh
   cd ~/saca && git pull
   bash scripts/nodo.sh <IP-del-maestro>:9000 <tu-nombre>
   ```

   Para otra corrida: **flecha ↑** y **Enter**. Para la caída (solo si te lo piden): `Ctrl` + `C`.
