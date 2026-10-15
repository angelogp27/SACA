# SACA — frontend (Parte D)

SPA en React + Vite con dos vistas: **Tablero** (cuatro paneles) y **Presentación** (10 diapositivas para la exposición del avance, con la demo del clúster en vivo). La vista y la diapositiva quedan en la URL (`#p3`).

| Panel | Datos | Origen |
|---|---|---|
| Mapa de estaciones | Todas las estaciones de 2025, coloreadas por riesgo histórico (percentil 98 horario / umbral AQI 51) | `public/datos/estaciones.json` |
| Serie horaria | NO2 observado de un día real (el de más horas sobre el umbral en la red) y la probabilidad de cruce que da el modelo entrenado | `public/datos/series.json` |
| Rendimiento | Speedup, eficiencia y Amdahl del cargador (Experimento 1) y del entrenamiento (Experimento 1b); pestaña Clúster con el Experimento 2 | `public/datos/experimento1.json`, `experimento1b.json`, `experimento2.json`, `experimento2_prueba.json` |
| Clúster en vivo | Nodos conectados al maestro; se eligen con casillas y se lanza el entrenamiento; gráficas por época (cálculo frente a red, cálculo por nodo, speedup, pérdida) y el tema del curso que ocurre | API del maestro en modo servidor (`/api`, eventos en vivo) |

Los JSON los genera el backend:

```sh
# desde la raíz del repo, con MongoDB cargado
go run ./entrenamiento/cmd/secuencial -modelo data/modelo.json
go run ./herramientas/cmd/exportar
# los de rendimiento los escriben scripts/experimento1(.ps1|.sh) y experimento1b
```

Si un JSON no existe, el panel correspondiente usa los datos de ejemplo de `src/data/mock.js` y lo indica en su subtítulo. Cuando exista la API (Parte C), `src/data/datosReales.js` es el único archivo que hay que cambiar.

**Clúster en vivo.** Necesita el maestro en modo servidor en la misma PC (la página llama a `/api`, que Vite reenvía a `127.0.0.1:8080` en `npm run dev` y `npm run preview`):

```sh
# desde la raíz del repo
go run ./entrenamiento/cmd/maestro -web 127.0.0.1:8080 -modo tcp_tailscale -resultado resultados/experimento2.csv
# o, para ensayar en una sola PC con 2 nodos locales:
powershell -ExecutionPolicy Bypass -File scripts/cluster-local.ps1 -Nodos 2 -Web -Muestra data/muestra_prueba.gob
```

Sin el servidor, el panel muestra los resultados guardados del Experimento 2. Detalle para el día de la presentación: [docs/cluster/guia_maestro.md](../docs/cluster/guia_maestro.md), paso 8.

```sh
npm install
npm run dev     # desarrollo
npm run build   # producción en dist/
npm run lint
```
