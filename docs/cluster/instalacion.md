# Instalación (una vez por laptop)

Lo mismo para el switch y para Tailscale. Al final de esta guía cada laptop compila todo con **un solo comando** y abre la página de SACA. La red (cable o VPN) se configura después, en [switch/](switch/README.md) o [tailscale/](tailscale/README.md).

| Qué | Para qué | ¿Obligatorio? |
|---|---|---|
| Git | Descargar el proyecto y actualizarlo (`git pull`) | Sí |
| Go 1.27 o mayor | Compilar el maestro, el nodo, el cargador de Go y el lanzador | Sí |
| Node.js (con npm) | Compilar la página | Sí, salvo que copies `frontend/dist` de otra laptop |
| gcc | Compilar el cargador de **Pthreads** (en Windows: MinGW-w64 de MSYS2) | No: sin él, la laptop limpia solo con Go |
| Los 4 CSV de la EPA en `data/raw` (~4 GB) | Que la laptop pueda **limpiar** | No: sin ellos, copia el `muestra.bin` (ver [casos especiales](#casos-especiales)) |
| MongoDB | Solo en el maestro, para **regenerar el mapa** al terminar un entrenamiento | No |
| Driver NVIDIA + CUDA | Que el nodo entrene con la GPU | No (ver [guía NVIDIA](tailscale/anterior/guia_nodo_con_nvidia.md#paso-5-revisar-la-tarjeta-nvidia-y-su-driver)) |

---

## Windows

### 1. Programas

| Programa | Dónde | En el instalador |
|---|---|---|
| Git | https://git-scm.com/download/win | Next en todo |
| Go | https://go.dev/dl/ → el `.msi` que termina en `windows-amd64.msi` | Next en todo |
| Node.js | https://nodejs.org → versión **LTS** | Next en todo |

Abre una **PowerShell nueva** (tecla Windows → `PowerShell`) y comprueba:

```powershell
git --version; go version; node --version; npm --version
```

Si alguno dice `no se reconoce como nombre de un cmdlet`, cierra todas las PowerShell y abre otra; si sigue, reinstálalo.

### 2. gcc para Pthreads (MSYS2, UCRT64)

Windows no trae compilador de C. MSYS2 instala **MinGW-w64**, que incluye la biblioteca de Pthreads para Windows (winpthreads).

1. Descarga e instala MSYS2 desde https://www.msys2.org (deja la carpeta `C:\msys64`).
2. Abre, desde el menú Inicio, la terminal **"MSYS2 UCRT64"** (la del ícono amarillo; **no** "MSYS2 MSYS") y pega:

   ```sh
   pacman -Syu --noconfirm
   pacman -S --needed --noconfirm mingw-w64-ucrt-x86_64-gcc
   ```

   Si la primera línea cierra la ventana (actualiza el propio MSYS2), ábrela otra vez y repite las dos.

3. Agrega gcc al PATH de Windows. En una PowerShell **normal**:

   ```powershell
   [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path", "User") + ";C:\msys64\ucrt64\bin", "User")
   ```

4. Abre una PowerShell **nueva** y comprueba:

   ```powershell
   gcc --version     # gcc.exe (Rev..., Built by MSYS2 project) 14.x o mayor
   ```

El script compila el cargador con `-static`, así que el `.exe` no necesita las DLL de MSYS2: si en una laptop no puedes instalar MSYS2, **copia `bin\cargador_pthreads.exe` de otra laptop Windows** que sí lo compiló.

> **Importante:** el cargador se compila con `-ffp-contract=off`. Sin esa opción, gcc puede fusionar operaciones de punto flotante y el `muestra.bin` saldría con **otra huella** que el resto. Los scripts ya la ponen; si compilas a mano, no la quites.

### 3. El proyecto

```powershell
cd C:\
git clone https://github.com/angelogp27/SACA.git saca
cd C:\saca
```

Si ya lo tenías: `cd C:\saca; git pull`. Todas las laptops deben estar en el **mismo commit** (`git log -1 --oneline`).

### 4. Los datos (CSV de la EPA)

```powershell
go run .\herramientas\cmd\descargar            # ~4 GB, unos 10 min
go run .\herramientas\cmd\descargar -verificar # comprueba tamaño y SHA-256 contra data\manifiesto.json
```

Si alguien ya los tiene, es más rápido copiar la carpeta `data\raw` completa por USB.

### 5. Compilar y abrir la página

```powershell
powershell -ExecutionPolicy Bypass -File scripts\saca.ps1
```

Compila `bin\maestro.exe`, `bin\trabajador.exe`, `bin\cargador_concurrente.exe`, `bin\cargador_pthreads.exe` (si hay gcc), `bin\exportar.exe`, `bin\saca.exe` y la página, y abre **http://127.0.0.1:8090**. Las siguientes veces, sin recompilar: `scripts\saca.ps1 -SinCompilar`.

La **primera vez** que corran `maestro.exe` y `trabajador.exe`, Windows puede preguntar por el firewall: marca **redes privadas** y **Permitir acceso**.

---

## Linux (Fedora)

```sh
sudo dnf install -y git golang nodejs npm gcc    # o Go desde https://go.dev/dl/ si la de dnf es menor a 1.27
git clone https://github.com/angelogp27/SACA.git saca && cd saca
go run ./herramientas/cmd/descargar              # los 4 CSV en data/raw
bash scripts/saca.sh                             # compila todo y abre http://127.0.0.1:8090
```

En Ubuntu/Debian: `sudo apt install git golang nodejs npm build-essential`. Las siguientes veces: `bash scripts/saca.sh --sin-compilar`.

---

## MongoDB (solo en la laptop del maestro, opcional)

El clúster **no** usa MongoDB para limpiar ni para entrenar. Solo se usa para **regenerar el mapa y la línea de tiempo** de la pestaña *Tablero* cuando termina un entrenamiento: el maestro guarda el modelo nuevo en `data/modelo.json` y corre `bin/exportar`, que lee los registros de MongoDB.

- Si MongoDB está corriendo y cargado (ver [guía de ejecución](../guias/guia_ejecucion.md#tutorial-2-sistema-completo-de-punta-a-punta)), el mapa se actualiza solo al terminar cada entrenamiento (recarga la pestaña *Tablero*).
- Si no, el maestro escribe `mapa no regenerado (¿MongoDB está corriendo?)` y el mapa se queda como estaba. El entrenamiento no se ve afectado.

---

## Casos especiales

- **La laptop no tiene los CSV** (o no hay espacio para 4 GB): no puede limpiar, pero sí entrenar. Copia `data/muestra.bin` (unos 360 MB) de una laptop que ya limpió, por USB o carpeta compartida, y comprueba que la huella sea la del maestro:
  - Windows: `(Get-FileHash data\muestra.bin -Algorithm SHA256).Hash.Substring(0,12).ToLower()`
  - Linux: `sha256sum data/muestra.bin | cut -c1-12`
- **Sin gcc** (no se pudo instalar MSYS2): la laptop limpia solo con Go; en la pantalla de limpieza su columna de Pthreads sale como error `falta bin\cargador_pthreads.exe`. Su `muestra.bin` es igual de válido.
- **Sin Node.js**: copia la carpeta `frontend\dist` de otra laptop y usa `-SinCompilar` (Windows) o `--sin-compilar` (Linux) después de compilar los programas de Go a mano (ver el script).
- **Poca RAM (8 GB)**: la limpieza con Go usa unos 4,5 GB y la de Pthreads unos 2 GB. Cierra el navegador pesado y otros programas mientras limpia.
- **`go: go.mod requires go >= 1.27`**: tu Go es viejo; instala el de https://go.dev/dl/.
