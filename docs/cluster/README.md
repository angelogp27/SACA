# Clúster de SACA: empieza aquí

Varias laptops del grupo trabajan juntas: cada una **limpia** los datos de la EPA en su propia máquina (con Pthreads y con Go, para compararlos), todas comprueban que obtuvieron **exactamente el mismo archivo** de muestra, y luego **entrenan juntas** el modelo de alerta de NO2.

Todo se maneja desde una página. Cada laptop corre un solo comando, el **lanzador** (`scripts/saca.sh` en Linux, `scripts\saca.ps1` en Windows), que compila todo y abre `http://127.0.0.1:8090`. Funciona igual en Linux y en Windows.

## 1. Qué guía seguir

| Paso | Guía |
|---|---|
| 1. Instalar (una vez por laptop): Git, Go, Node, gcc (MSYS2 en Windows), proyecto, CSV | [instalacion.md](instalacion.md) |
| 2a. Todas en la misma mesa, con cable y un **switch** | [switch/](switch/README.md) |
| 2b. Cada una desde su casa, por **Tailscale** (VPN) | [tailscale/](tailscale/README.md) |

Cada guía de red trae sus **casos especiales**: laptop ya configurada, otra huella, sin CSV, sin gcc, cambiar de maestro, etc.

## 2. Las pantallas

1. **Inicio**: *Iniciar clúster* (esta laptop es el maestro; se niega si otra ya lo inició) o *Unirse al clúster* (detecta al maestro en la red local; por Tailscale se escribe su IP).
2. **Clúster iniciado**: los nodos conectados, con su procesador (Ryzen, Intel) y si tienen GPU NVIDIA. *Iniciar limpieza* o *Saltar limpieza* (si ya limpiaron: solo comprueba las huellas).
3. **Limpieza**: por cada nodo, el número de hilos, el % de CPU a lo largo del avance y el tiempo total, primero con **Pthreads** y luego con **Go** (uno tras otro, para no pelear por la CPU). Los nodos que llegan tarde aparecen arriba a la derecha y se pueden sumar. *Parar limpieza* pregunta si guardar los resultados ya completados.
4. **Antes de entrenar**: se eligen los nodos (amarillo = no participa) e *Iniciar entrenamiento* comprueba que todos tengan el mismo `muestra.bin` que el maestro. Después se ve el entrenamiento en vivo.

Solo la laptop del maestro tiene los botones; las demás ven todo en vivo. El clúster admite **hasta 8 laptops**, contando la del maestro; los lugares que quedan se ven como *(Libre)*. Arriba a la izquierda, la laptop del maestro muestra su nombre y la IP a la que deben unirse los demás: la del switch (`192.168.50.x`) si existe; si no, la de Tailscale.

## 3. Cómo funciona

```
 laptop maestro                         cada laptop (también la del maestro)
 ┌───────────────────────────┐          ┌─────────────────────────────────────┐
 │ saca (página :8090)       │          │ saca (página :8090)                 │
 │ maestro  TCP :9000 ◄──────┼──────────┼── trabajador                       │
 │          API :8080 ◄──────┼──────────┼── (la página de esa laptop)        │
 │ anuncio UDP :9001 ────────┼─────────►│   limpia sus CSV → data/muestra.bin │
 └───────────────────────────┘          └─────────────────────────────────────┘
```

- **Limpieza:** cada nodo lee **sus** CSV (`data/raw`) y genera `data/muestra.bin`. Por la red solo viajan avisos, el avance y la huella; nunca los datos.
- **Huella:** el SHA-256 del `muestra.bin`, los primeros 12 caracteres. Los dos cargadores escriben el archivo **byte a byte igual** con cualquier número de hilos, así que todas las laptops deben obtener la misma huella. Si una no coincide, no entrena.
- **Entrenamiento:** el maestro reparte las estaciones entre los nodos; en cada época manda el modelo, cada nodo calcula su gradiente y el maestro los suma.
- **MongoDB** ya no participa en la limpieza ni en el entrenamiento. Solo se usa en la laptop del maestro para **regenerar el mapa** al terminar un entrenamiento (si no está corriendo, el mapa queda como estaba).

## 4. Palabras que vas a ver

| Palabra | Qué significa |
|---|---|
| **Maestro** | La laptop que coordina. Hay una sola: la que pulsó *Iniciar clúster* |
| **Nodo** o **trabajador** | Cada laptop que aporta su cálculo (el maestro también puede serlo) |
| **Limpieza** | Leer los 4 CSV, descartar filas inválidas, unir los 4 gases por estación y hora, y armar la muestra |
| **Huella** | El código de 12 caracteres que identifica el `muestra.bin`: igual huella = mismo archivo |
| **Época** | Una vuelta de entrenamiento (por defecto 200) |

## 5. Inventario del grupo

Cada uno completa su fila al terminar la instalación. La huella es la de su `data/muestra.bin` después de limpiar.

| Integrante | Sistema | Procesador | RAM | GPU | gcc (Pthreads) | CSV | IP switch | IP Tailscale | Huella |
|---|---|---|---|---|---|---|---|---|---|
| Angelo | Fedora | Ryzen 7 7735HS | 13 GB | AMD Radeon (integrada) | sí | sí | 192.168.50.1 | 100.118.152.101 | de62c327dffc |
| Piero | Windows 11 | Ryzen 5 3600 | 16 GB | AMD RX 580 | | | | 100.67.177.8 | |
| | | | | | | | | | |
| | | | | | | | | | |

## 6. Más detalle

- Los mensajes entre el maestro y los nodos: [../contratos/contrato_protocolo_tcp.md](../contratos/contrato_protocolo_tcp.md)
- El formato de `muestra.bin` y por qué sale igual en todos: [../contratos/contrato_formato_binario.md](../contratos/contrato_formato_binario.md)
- La arquitectura del clúster (MPI, OpenMP, CUDA): [arquitectura_cluster.md](arquitectura_cluster.md)
- La exposición: [../presentacion/guia_presentacion_cluster.md](../presentacion/guia_presentacion_cluster.md)
- El motor CUDA: [../../nativo/cuda/README.md](../../nativo/cuda/README.md)
