# Switch: laptop de un nodo

Antes: [herramientas](../README.md#por-dónde-empiezo) instaladas y tu IP en [ips.md](ips.md) (en los ejemplos, `192.168.50.2`: cámbiala por la tuya).

## Windows

PowerShell **como administrador** (clic derecho en Inicio → *Terminal (Administrador)*).

**1. IP fija en el cable** (una vez). Busca el adaptador del cable (`Up` y no Wi-Fi, suele ser `Ethernet`):

```powershell
Get-NetAdapter | Format-Table Name, Status
Set-NetIPInterface -InterfaceAlias "Ethernet" -Dhcp Disabled
New-NetIPAddress -InterfaceAlias "Ethernet" -IPAddress 192.168.50.2 -PrefixLength 24
Set-NetConnectionProfile -InterfaceAlias "Ethernet" -NetworkCategory Private
```

**2. Firewall** (una vez): anuncios (UDP 9001) y pares (TCP 9002), para el entrenamiento en árbol o en anillo.

```powershell
New-NetFirewallRule -DisplayName "SACA anuncios" -Direction Inbound -Protocol UDP -LocalPort 9001 -Action Allow -Profile Private
New-NetFirewallRule -DisplayName "SACA pares" -Direction Inbound -Protocol TCP -LocalPort 9002 -Action Allow -Profile Private
```

Si algún día eres el maestro, agrega también la regla de [maestro.md](maestro.md).

**3. Cada sesión:** `ping 192.168.50.1` (al maestro: menos de 1 ms), luego `powershell -ExecutionPolicy Bypass -File scripts\saca.ps1` y en la página **Unirse al clúster** (debe decir *Maestro detectado*; si no, escribe `192.168.50.1`). Sigue en [uso.md](../uso.md).

---

> ⛔ **¿Usas Windows? Terminaste: no sigas leyendo.** Lo de abajo es solo para Linux (Fedora).

## Linux (Fedora)

**1. IP fija en el cable, con todo abierto solo en ese cable** (una vez). Busca el adaptador (`nmcli device status`, el de tipo *ethernet*) y cambia `enp5s0` y la IP por los tuyos:

```sh
nmcli con add type ethernet ifname enp5s0 con-name lab-switch \
  ipv4.method manual ipv4.addresses 192.168.50.2/24 ipv4.never-default yes \
  ipv6.method disabled connection.autoconnect yes connection.zone trusted
nmcli con up lab-switch
```

**2. Cada sesión:** `ping 192.168.50.1`, luego `bash scripts/saca.sh` y **Unirse al clúster**. Sigue en [uso.md](../uso.md).

---

## Si algo falla

- **"No se detectó ningún clúster"**: escribe `192.168.50.1` y pulsa *Unirse*. Si tampoco conecta, el maestro no abrió su firewall o aún no pulsó *Iniciar clúster*.
- **"No se pudo conectar con el par …:9002"** al entrenar: falta la regla *SACA pares* en esa laptop. Mientras tanto, el maestro puede elegir *Todo por el maestro*.
- **Dos laptops con la misma IP** (Windows avisa *conflicto*): revisa [ips.md](ips.md) y cambia la tuya: `Remove-NetIPAddress -InterfaceAlias "Ethernet" -IPAddress 192.168.50.2` y vuelve al paso 1.
- **Pierdes Internet al conectar el cable**: `Remove-NetRoute -InterfaceAlias "Ethernet" -DestinationPrefix 0.0.0.0/0`.
- **Deshacer todo**: Windows: `Remove-NetIPAddress -InterfaceAlias "Ethernet" -IPAddress 192.168.50.2 -Confirm:$false; Set-NetIPInterface -InterfaceAlias "Ethernet" -Dhcp Enabled; Remove-NetFirewallRule -DisplayName "SACA anuncios","SACA pares"`. Fedora: `nmcli con delete lab-switch`.
