# Clúster por switch (cable, red local)

Todas las laptops en la misma mesa, conectadas por cable a un switch. Es la red más rápida y estable: latencia menor a 1 ms (por Tailscale son decenas de ms). Las laptops **no necesitan Internet** para el clúster; cada una sigue usando su Wi-Fi para Internet.

```
              ┌──────────────── switch ────────────────┐
              │                │                       │
     cable (o adaptador USB)   cable                   cable
              │                │                       │
     ┌────────┴──────┐  ┌──────┴────────┐      ┌───────┴───────┐
     │ laptop A      │  │ laptop B      │      │ laptop C      │
     │ 192.168.50.1  │  │ 192.168.50.2  │      │ 192.168.50.3  │
     └───────────────┘  └───────────────┘      └───────────────┘
   Cualquiera puede ser el maestro: lo decide quien pulse "Iniciar clúster".
```

**Antes:** cada laptop hizo la [instalación](../instalacion.md) (programas, gcc, proyecto, CSV).

| Paso | Qué | Una vez o cada sesión |
|---|---|---|
| 1 | Plan de IPs | Una vez |
| 2 | IP fija en el cable (Fedora o Windows) | Una vez por laptop |
| 3 | Abrir puertos del firewall | Una vez por laptop |
| 4 | Comprobar la red | Cada sesión (10 s) |
| 5 | Lanzar SACA y armar el clúster | Cada sesión |

---

## 1. Plan de IPs

Una subred solo para el switch, **sin gateway** (así Internet sigue por Wi-Fi). Cada laptop, una IP distinta:

| Laptop | IP |
|---|---|
| Fedora (Angelo) | `192.168.50.1` |
| Windows 1 | `192.168.50.2` |
| Windows 2 | `192.168.50.3` |
| … la siguiente | `192.168.50.4`, `.5`, … |

La IP **no** decide quién es el maestro: el lanzador anuncia al maestro por la red y los demás lo encuentran solos.

---

## 2. IP fija en el cable

### Fedora

1. Busca el nombre del adaptador del cable (con adaptador USB suele ser algo como `enp5s0f4u1c2`):

   ```sh
   nmcli device status        # el de tipo "ethernet"
   ```

2. Crea el perfil (cambia el nombre del adaptador y la IP según tu fila del plan). La zona `trusted` deja pasar todo **solo por este cable** (es una red aislada de la mesa):

   ```sh
   nmcli con add type ethernet ifname enp5s0f4u1c2 con-name lab-switch \
     ipv4.method manual ipv4.addresses 192.168.50.1/24 ipv4.never-default yes \
     ipv6.method disabled connection.autoconnect yes connection.zone trusted
   nmcli con up lab-switch
   ip -4 -br addr show enp5s0f4u1c2      # debe mostrar 192.168.50.1/24
   ```

### Windows

PowerShell **como administrador** (clic derecho en Inicio → *Terminal (Administrador)*).

1. Busca el adaptador del cable: el que está `Up` y no es Wi-Fi (suele llamarse `Ethernet` o `Ethernet 2`):

   ```powershell
   Get-NetAdapter | Format-Table Name, InterfaceDescription, Status, LinkSpeed
   ```

2. IP fija **sin gateway** y red **Privada** (cambia `"Ethernet"` por el nombre y `.2` por tu IP):

   ```powershell
   Set-NetIPInterface -InterfaceAlias "Ethernet" -Dhcp Disabled
   New-NetIPAddress -InterfaceAlias "Ethernet" -IPAddress 192.168.50.2 -PrefixLength 24
   Set-NetConnectionProfile -InterfaceAlias "Ethernet" -NetworkCategory Private
   ```

---

## 3. Firewall

Qué puertos usa SACA:

| Puerto | Para qué | Dónde abrirlo |
|---|---|---|
| UDP **9001** | Anuncios del clúster (para que "Unirse" encuentre al maestro y "Iniciar" sepa si ya hay uno) | En **todas** |
| TCP **9000** | Los nodos se conectan al maestro | En el maestro |
| TCP **8080** | Las otras laptops ven el clúster en vivo (su página habla con la API del maestro) | En el maestro |
| TCP **9002** | Los nodos se suman los gradientes **entre ellos** (comunicación en árbol o en anillo) | En **todas** |

Como **cualquier** laptop puede ser el maestro, lo más simple es abrir los cuatro en todas.

- **Fedora:** con `connection.zone trusted` del paso 2 ya está todo abierto en el cable. Compruébalo: `firewall-cmd --get-zone-of-interface=enp5s0f4u1c2` → `trusted`.
- **Windows** (PowerShell como administrador):

  ```powershell
  New-NetFirewallRule -DisplayName "SACA anuncios" -Direction Inbound -Protocol UDP -LocalPort 9001 -Action Allow -Profile Private
  New-NetFirewallRule -DisplayName "SACA maestro" -Direction Inbound -Protocol TCP -LocalPort 9000,8080 -Action Allow -Profile Private
  New-NetFirewallRule -DisplayName "SACA pares" -Direction Inbound -Protocol TCP -LocalPort 9002 -Action Allow -Profile Private
  ```

  `-Profile Private` funciona porque en el paso 2 marcaste el cable como red **Privada**.

La página de cada laptop (`127.0.0.1:8090`) solo se abre desde esa misma laptop; no hace falta abrir el 8090.

---

## 4. Comprobar la red (cada sesión)

Desde cada laptop, ping a otra:

```sh
ping 192.168.50.1          # Windows: ping 192.168.50.1   → tiempos de <1 ms o 1 ms
```

---

## 5. Lanzar SACA y armar el clúster

En **cada** laptop, desde la carpeta del proyecto:

```sh
bash scripts/saca.sh                                             # Linux
powershell -ExecutionPolicy Bypass -File scripts\saca.ps1        # Windows
```

Se abre la página en `http://127.0.0.1:8090`, pestaña **Clúster**:

1. **Una sola laptop** pulsa **INICIAR CLÚSTER** (será el maestro). Por defecto también participa como nodo.
2. Las demás pulsan **UNIRSE AL CLÚSTER**: debe decir *Maestro detectado: … (192.168.50.x)*. Si no lo detecta, escribe la IP del maestro.
3. En el maestro aparecen todos los nodos. **INICIAR LIMPIEZA**: cada laptop limpia sus CSV con Pthreads y luego con Go, y genera su `data/muestra.bin`. Si ya limpiaron antes, **SALTAR LIMPIEZA** solo comprueba que todos tengan el mismo archivo (misma huella).
4. **Continuar** → elige los nodos (gris = no participa) → **INICIAR ENTRENAMIENTO**.

Solo la laptop del maestro tiene los botones; las demás ven todo en vivo. Los tiempos de limpieza quedan en `resultados/limpieza.csv` y los de entrenamiento en `resultados/cluster.csv` (en la laptop del maestro).

---

## Casos especiales

### Configuración

- **Esta laptop ya estaba configurada** (de una sesión anterior): no repitas los pasos 2 y 3. Comprueba solamente:
  - Fedora: `nmcli con show --active` lista `lab-switch`; si no, `nmcli con up lab-switch`. Si creaste `lab-switch` con la guía anterior (sin zona, abriendo solo el 9000), ponle la zona: `nmcli con mod lab-switch connection.zone trusted && nmcli con up lab-switch`.
  - Windows: `Get-NetIPAddress -InterfaceAlias "Ethernet" -AddressFamily IPv4` muestra tu `192.168.50.x`, y `Get-NetFirewallRule -DisplayName "SACA*"` lista las dos reglas.
- **`New-NetIPAddress`: la IP ya existe**: ya estaba puesta; sigue.
- **Dos laptops con la misma IP**: Windows avisa *conflicto de direcciones* o el ping responde a ratos. Cambia una: `Remove-NetIPAddress -InterfaceAlias "Ethernet" -IPAddress 192.168.50.2` y vuelve a poner otra.
- **El adaptador cambió de nombre** (otro puerto USB, otro adaptador): en Fedora `nmcli con mod lab-switch connection.interface-name <nuevo>`; en Windows repite el paso 2 con el nombre nuevo.
- **Laptop sin puerto Ethernet**: un adaptador USB-Ethernet sirve. Si es de 100 Mbps (como el `1a86:e396` de Fedora), no importa: en cada época viajan unos pocos números; lo que pesa es la latencia.
- **Windows pierde Internet al conectar el cable**: le quedó un gateway en el cable. `Remove-NetRoute -InterfaceAlias "Ethernet" -DestinationPrefix 0.0.0.0/0`.
- **La red del cable sale como "Pública"** (Windows la reclasificó): `Set-NetConnectionProfile -InterfaceAlias "Ethernet" -NetworkCategory Private`. Si sigue Pública, las reglas `-Profile Private` no aplican y "Unirse" no detecta al maestro.

### Al armar el clúster

- **"Ya hay un clúster iniciado por X"**: otra laptop ya es el maestro; pulsa *Unirse*. Si ese maestro ya se cerró, el aviso desaparece solo en unos 4 s.
- **"No se detectó ningún clúster"**: el anuncio UDP no llega (firewall del puerto 9001, red Pública o el maestro aún no inició). Escribe la IP del maestro en el campo y pulsa *Unirse*.
- **"No se pudo conectar con el maestro 192.168.50.x:9000"**: falta abrir el TCP 9000 en el maestro, o el maestro se cerró.
- **Las otras laptops no ven el clúster en vivo** (pero sus nodos sí aparecen en el maestro): falta abrir el TCP 8080 en el maestro.
- **"No se pudo conectar con el par …:9002"** al entrenar en árbol o anillo: falta abrir el TCP 9002 en esa laptop. Mientras tanto, elige *Todo por el maestro*.
- **Cambiar de maestro**: en el maestro actual, *Apagar clúster*; en la laptop nueva, *Iniciar clúster*; las demás, *Salir del clúster* y *Unirse*.
- **Un nodo llega tarde a la limpieza**: aparece arriba a la derecha de la pantalla de limpieza; tócalo para sumarlo. También puede llegar después y limpiar desde *Realizar limpieza*.
- **"Ya hay una página de SACA abierta"** (`no se pudo abrir la página en el puerto 8090`): ya hay un `saca` corriendo en esa laptop; usa esa ventana o ciérrala con Ctrl+C.

### Limpieza y huellas

- **Un nodo sale con otra huella** (rojo en la pantalla de elegir nodos): casi siempre es otro commit o un cargador compilado a mano sin `-ffp-contract=off`. En esa laptop: `git pull`, vuelve a correr `scripts/saca` (recompila) y *Realizar limpieza*. Al pulsar *Iniciar entrenamiento*, los nodos con otra huella se quitan solos de la selección.
- **Un nodo no tiene los CSV**: su limpieza sale como error. Copia `data/muestra.bin` de otra laptop (ver [instalación, casos especiales](../instalacion.md#casos-especiales)).
- **Pthreads sale con error `falta bin\cargador_pthreads.exe`**: esa laptop no tiene gcc; su limpieza con Go vale igual.
- **El maestro no tiene muestra** (*el maestro no tiene muestra.bin*): incluye al nodo del maestro en la limpieza, o copia el `.bin` a la laptop del maestro.
- **Parar la limpieza**: *Parar limpieza* pregunta si guardar los tiempos de lo que ya terminó. Un `.bin` a medias nunca reemplaza al bueno.

### Después de entrenar

- **El mapa no cambió**: el maestro necesita MongoDB corriendo para regenerarlo (ver [instalación](../instalacion.md#mongodb-solo-en-la-laptop-del-maestro-opcional)). Recarga la pestaña *Tablero*.

---

## Volver a como estaba

- Fedora: `nmcli con down lab-switch` (o `nmcli con delete lab-switch` para borrarlo).
- Windows (administrador):

  ```powershell
  Remove-NetIPAddress -InterfaceAlias "Ethernet" -IPAddress 192.168.50.2 -Confirm:$false
  Set-NetIPInterface -InterfaceAlias "Ethernet" -Dhcp Enabled
  Remove-NetFirewallRule -DisplayName "SACA anuncios", "SACA maestro", "SACA pares"
  ```
