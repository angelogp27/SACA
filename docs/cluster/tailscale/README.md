# Clúster por Tailscale (VPN, cada uno desde su casa)

Tailscale une las laptops del grupo en una red privada por Internet: cada una recibe una IP `100.x.y.z` y se ven como si estuvieran en la misma red. Sirve cuando no están en la misma mesa. Es más lento que el [switch](../switch/README.md) (decenas de ms por época en vez de menos de 1 ms), lo que es justamente un resultado a medir.

**Antes:** cada laptop hizo la [instalación](../instalacion.md).

> **Diferencia con el switch:** Tailscale **no** transmite anuncios de broadcast, así que *Unirse* no detecta al maestro solo: hay que **escribir la IP de Tailscale del maestro** (`100.x.y.z`). Todo lo demás es igual.

---

## 1. Tailscale (una vez por laptop)

1. Instala Tailscale: https://tailscale.com/download (Windows: el instalador; Linux: `curl -fsSL https://tailscale.com/install.sh | sh` y `sudo tailscale up`).
2. Inicia sesión con tu cuenta.
3. **El maestro** comparte su máquina: en https://login.tailscale.com/admin/machines → menú `…` de su laptop → **Share…** → copia el enlace y lo manda al grupo. Cada uno lo abre y lo acepta con **su** cuenta. Conviene también **Disable key expiry** en la laptop del maestro.
4. Anota tu IP: `tailscale ip -4`.
5. Comprueba que ves al maestro: `tailscale ping <IP-del-maestro>`.

Como cualquiera puede ser el maestro, lo ideal es que **todos** compartan su máquina con todos.

---

## 2. Firewall (una vez por laptop)

| Puerto | Para qué | Dónde |
|---|---|---|
| TCP **9000** | Los nodos se conectan al maestro | En el maestro |
| TCP **8080** | Las otras laptops ven el clúster en vivo | En el maestro |
| UDP 9001 | Anuncios: **no sirve por Tailscale** (no hay broadcast) | No hace falta |

- **Fedora:** deja pasar todo lo que llega por la interfaz de Tailscale:

  ```sh
  sudo firewall-cmd --zone=trusted --add-interface=tailscale0 --permanent && sudo firewall-cmd --reload
  ```

- **Windows** (PowerShell como administrador). La regla se limita a las IPs de Tailscale (`100.64.0.0/10`), sin importar si Windows marcó esa red como pública o privada:

  ```powershell
  New-NetFirewallRule -DisplayName "SACA maestro (Tailscale)" -Direction Inbound -Protocol TCP -LocalPort 9000,8080 -RemoteAddress 100.64.0.0/10 -Action Allow
  ```

---

## 3. Lanzar SACA y armar el clúster (cada sesión)

En cada laptop:

```sh
bash scripts/saca.sh                                             # Linux
powershell -ExecutionPolicy Bypass -File scripts\saca.ps1        # Windows
```

1. El maestro pulsa **INICIAR CLÚSTER** y comparte su IP de Tailscale en el grupo (`tailscale ip -4`).
2. Los demás escriben esa IP en *IP del maestro* y pulsan **UNIRSE AL CLÚSTER**.
3. Desde ahí, igual que por switch: **INICIAR LIMPIEZA** (o **SALTAR LIMPIEZA**) → **Continuar** → elegir nodos → **INICIAR ENTRENAMIENTO**.

Los resultados del entrenamiento quedan en `resultados/cluster.csv` de la laptop del maestro, con el mismo nombre de modo (`tcp_lan`) que por switch. Para no mezclar las mediciones de las dos redes, renombra el archivo al terminar cada sesión (p. ej. `cluster_tailscale.csv`).

---

## Casos especiales

Valen todos los de [switch, casos especiales](../switch/README.md#casos-especiales) que no tratan del cable (huellas, nodo que llega tarde, cambiar de maestro, MongoDB). Además:

- **"No se detectó ningún clúster en la red"**: es lo normal por Tailscale; escribe la IP `100.x.y.z` del maestro.
- **"No se pudo conectar con el maestro 100.x.y.z:9000"**:
  - `tailscale ping <IP>` no responde: no aceptaste el enlace compartido del maestro, o su laptop está apagada o dormida.
  - Responde pero no conecta: falta la regla del firewall del paso 2 en el maestro.
- **Esta laptop ya estaba configurada** (de las guías anteriores): si ya tenías la regla `SACA maestro 9000`, agrega solo el 8080 (o crea la regla del paso 2). Comprueba con `Get-NetFirewallRule -DisplayName "SACA*"`.
- **La conexión pasa por `DERP`** (`tailscale ping` dice `via DERP(...)`): funciona, pero más lento; no es un error.
- **Copiar `muestra.bin` por Internet** (laptop sin CSV): 360 MB tardan minutos; súbelo a Drive o usa `tailscale file cp data/muestra.bin <laptop>:`.

---

## Guías anteriores (flujo manual)

Antes del lanzador, el clúster se armaba a mano con `scripts/nodo.ps1` y una `muestra.gob` copiada de Drive. Esas guías por rol siguen en [anterior/](anterior/) porque traen detalles útiles (instalación paso a paso para quien nunca usó una terminal, y **cómo compilar el motor CUDA** en un nodo con NVIDIA), pero sus pasos de la muestra están **desactualizados**:

- [Maestro](anterior/guia_maestro.md)
- [Nodo sin GPU NVIDIA](anterior/guia_nodo_sin_nvidia.md)
- [Nodo con GPU NVIDIA](anterior/guia_nodo_con_nvidia.md)
