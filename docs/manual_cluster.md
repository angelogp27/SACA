# Manual del clúster: cómo unir tu PC y ejecutar el entrenamiento distribuido

Este manual es para **cualquier integrante del grupo**, aunque no conozca el código. Explica paso a paso cómo convertir las PCs del grupo en un clúster que entrena el modelo de SACA entre todas, **cada una desde su casa**, conectadas por internet.

- Si solo vas a **aportar tu PC como nodo**, sigue los pasos 1, 2, 3 y 6. Si tienes **GPU NVIDIA**, haz también el paso 4.
- Quien maneja **el maestro** hace además el paso 5.
- Antes de la presentación, todos hacen el **ensayo** del paso 9.

Contexto técnico: [guia_cluster.md](guia_cluster.md) (arquitectura) y [contrato_protocolo_tcp.md](contrato_protocolo_tcp.md) (mensajes). Para la exposición, ver [guia_presentacion_cluster.md](guia_presentacion_cluster.md).

---

## Cómo funciona, en una figura

```
                   red privada de Tailscale (por internet, cifrada)
                                       │
       ┌───────────────────────────────┼────────────────────────────────┐
       │                               │                                │
┌──────┴────────────┐       ┌──────────┴──────────┐       ┌─────────────┴────────┐
│ MAESTRO           │◄─TCP──│ NODO CPU (Ana)      │       │ NODO GPU (Luis)      │
│ maestro.exe :9000 │       │ trabajador.exe      │       │ trabajador.exe       │
│ - reparte las     │       │ motor go: goroutines│       │ motor cuda:          │
│   estaciones      │◄──────┼─────────────────────┼─TCP───│ gradiente_cuda.exe   │
│ - suma gradientes │       └─────────────────────┘       │ en la GPU NVIDIA     │
│ - actualiza pesos │                                     └──────────────────────┘
└───────────────────┘
```

1. Cada nodo se conecta al maestro y se **registra**. El maestro le asigna una **partición**: un subconjunto disjunto de las estaciones de monitoreo, por hash de `estacion_id`.
2. En cada **época**, el maestro envía los pesos del modelo (13 números). Cada nodo calcula el **gradiente parcial** sobre sus ejemplos y lo devuelve.
3. El maestro **suma** los gradientes, actualiza los pesos y repite. Como el gradiente es una suma sobre ejemplos independientes, el resultado es **exactamente el mismo** que en una sola PC. El maestro lo comprueba al final.

Cada nodo calcula con lo que tiene: **goroutines** en la CPU (motor `go`) o **CUDA** en una GPU NVIDIA (motor `cuda`).

---

## Paso 0. Inventario (llénenlo entre todos)

| Integrante | Rol | CPU (núcleos/hilos) | RAM | GPU | Sistema operativo | IP de Tailscale | Motor |
|---|---|---|---|---|---|---|---|
| Piero | maestro | Ryzen 5 3600 (6/12) | 16 GB | AMD RX 580 (sin CUDA) | Windows 11 | 100.___.___.___ | go |
| | nodo GPU | | | NVIDIA ___ | | 100.___.___.___ | cuda |
| | nodo GPU | | | NVIDIA ___ | | 100.___.___.___ | cuda |
| | nodo CPU | | | — | | 100.___.___.___ | go |
| | nodo CPU | | | — | | 100.___.___.___ | go |

Cómo obtener tus datos en Windows: `Configuración → Sistema → Información` (CPU y RAM), `nvidia-smi` en PowerShell (GPU NVIDIA) y `tailscale ip -4` (paso 2).

---

## Paso 1. Requisitos según el rol

| Rol | Necesita |
|---|---|
| Nodo CPU | Git, Go 1.27 o superior, Tailscale, el repo y `data/muestra.gob` |
| Nodo GPU | Todo lo anterior, más driver NVIDIA, CUDA Toolkit y Visual Studio Build Tools (paso 4) |
| Maestro | Todo lo del nodo CPU, más abrir el puerto 9000 en el firewall (paso 5) |

No hace falta MongoDB, Docker ni los CSV de la EPA: el clúster trabaja solo con `data/muestra.gob`.

---

## Paso 2. Instalar Go, Git y Tailscale

### 2.1 Go y Git

- Go: https://go.dev/dl/ → instalador `.msi` para Windows. En una PowerShell nueva: `go version` debe mostrar `go1.27` o superior (lo pide `go.mod`; con una versión anterior, Go intenta descargar la correcta).
- Git: https://git-scm.com/download/win. Comprobar con `git --version`.

### 2.2 Tailscale: la red privada del grupo

Las PCs están en casas distintas, detrás de routers que no dejan entrar conexiones. **Tailscale** crea una red privada cifrada entre ellas, sin tocar el router. Cada PC recibe una IP fija del tipo `100.x.y.z`.

1. Descargar e instalar: https://tailscale.com/download (Windows, macOS o Linux).
2. Iniciar sesión (con Google, Microsoft o GitHub).
3. Elegir **una** de estas dos formas de que los nodos vean al maestro:
   - **Opción A (recomendada): compartir el nodo del maestro.** Cada uno usa su propia cuenta. Quien maneja el maestro entra a https://login.tailscale.com/admin/machines, abre el menú `…` de su PC → **Share…** y la comparte con el correo de cada integrante. Cada uno acepta la invitación. Basta con esto, porque **solo los nodos se conectan al maestro**: el maestro nunca abre conexiones hacia los nodos.
   - **Opción B: una sola red para todos.** El dueño de la red invita a los demás desde `Users → Invite users`. El plan gratuito tiene un límite de usuarios; si no alcanza, usar la opción A.
4. Comprobar:

   ```powershell
   tailscale ip -4                  # tu IP 100.x.y.z (anótala en el inventario)
   tailscale ping 100.a.b.c         # la IP del maestro: debe responder
   ```

   `tailscale ping` dice si la conexión es **directa** (`via 190.x.x.x:41641`) o **retransmitida** (`via DERP(mia)`). Las dos funcionan; la retransmitida tiene más latencia. Anota los ms: son parte de los resultados.

> Si Tailscale no estuviera permitido, ZeroTier o Radmin VPN cumplen la misma función: lo único que importa es que el nodo pueda abrir una conexión TCP a `IP_DEL_MAESTRO:9000`.

---

## Paso 3. Obtener el código y la muestra

```powershell
git clone <URL del repositorio> SACA     # la primera vez
cd SACA
git pull                                  # las siguientes veces
git log -1 --oneline                      # todos deben estar en el MISMO commit
```

La muestra de entrenamiento `data/muestra.gob` (unos 20 MB) **no está en git**. Quien tiene la base de datos la genera con `scripts/pipeline.ps1` y la sube a la carpeta compartida del grupo. Descárgala y colócala en `SACA\data\muestra.gob`.

Comprueba que es la misma que la del maestro:

```powershell
(Get-FileHash data\muestra.gob -Algorithm SHA256).Hash.Substring(0,12).ToLower()
```

Debe dar **`5e90db4b1158`**, o la huella que anuncie el maestro si se regeneró. Si no coincide, el maestro rechazará tu nodo con el motivo `muestra_distinta`.

---

## Paso 4. Solo nodos con GPU NVIDIA: compilar el motor CUDA

1. Driver de NVIDIA actualizado. `nvidia-smi` debe listar tu GPU.
2. **Visual Studio Build Tools 2022** (https://visualstudio.microsoft.com/es/downloads/, sección "Herramientas para Visual Studio"), con la carga de trabajo **"Desarrollo para el escritorio con C++"**.
3. **CUDA Toolkit 12.x**: https://developer.nvidia.com/cuda-downloads. Comprobar con `nvcc --version`.
4. Abrir **"x64 Native Tools Command Prompt for VS 2022"** desde el menú Inicio, ir a la carpeta del repo y compilar:

   ```bat
   cd C:\ruta\a\SACA
   mkdir bin
   nvcc -O3 -o bin\gradiente_cuda.exe nativo\cuda\gradiente_servidor.cu
   bin\gradiente_cuda.exe --info
   ```

   Salida esperada: `GPU: NVIDIA GeForce RTX 3060, sm_86, 28 SM, 12.0 GB` (con tu modelo).

Detalles y problemas frecuentes: [nativo/cuda/README.md](../nativo/cuda/README.md). Si no logras compilar, **no pasa nada**: tu nodo calculará con la CPU y el clúster funciona igual.

---

## Paso 5. Solo el maestro: firewall y arranque

### 5.1 Abrir el puerto 9000 (una sola vez)

En una PowerShell **como administrador**:

```powershell
New-NetFirewallRule -DisplayName "SACA maestro 9000" -Direction Inbound -Protocol TCP -LocalPort 9000 -Action Allow
```

La primera vez que se ejecute `maestro.exe`, Windows puede preguntar si se permite el acceso: aceptar en **redes privadas**.

### 5.2 Lanzar el maestro

```powershell
cd SACA
go build -o bin\maestro.exe .\entrenamiento\cmd\maestro
.\bin\maestro.exe -nodos 4 -modo tcp_tailscale -maquina "4 PCs por Tailscale, 1 GPU" -resultado resultados\experimento2.csv
```

- `-nodos` es la cantidad de **nodos trabajadores** (sin contar al maestro). El entrenamiento empieza solo cuando están todos.
- Si el maestro también quiere aportar su CPU, abre otra PowerShell y corre un nodo apuntando a sí mismo (paso 6, con `-Maestro localhost:9000`), y lo cuenta en `-nodos`.

Salida esperada al arrancar:

```
muestra data/muestra.gob, huella 5e90db4b1158 (los nodos deben mostrar la misma)
maestro escuchando en [::]:9000, esperando 4 nodos
  los nodos se conectan con: -maestro 192.168.18.7:9000
  los nodos se conectan con: -maestro 100.101.102.103:9000     ← esta es la de Tailscale
```

Comparte con el grupo la dirección `100.x.y.z:9000`.

---

## Paso 6. Todos los nodos: unirse al clúster

En la carpeta del repo:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\nodo.ps1 -Maestro 100.101.102.103:9000 -Id ana
```

En Linux o macOS: `scripts/nodo.sh 100.101.102.103:9000 ana`.

Usa como `-Id` tu nombre en minúsculas, sin espacios. El script hace 4 comprobaciones y lanza el trabajador:

```
== 1/4 programa del trabajador
   compilado bin/trabajador.exe (commit dd1560f)
== 2/4 muestra de datos
   huella 5e90db4b1158 (debe ser igual a la que muestra el maestro)
== 3/4 motor de cálculo
   GPU NVIDIA: NVIDIA GeForce RTX 3060            ← o "sin GPU NVIDIA: se calcula con goroutines en la CPU"
== 4/4 conexión con el maestro 100.101.102.103:9000
   OK
== nodo ana en marcha (Ctrl+C para salir)
... registrado como ana: partición 1 de 4 → 38975 de 146715 ejemplos de entrenamiento (26.6%)
... motor cuda validado contra el de Go: mismo gradiente (tolerancia relativa 1e-8)     ← solo nodos GPU
... listo: calcula con el motor cuda (...); esperando los parámetros del maestro
```

Deja la ventana abierta. Cuando el maestro termina, el nodo muestra `entrenamiento finalizado` y se cierra solo.

---

## Paso 7. Leer los resultados (en la pantalla del maestro)

Durante el entrenamiento, una línea cada 20 épocas:

```
época  20: pérdida=0.325505 |   48.12 ms (cálculo del más lento   4.69 ms, red y espera   43.43 ms)
```

- **cálculo del más lento**: el nodo que más tarda marca el ritmo de todos, porque el modo es síncrono.
- **red y espera**: todo lo demás, es decir, el viaje de ida y vuelta por internet más la serialización JSON.

Al final, la tabla por nodo y el resumen:

```
nodo           part  motor dispositivo                           ejemplos  cálculo/ép.  desde
piero             0  go    CPU DESKTOP-TLB2NU1, 12 goroutines       34760      1.10 ms  127.0.0.1:61719
ana               1  go    CPU ANA-PC, 8 goroutines                 38975      2.40 ms  100.64.0.5:61720
luis              2  cuda  NVIDIA GeForce RTX 3060, sm_86, ...      38653      0.35 ms  100.64.0.7:52011
...
T(4 nodos) = 9.8s, 49.0 ms por época: cálculo del más lento 2.4 ms + red y espera 46.6 ms (95% del tiempo es comunicación)
evaluación en prueba (meses 10-12): ... F1=0.022 AUC=0.995
verificación: pérdida final distribuida 0.125720142, secuencial 0.125720142, diferencia 2.6e-15 → OK, mismo modelo
secuencial en esta PC: 2.577s → speedup S(4) = 0.26
```

(Los números de este ejemplo son ilustrativos. Los reales se anotan en el ensayo.)

Con `-resultado resultados\experimento2.csv` se agrega una fila `tcp_tailscale` y otra `secuencial` al CSV del Experimento 2. Para regenerar la tabla:

```powershell
go run .\experimentos\cmd\analisis -entrada resultados\experimento2.csv -json frontend\public\datos\experimento2.json -md resultados\experimento2.md -nombre "Experimento 2" -descripcion "Entrenamiento distribuido por TCP" -paralelo "tcp_local,tcp_tailscale" "-serial="
```

---

## Paso 8. Problemas frecuentes

| Lo que ves | Causa probable | Qué hacer |
|---|---|---|
| `maestro ... no disponible todavía ... reintentando` | El maestro no arrancó, Tailscale está desconectado o el firewall bloquea | Que el maestro confirme que está escuchando. `tailscale status` en ambos lados. Regla de firewall del paso 5.1 |
| `el maestro rechazó el registro: muestra_distinta` | Tu `muestra.gob` no es la misma | Descargarla de nuevo y comparar la huella (paso 3) |
| `registro_rechazado: todas las particiones ya están asignadas` | Ya entraron `-nodos` nodos | El maestro debe reiniciarse con un `-nodos` mayor |
| `registro_rechazado: ya hay un nodo registrado con ese worker_id` | Dos personas usan el mismo `-Id` | Cambiar el `-Id` |
| `falta data/muestra.gob` | No se copió la muestra | Paso 3 |
| `AVISO: falta bin/gradiente_cuda.exe` | Nodo GPU sin el motor compilado | Paso 4; mientras tanto calcula con la CPU |
| `AVISO: el motor cuda no da el mismo gradiente que el de Go` | Problema del motor CUDA en esa GPU | Sigue con la CPU. Anotar la GPU y el mensaje y avisar |
| `entrenamiento abortado: época N: el nodo X se cayó` | Un nodo cerró su ventana o perdió la conexión | Es el comportamiento esperado del modo síncrono. Volver a lanzar el maestro y los nodos |
| `nodo(s) no enviaron su gradiente en 2m0s` | Un nodo se colgó (suspensión, wifi) | Desactivar la suspensión de la PC (paso 9). Reintentar |
| `nvcc fatal : Cannot find compiler 'cl.exe'` | No se usó la consola de Visual Studio | Paso 4.4 |
| `go: command not found` / `go no se reconoce` | Go no está instalado o la consola es anterior a la instalación | Instalar Go y abrir una PowerShell nueva |
| `no se puede cargar ... porque la ejecución de scripts está deshabilitada` | Política de PowerShell | Usar `powershell -ExecutionPolicy Bypass -File ...`, como en los ejemplos |

---

## Paso 9. Ensayo (obligatorio antes de presentar)

### 9.1 Ensayo individual (cada uno, en su PC, unos 5 min)

Simula el clúster completo en tu PC, con el maestro y 3 nodos como procesos separados:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\cluster-local.ps1 -Nodos 3
```

Debe terminar con `verificación: ... → OK, mismo modelo`. Si funciona, tu PC tiene todo lo necesario. En un nodo GPU, la línea `motor cuda validado` confirma que el motor CUDA está bien.

> Sin GPU NVIDIA también se puede probar el camino CUDA con la emulación en CPU: `nativo\cuda\emulacion\compilar.ps1` (ver [nativo/cuda/README.md](../nativo/cuda/README.md)).

### 9.2 Ensayo general (todos juntos, por llamada, unos 20 min)

1. Todos en el mismo commit (`git log -1 --oneline`) y con la misma huella.
2. El maestro arranca con `-nodos` = cantidad de nodos presentes.
3. Cada nodo corre `scripts\nodo.ps1`.
4. Anotar en la tabla de abajo la latencia (`tailscale ping`), el tiempo por época y el porcentaje de comunicación.
5. Repetir con 1, 2 y todos los nodos, con `-resultado resultados\experimento2.csv -modo tcp_tailscale`, y subir el CSV al repo.
6. Probar la caída de un nodo: cerrar la ventana de un trabajador a mitad del entrenamiento. El maestro debe abortar con un mensaje claro.

| Fecha | Nodos | Latencia media (ms) | ms/época | % comunicación | S(p) | Pérdida = secuencial |
|---|---|---|---|---|---|---|
| | | | | | | |

### 9.3 Antes de medir o presentar

- Desactivar la suspensión: `Configuración → Sistema → Inicio/apagado → Suspender: Nunca`.
- Cerrar juegos, descargas y la sincronización de OneDrive o Google Drive.
- Preferir cable sobre wifi.
- Cargar la laptop.
