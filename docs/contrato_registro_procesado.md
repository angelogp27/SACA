# Contrato — registro procesado (Parte A → MongoDB → Parte B)

Define la forma del documento que el cargador escribe en MongoDB después de limpiar y unir los 4 archivos horarios de la EPA. Es la interfaz entre Parte A y todo lo que consume datos limpios (entrenamiento, API).

## Clave de alineación

Los 4 archivos (`hourly_44201` O3, `hourly_42602` NO2, `hourly_42401` SO2, `hourly_42101` CO) se combinan por la clave compuesta `(estacion_id, timestamp_utc)`. Si una estación no mide alguno de los 4 gases en esa hora, el campo correspondiente queda `null`; el registro no se descarta por eso.

## Campos

| Campo | Tipo | Origen (columna AQS) | Notas |
|---|---|---|---|
| `estacion_id` | string | `State Code` + `County Code` + `Site Num` | Formato `"SS-CCC-NNNN"` (p. ej. `"06-037-0002"`), mismo criterio que el ID de sitio de AQS |
| `timestamp_utc` | string (RFC3339) | `Date GMT` + `Time GMT` | Siempre en UTC, nunca hora local — evita ambigüedad de DST entre estaciones de distintos husos |
| `latitud` | float64 | `Latitude` | |
| `longitud` | float64 | `Longitude` | |
| `o3_ppm` | float64, nullable | `hourly_44201` → `Sample Measurement` | Partes por millón |
| `no2_ppb` | float64, nullable | `hourly_42602` → `Sample Measurement` | Partes por billón |
| `so2_ppb` | float64, nullable | `hourly_42401` → `Sample Measurement` | Partes por billón |
| `co_ppm` | float64, nullable | `hourly_42101` → `Sample Measurement` | Partes por millón |
| `estado` | string | `State Name` | |
| `condado` | string | `County Name` | |

Unidades fijas por contaminante según la tabla de umbrales AQI del proyecto (`.context/proyecto9.md` §4). Si `Units of Measure` de una fila no coincide con la unidad esperada, se convierte antes de escribir (ppm↔ppb, factor 1000), nunca se descarta solo por eso.

## Reglas de limpieza (se aplican antes de escribir el registro)

- Se descarta la fila si `Sample Measurement` está vacío o nulo.
- Se descarta la fila si `Qualifier` es un código de **solicitud de exclusión por evento excepcional**: los que empiezan por `R` seguida de una letra en el catálogo de qualifiers de AQS (p. ej. `RS`, `RT` = incendio forestal). Los informativos (`I*`, p. ej. `IT`), los de validación/QA (`V`, `QX`, `MD`, `SX`) y los numéricos (`1`, `6`, …) se conservan: la medición es válida, solo viene anotada. En los datos de 2025 esto descarta 1625 filas de 16 972 287.
- Si la unidad no es ppm ni ppb (no convertible con seguridad) o algún campo numérico/fecha no se puede leer, la fila se descarta y se cuenta por motivo (`sin_medicion`, `qualifier_excluido`, `unidad_desconocida`, `formato_invalido`); los cargadores imprimen ese conteo.
- No se descarta una fila completa por falta de alguno de los otros 3 gases en esa hora — el join es un outer join por `(estacion_id, timestamp_utc)`.

## Ejemplo

```json
{
  "estacion_id": "06-037-0002",
  "timestamp_utc": "2025-03-14T18:00:00Z",
  "latitud": 34.0669,
  "longitud": -118.2273,
  "o3_ppm": 0.041,
  "no2_ppb": 12.3,
  "so2_ppb": null,
  "co_ppm": 0.3,
  "estado": "California",
  "condado": "Los Angeles"
}
```

Este es el documento base. Las variables derivadas (rezagos, medias móviles, seno/coseno de hora y día) se calculan a partir de esta colección en la Parte B, no se almacenan aquí.
