# Clúster de SACA: empieza aquí

Vamos a unir las computadoras del grupo, cada una desde su casa, para que **entrenen juntas** el modelo del proyecto. No necesitas entender el código: solo sigue **la guía de tu rol**, paso a paso, copiando y pegando los comandos.

## 1. ¿Qué guía me toca?

| Si tú… | Tu rol | Tu guía |
|---|---|---|
| coordinas la demo y tienes los datos del proyecto (hoy, Piero) | **Maestro** | [guia_maestro.md](guia_maestro.md) |
| tienes una tarjeta gráfica **NVIDIA** (GeForce, RTX, GTX, Quadro) | **Nodo CON GPU NVIDIA** | [guia_nodo_con_nvidia.md](guia_nodo_con_nvidia.md) |
| **no** tienes NVIDIA: tu tarjeta es AMD (Radeon), Intel, integrada o **cualquier Mac** | **Nodo SIN GPU NVIDIA** | [guia_nodo_sin_nvidia.md](guia_nodo_sin_nvidia.md) |

### ¿Cómo sé si tengo una tarjeta NVIDIA?

- **Windows:** abre el Administrador de tareas (`Ctrl` + `Shift` + `Esc`), ve a la pestaña **Rendimiento** y mira los elementos que dicen **GPU 0** y **GPU 1**. Si alguno dice **NVIDIA**, tienes NVIDIA. Si solo aparecen AMD Radeon o Intel, no la tienes.
- **Linux:** escribe `lspci | grep -i nvidia` en la terminal. Si aparece una línea, tienes NVIDIA.
- **Mac:** las Mac no tienen NVIDIA. Usa la guía SIN GPU NVIDIA.

> Muchas laptops tienen **dos** GPU: una Intel o AMD integrada y una NVIDIA. Si una de las dos es NVIDIA, usa la guía CON GPU NVIDIA.

## 2. Cómo funciona, en una figura

```
                      Internet (red privada de Tailscale)
                                    │
       ┌────────────────────────────┼────────────────────────────┐
       │                            │                            │
┌──────┴───────────┐     ┌──────────┴──────────┐     ┌───────────┴─────────┐
│ MAESTRO          │◄────│ NODO SIN NVIDIA     │     │ NODO CON NVIDIA     │
│ (PC de Piero)    │     │ calcula con la CPU  │     │ calcula con la GPU  │
│ reparte el       │◄────┼─────────────────────┼─────│ (CUDA)              │
│ trabajo y junta  │     └─────────────────────┘     └─────────────────────┘
│ los resultados   │
└──────────────────┘
```

1. Cada nodo se conecta al maestro. El maestro le asigna **su parte de los datos** (un grupo de estaciones de monitoreo).
2. En cada **época** (vuelta de entrenamiento), el maestro manda el modelo actual. Cada nodo calcula sobre su parte y le devuelve el resultado.
3. El maestro junta todo, mejora el modelo y repite 200 veces. Al final comprueba que el resultado es **idéntico** al que daría una sola PC.

## 3. Palabras que vas a ver

| Palabra | Qué significa |
|---|---|
| **Terminal** o **PowerShell** | La ventana negra o azul donde se escriben comandos |
| **Maestro** | La PC que coordina. Hay una sola |
| **Nodo** o **trabajador** | Cada PC que aporta su cálculo. Tú eres uno |
| **Época** | Una vuelta de entrenamiento. Por defecto hay 200 |
| **Partición** | La parte de los datos que le toca a cada nodo |
| **Huella** | Un código de 12 letras y números que identifica el archivo de datos. Si tu huella no es igual a la del maestro, tienes otro archivo y el maestro te rechaza |
| **Tailscale** | Un programa gratuito que conecta nuestras PCs como si estuvieran en la misma red, aunque estén en casas distintas |
| **Motor** | Con qué calcula tu nodo: `go` (el procesador) o `cuda` (la tarjeta NVIDIA) |

## 4. Calendario: qué hace cada uno y cuándo

| Momento | Maestro | Cada nodo |
|---|---|---|
| **1. Preparación** (cada uno en su casa, unos 45 min; 1-2 h con NVIDIA) | Sube los datos al Drive, invita a todos en Tailscale y abre el puerto | Instala los programas, se une a Tailscale, descarga el proyecto y los datos |
| **2. Ensayo individual** (5 min) | Prueba el clúster completo en su PC | Prueba el clúster completo en su PC y **envía sus datos al grupo** |
| **3. Ensayo general** (todos juntos por videollamada, 30 min) | Lanza el maestro, mide y guarda los resultados | Se conecta al maestro y repite cuando el maestro lo pida |
| **4. Presentación** | Comparte pantalla y lanza el maestro | Se conecta al maestro y sigue las indicaciones |

## 5. Inventario del grupo

Cada uno completa su fila con lo que pide el paso "Enviar tus datos al grupo" de su guía.

| Integrante | Rol | Sistema | Procesador | RAM | GPU | IP de Tailscale | Huella | Ensayo individual |
|---|---|---|---|---|---|---|---|---|
| Piero | Maestro | Windows 11 | Ryzen 5 3600 | 16 GB | AMD RX 580 | 100.___.___.___ | 5e90db4b1158 | |
| | | | | | | | | |
| | | | | | | | | |
| | | | | | | | | |
| | | | | | | | | |

## 6. Para quien quiera más detalle

- La exposición (guion, demo, preguntas): [../guia_presentacion_cluster.md](../guia_presentacion_cluster.md)
- La arquitectura técnica y MPI: [../guia_cluster.md](../guia_cluster.md)
- Los mensajes que se intercambian: [../contrato_protocolo_tcp.md](../contrato_protocolo_tcp.md)
- El motor CUDA: [../../nativo/cuda/README.md](../../nativo/cuda/README.md)
