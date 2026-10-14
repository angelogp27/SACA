# SACA — frontend (Parte D)

SPA en React + Vite con cuatro paneles:

| Panel | Datos | Origen |
|---|---|---|
| Mapa de estaciones | Todas las estaciones de 2025, coloreadas por riesgo histórico (percentil 98 horario / umbral AQI 51) | `public/datos/estaciones.json` |
| Serie horaria | NO2 observado de un día real (el de más horas sobre el umbral en la red) y la probabilidad de cruce que da el modelo entrenado | `public/datos/series.json` |
| Rendimiento | Speedup, eficiencia y Amdahl del cargador (Experimento 1) y del entrenamiento (Experimento 1b) | `public/datos/experimento1.json`, `experimento1b.json` |
| Clúster | Maqueta; la telemetría por WebSocket llega en la semana 11 | `src/data/mock.js` |

Los JSON los genera el backend:

```sh
# desde la raíz del repo, con MongoDB cargado
go run ./entrenamiento/cmd/secuencial -modelo data/modelo.json
go run ./herramientas/cmd/exportar
# los de rendimiento los escriben scripts/experimento1(.ps1|.sh) y experimento1b
```

Si un JSON no existe, el panel correspondiente usa los datos de ejemplo de `src/data/mock.js` y lo indica en su subtítulo. Cuando exista la API (Parte C), `src/data/datosReales.js` es el único archivo que hay que cambiar.

```sh
npm install
npm run dev     # desarrollo
npm run build   # producción en dist/
npm run lint
```
