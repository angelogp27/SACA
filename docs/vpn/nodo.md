# VPN (Tailscale): laptop de un nodo

Para cuando cada uno está en su casa. Antes: [herramientas](../README.md#por-dónde-empiezo) instaladas. Si te toca ser el maestro, sigue [maestro.md](maestro.md).

## Windows

**1. Tailscale** (una vez):

1. Instálalo desde https://tailscale.com/download e inicia sesión.
2. Abre el enlace que compartió el maestro y acéptalo con tu cuenta.
3. Comprueba que lo ves: `tailscale ping <IP del maestro>` (su `100.x.y.z`).

**2. Firewall** (una vez, PowerShell como administrador): pares (9002, para el entrenamiento en árbol o en anillo), solo desde Tailscale:

```powershell
New-NetFirewallRule -DisplayName "SACA pares (Tailscale)" -Direction Inbound -Protocol TCP -LocalPort 9002 -RemoteAddress 100.64.0.0/10 -Action Allow
```

**3. Cada sesión:** `powershell -ExecutionPolicy Bypass -File scripts\saca.ps1`, escribe la IP del maestro en *IP del maestro* y pulsa **Unirse al clúster**. Sigue en [uso.md](../uso.md).

---

> ⛔ **¿Usas Windows? Terminaste: no sigas leyendo.** Lo de abajo es solo para Linux (Fedora).

## Linux (Fedora)

**1. Tailscale** (una vez):

```sh
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
tailscale ping <IP del maestro>
```

Acepta el enlace del maestro como en el paso 1.2 de Windows.

**2. Firewall** (una vez):

```sh
sudo firewall-cmd --zone=trusted --add-interface=tailscale0 --permanent && sudo firewall-cmd --reload
```

**3. Cada sesión:** `bash scripts/saca.sh`, escribe la IP del maestro y **Unirse al clúster**. Sigue en [uso.md](../uso.md).

---

## Si algo falla

- **"No se pudo conectar con el maestro 100.x.y.z:9000"**: si `tailscale ping` no responde, no aceptaste su enlace o su laptop está dormida; si responde, al maestro le falta su regla de firewall.
- **"No se pudo conectar con el par …:9002"** al entrenar: falta tu regla del paso 2. Mientras tanto, el maestro puede elegir *Todo por el maestro*.
- **Copiar `muestra.bin` por Internet** (sin CSV): súbelo a Drive o usa `tailscale file cp data/muestra.bin <laptop>:`.
