# Documentación de SACA

## Por dónde empiezo

1. **Instala lo necesario** (una vez): [Windows](herramientas/windows.md) · [Linux (Fedora)](herramientas/linux-fedora.md)
2. **Conecta la red** (una vez), según dónde estén:
   - En la misma mesa, por cable: [switch](switch/ips.md) → tu IP, y luego [maestro](switch/maestro.md) o [nodo](switch/nodo.md)
   - Cada uno en su casa: [VPN (Tailscale)](vpn/nodo.md) → [maestro](vpn/maestro.md) o [nodo](vpn/nodo.md)
   - **Nunca por Wi-Fi:** SACA solo acepta el cable del switch (`192.168.50.x`) o la VPN (`100.x`). Un nodo que llega por Wi-Fi es rechazado y no se detecta el clúster.
3. **Usa la página**: [uso.md](uso.md)

## En cada sesión

1. `git pull` (todos en el mismo commit).
2. Abre SACA: `powershell -ExecutionPolicy Bypass -File scripts\saca.ps1` (Windows) o `bash scripts/saca.sh` (Linux). Compila y abre la página.
3. Una laptop pulsa **Iniciar clúster**; las demás, **Unirse**. Lo demás (limpieza, entrenamiento, predicción) se hace desde la página.
4. (Opcional) Para ver la CPU, RAM y GPU de todas las laptops en vivo, en otra terminal: `./bin/saca-top` (Linux) o `bin\saca-top.exe` (Windows). `Ctrl+C` para salir. Más: [uso.md](uso.md#ver-el-uso-de-todas-las-laptops-saca-top).

## Lo demás

Arquitectura, contratos, experimentos, plan de avance y presentación: [desarrollo/](desarrollo/README.md).
