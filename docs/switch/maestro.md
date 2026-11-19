# Switch: laptop del maestro

Antes: [herramientas](../README.md#por-dónde-empiezo) instaladas y tu IP en [ips.md](ips.md) (el maestro: `192.168.50.1`).

## Windows

PowerShell **como administrador** (clic derecho en Inicio → *Terminal (Administrador)*).

**1. IP fija en el cable** (una vez). Busca el adaptador del cable (`Up` y no Wi-Fi, suele ser `Ethernet`):

```powershell
Get-NetAdapter | Format-Table Name, Status
Set-NetIPInterface -InterfaceAlias "Ethernet" -Dhcp Disabled
New-NetIPAddress -InterfaceAlias "Ethernet" -IPAddress 192.168.50.1 -PrefixLength 24
Set-NetConnectionProfile -InterfaceAlias "Ethernet" -NetworkCategory Private
```

**2. Firewall** (una vez): anuncios (UDP 9001), nodos (TCP 9000), página de los demás (TCP 8080) y pares (TCP 9002).

```powershell
New-NetFirewallRule -DisplayName "SACA anuncios" -Direction Inbound -Protocol UDP -LocalPort 9001 -Action Allow -Profile Private
New-NetFirewallRule -DisplayName "SACA maestro" -Direction Inbound -Protocol TCP -LocalPort 9000,8080 -Action Allow -Profile Private
New-NetFirewallRule -DisplayName "SACA pares" -Direction Inbound -Protocol TCP -LocalPort 9002 -Action Allow -Profile Private
```

**3. Cada sesión:** `ping 192.168.50.2` (a cualquier nodo: menos de 1 ms), luego `powershell -ExecutionPolicy Bypass -File scripts\saca.ps1` y en la página **Iniciar clúster**. Sigue en [uso.md](../uso.md).

---

> ⛔ **¿Usas Windows? Terminaste: no sigas leyendo.** Lo de abajo es solo para Linux (Fedora).

## Linux (Fedora)

**1. IP fija en el cable, con todo abierto solo en ese cable** (una vez). Busca el adaptador (`nmcli device status`, el de tipo *ethernet*) y cambia `enp5s0` por el tuyo:

```sh
nmcli con add type ethernet ifname enp5s0 con-name lab-switch \
  ipv4.method manual ipv4.addresses 192.168.50.1/24 ipv4.never-default yes \
  ipv6.method disabled connection.autoconnect yes connection.zone trusted
nmcli con up lab-switch
```

La zona `trusted` abre los puertos de SACA solo en el cable: no hace falta tocar el firewall.

**2. Cada sesión:** `ping 192.168.50.2`, luego `bash scripts/saca.sh` y **Iniciar clúster**. Sigue en [uso.md](../uso.md).

---

## Si algo falla

- **Los nodos no te encuentran**: la red del cable quedó *Pública* (Windows): `Set-NetConnectionProfile -InterfaceAlias "Ethernet" -NetworkCategory Private`. En Fedora: `nmcli con up lab-switch`.
- **Los nodos aparecen, pero no ven el clúster en vivo**: falta el TCP 8080 (paso 2).
- **Pierdes Internet al conectar el cable**: quedó un gateway en el cable: `Remove-NetRoute -InterfaceAlias "Ethernet" -DestinationPrefix 0.0.0.0/0`.
- **`no se pudo abrir la página en el puerto 8090`**: ya hay un SACA abierto en esta laptop; usa esa ventana.
- **Deshacer todo**: Windows: `Remove-NetIPAddress -InterfaceAlias "Ethernet" -IPAddress 192.168.50.1 -Confirm:$false; Set-NetIPInterface -InterfaceAlias "Ethernet" -Dhcp Enabled; Remove-NetFirewallRule -DisplayName "SACA anuncios","SACA maestro","SACA pares"`. Fedora: `nmcli con delete lab-switch`.
