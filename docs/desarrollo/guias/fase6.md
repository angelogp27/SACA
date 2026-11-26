# OpenMP, CUDA y MPI en el clúster

Todo es opcional: lo que una laptop no tenga aparece desactivado en la página. `scripts/saca.sh` y `scripts/saca.ps1` compilan lo que encuentren.

| Qué | Programa | Necesita |
|---|---|---|
| OpenMP | `bin/red_openmp` | gcc (el mismo de Pthreads) |
| CUDA | `bin/red_cuda` | `nvcc`; en Windows, abrir SACA desde *x64 Native Tools Command Prompt for VS 2022* la primera vez |
| MPI | `bin/puente_mpi` | Open MPI (Fedora) o MS-MPI (Windows) |

## MPI en Fedora

1. En todas: `sudo dnf install -y openmpi openmpi-devel openssh-server && sudo systemctl enable --now sshd`
2. El maestro entra por SSH a los nodos sin contraseña: `ssh-keygen` (una vez) y `ssh-copy-id usuario@192.168.50.X` por cada nodo. Si el usuario es otro, ponlo en `~/.ssh/config` (`Host 192.168.50.X` / `User otro`).
3. Entre laptops, MPI usa puertos al azar: con el cable en la zona `trusted` ([switch](../../switch/nodo.md)) ya pasan.

## MPI en Windows

1. En todas, instala **MS-MPI** (`msmpisetup.exe`, https://learn.microsoft.com/message-passing-interface/microsoft-mpi) y, en MSYS2 UCRT64: `pacman -S --needed mingw-w64-ucrt-x86_64-msmpi`
2. En todas, antes de entrenar: `smpd -d` en una PowerShell. MS-MPI exige el mismo usuario y contraseña de Windows en todas.
3. Permite `mpiexec`, `smpd` y `puente_mpi` en el firewall (redes privadas).

**No se mezclan Windows y Fedora en una corrida con MPI** (MS-MPI y Open MPI no se hablan): la página lo desactiva; usa árbol o anillo.

**Predicción con MPI: solo Windows.** El maestro debe ser Windows; las laptops Linux de la sala quedan fuera de esa predicción (con TCP sí participan).

## Cómo funciona

- El trabajador Go sigue siendo quien habla con el maestro. OpenMP y CUDA son procesos aparte (`nativo/comun/motor_red.h`): reciben los datos una vez y, en cada mini-lote, los parámetros.
- Antes de usarse, cada motor se compara con Go (tolerancia 1e-8). Si CUDA falla, sigue la CPU; si OpenMP no coincide o no está, la corrida se detiene con el motivo.
- Con CUDA, cada mini-lote se parte entre CPU y GPU según la velocidad de cada una. En la predicción, las dos toman trozos de 65 536 filas hasta terminar.
- Con MPI, el maestro lanza con `mpiexec` un puente por laptop; el trabajador le pasa el gradiente y el puente hace `MPI_Allreduce`. En la predicción, `MPI_Gatherv` junta las filas en el rango 0 (no hay `Scatterv`: los datos ya están en cada laptop).
- Los MB por la red con MPI son un estimado: MPI no informa cuántos bytes envió.

## Probar

`go test ./internal/distribuido` usa los programas de `bin/` si existen. Sin GPU, el motor CUDA se prueba emulado: `bash nativo/cuda/emulacion/compilar.sh red` (los tiempos no valen).
