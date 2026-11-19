# VPN (Tailscale): laptop del maestro

Para cuando cada uno está en su casa. Más lento que el [switch](../switch/ips.md) (decenas de ms por mensaje), y es justamente una de las cosas que se miden. Antes: [herramientas](../README.md#por-dónde-empiezo) instaladas.

## Windows

**1. Tailscale** (una vez):

1. Instálalo desde https://tailscale.com/download e inicia sesión.
2. Comparte tu laptop con el grupo: https://login.tailscale.com/admin/machines → `…` de tu laptop → **Share…** → manda el enlace al grupo (cada uno lo acepta con su cuenta). En el mismo menú, **Disable key expiry**.
3. Tu IP: `tailscale ip -4` (algo como `100.x.y.z`). Pásala al grupo.

**2. Firewall** (una vez, PowerShell como administrador): nodos (9000), página de los demás (8080) y pares (9002), solo desde Tailscale:

```powershell
New-NetFirewallRule -DisplayName "SACA (Tailscale)" -Direction Inbound -Protocol TCP -LocalPort 9000,8080,9002 -RemoteAddress 100.64.0.0/10 -Action Allow
```

**3. Cada sesión:** `powershell -ExecutionPolicy Bypass -File scripts\saca.ps1` y en la página **Iniciar clúster**. Sigue en [uso.md](../uso.md).

---

> ⛔ **¿Usas Windows? Terminaste: no sigas leyendo.** Lo de abajo es solo para Linux (Fedora).

## Linux (Fedora)

**1. Tailscale** (una vez):

```sh
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
tailscale ip -4                      # pásala al grupo
```

Comparte tu laptop como en el paso 1.2 de Windows.

**2. Firewall** (una vez): deja pasar todo lo que llega por Tailscale:

```sh
sudo firewall-cmd --zone=trusted --add-interface=tailscale0 --permanent && sudo firewall-cmd --reload
```

**3. Cada sesión:** `bash scripts/saca.sh` y **Iniciar clúster**. Sigue en [uso.md](../uso.md).

---

## Si algo falla

- **Los nodos no conectan**: que prueben `tailscale ping <tu IP 100.x>`. Si no responde, no aceptaron tu enlace o tu laptop está dormida; si responde, falta tu regla de firewall.
- **`tailscale ping` dice `via DERP`**: funciona, solo más lento.
- **"Unirse" no te detecta solo**: es normal por VPN (no hay anuncios); los nodos escriben tu IP `100.x.y.z`.
