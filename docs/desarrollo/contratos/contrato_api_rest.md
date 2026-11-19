# Contrato — API REST + WebSocket (Parte C)

Define las rutas que expone el servicio Go de la Parte C. Es la interfaz entre el backend y la SPA (Parte D).

## Autenticación

- JWT por header `Authorization: Bearer <token>`.
- Dos roles: `lector` (solo consulta) y `operador` (además puede lanzar y gestionar entrenamientos).
- `POST /auth/login` — recibe credenciales, devuelve el JWT. El mecanismo de emisión de usuarios/roles queda por definir cuando se implemente esta ruta; no bloquea el resto del contrato.

## Rutas REST

| Método | Ruta | Rol mínimo | Descripción |
|---|---|---|---|
| GET | `/health` | ninguno | Liveness check, `{"status":"ok"}` |
| POST | `/auth/login` | ninguno | Devuelve JWT |
| GET | `/predicciones?estacion_id=&horizonte_horas=` | `lector` | Predicción de cruce de umbral AQI 51 para esa estación y horizonte (1-24h) |
| POST | `/entrenamientos` | `operador` | Lanza un entrenamiento nuevo; body con `modo_sincronizacion` (`sincrono`\|`asincrono`) y `num_nodos_esperados` |
| GET | `/entrenamientos/{id}` | `lector` | Estado del entrenamiento: época actual, pérdida, si terminó |
| GET | `/entrenamientos/{id}/metricas` | `lector` | Precisión, exhaustividad, F1, AUC-ROC una vez terminado |

## Ejemplos

`GET /predicciones?estacion_id=06-037-0002&horizonte_horas=6`

```json
{
  "estacion_id": "06-037-0002",
  "horizonte_horas": 6,
  "probabilidad": 0.73,
  "alerta": true,
  "contaminante": "o3",
  "calculado_en": "2025-03-14T18:00:00Z"
}
```

`POST /entrenamientos`

```json
{ "modo_sincronizacion": "sincrono", "num_nodos_esperados": 4 }
```

respuesta:

```json
{ "entrenamiento_id": "ent-20250314-01", "estado": "iniciado" }
```

## WebSocket — `/ws/telemetria`

Requiere JWT igual que las rutas REST (token en el query string o en el primer mensaje de handshake, a definir en implementación). El servidor empuja eventos según ocurren, sin que el cliente pida nada:

```json
{ "tipo": "perdida_epoca", "entrenamiento_id": "ent-20250314-01", "epoca": 7, "perdida": 0.41 }
```

```json
{ "tipo": "telemetria_nodo", "worker_id": "w1", "cpu_pct": 62.3, "ram_mb": 512 }
```

```json
{ "tipo": "alerta_nueva", "estacion_id": "06-037-0002", "contaminante": "o3", "probabilidad": 0.81 }
```

## Cache

Las respuestas de `GET /predicciones` se cachean en Redis por `(estacion_id, horizonte_horas)`; el objetivo de latencia p95 es <100ms con cache activa.
