# Usar la página

Igual por switch o por VPN. Abre SACA (`scripts\saca.ps1` en Windows, `bash scripts/saca.sh` en Linux): se abre **http://127.0.0.1:8090**.

## 1. Armar el clúster

- **Una** laptop pulsa **Iniciar clúster**: es el maestro (y también calcula como nodo).
- Las demás pulsan **Unirse al clúster** (por VPN, escribiendo la IP del maestro). Hasta 8 laptops.

Arriba a la izquierda dice si eres *Maestro* (con la IP a la que se unen los demás) o *Nodo*.

## 2. Nuevo y Resultados

Arriba: **Nuevo** / **Resultados** y las fases **Limpieza e ingeniería de features → Entrenamiento → Predicción distribuida → Resultados finales** (verde terminada, amarillo en curso, gris pendiente). Toca una fase para abrirla.

- **Nuevo**: lo que se elige antes de empezar. Solo el maestro tiene los botones; los nodos ven lo mismo sin ellos.
- **Resultados**: la fase en vivo, **Descargar CSV** y, debajo, el **historial** de esa fase (toca una fila para verla como terminó). Cuando el maestro inicia algo, todas las laptops pasan solas aquí.

## 3. Las fases

| Fase | En Nuevo (maestro) | En Resultados |
|---|---|---|
| **Limpieza e ingeniería de features** | **Iniciar limpieza**: cada laptop limpia sus 4 CSV con Pthreads y con Go y genera su `muestra.bin`. Si ya limpiaron: **Saltar limpieza** (solo comprueba que todos tengan el mismo archivo) | Por nodo y tecnología: % de CPU y tiempo de cada fase |
| **Entrenamiento** | Modelo (▶ = video), épocas, mini-lotes, comunicación (árbol, anillo o todo por el maestro) y reparto (dinámico, según la capacidad o partes iguales). Toca un nodo para quitarlo o sumarlo. **Iniciar entrenamiento** | Un cuadro por época (más oscuro = menor pérdida) y los nodos de tamaño según su parte de los datos |
| **Predicción distribuida** | **Iniciar predicción** (con el último modelo entrenado) | Filas, MB y tiempos de cada nodo |

**Resultados finales** (también arriba a la derecha): el mapa de estaciones, el NO2 con la alerta del modelo y el último proceso de cada fase.

## 4. Dónde quedan los resultados

En la laptop del maestro, carpeta `resultados/`: `limpieza.csv`, `cluster.csv` (entrenamientos), `transferencias.csv` (MB por la red) e `historial/` (lo que muestra la página). Los CSV de *Descargar CSV* se bajan a la carpeta de descargas de quien los pide.

## Si algo falla

- **Un nodo con otra huella** (otro `muestra.bin`): en esa laptop, `git pull`, vuelve a abrir SACA y que el maestro haga la limpieza. Al entrenar, los nodos con otra huella se quitan solos.
- **"El maestro no tiene muestra.bin"**: incluye al maestro en la limpieza o cópiale el `muestra.bin` de otra laptop.
- **Cambiar de maestro**: el actual pulsa *Apagar clúster*; el nuevo, *Iniciar clúster*; los demás, *Salir del clúster* y *Unirse*.
- **Se perdió la conexión con el maestro**: los nodos se reconectan solos cuando vuelve.
