# Herramientas en Linux (Fedora) (una vez)

Al terminar, tu laptop compila y abre SACA con un solo comando. Después sigue con la red: [switch](../switch/ips.md) o [VPN](../vpn/nodo.md).

## 1. Programas y proyecto

```sh
sudo dnf install -y git golang nodejs npm gcc openmpi openmpi-devel openssh-server
sudo systemctl enable --now sshd
git clone https://github.com/angelogp27/SACA.git saca && cd saca
```

Si `go version` es menor que la que pide `go.mod`, instala Go desde https://go.dev/dl/. En Ubuntu/Debian: `sudo apt install git golang nodejs npm build-essential openmpi-bin libopenmpi-dev openssh-server`.

## 2. Los 4 CSV de la EPA (~4 GB)

Van en `saca/data/raw/` con estos nombres exactos:

| Archivo | Gas |
|---|---|
| `hourly_42101_2025.csv` | CO |
| `hourly_42401_2025.csv` | SO2 |
| `hourly_42602_2025.csv` | NO2 |
| `hourly_44201_2025.csv` | O3 |

Cópialos de otra laptop (USB o Drive) o descárgalos: `go run ./herramientas/cmd/descargar` (unos 10 min).

**Sin espacio para 4 GB:** copia solo `data/muestra.bin` (~360 MB) de una laptop que ya limpió. No podrás limpiar, pero sí entrenar y predecir.

## 3. Primera vez

```sh
bash scripts/saca.sh
```

Compila todo y abre **http://127.0.0.1:8090**. En el paso 3/4 debe salir `bin/puente_mpi`.

## 4. NVIDIA (opcional)

Solo si tienes GPU NVIDIA. Ayuda a la CPU en el entrenamiento y la predicción. Si no te sale, sáltalo: tu laptop funciona igual.

```sh
nvidia-smi                                    # tu tarjeta y CUDA Version 12 o mayor
sudo dnf install cuda-toolkit                 # repositorio de NVIDIA; en Ubuntu: sudo apt install nvidia-cuda-toolkit
nvcc -O3 -o bin/gradiente_cuda nativo/cuda/gradiente_servidor.cu
nvcc -O3 -o bin/red_cuda nativo/cuda/red/red.cu
./bin/gradiente_cuda --info                   # debe mostrar tu tarjeta
```

Más detalle: [nativo/cuda/README.md](../../nativo/cuda/README.md).

## Antes de entrenar con MPI

- El maestro entra por SSH a los nodos sin contraseña: `ssh-keygen` (una vez) y `ssh-copy-id usuario@192.168.50.X` por cada nodo.
- Cable en la zona `trusted` ([switch](../switch/nodo.md)) para que pasen los puertos de MPI.
- Solo con otras laptops Linux: no se mezcla con Windows. Más: [fase 6](../desarrollo/guias/fase6.md).

## Si algo falla

- **`go.mod requires go >= ...`**: instala Go desde https://go.dev/dl/.
- **8 GB de RAM**: cierra el navegador pesado mientras limpia.
