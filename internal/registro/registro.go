// Package registro define el esquema del documento que el cargador escribe
// en MongoDB, según docs/contrato_registro_procesado.md.
package registro

// RegistroProcesado es el resultado de limpiar y unir por (estacion_id,
// timestamp_utc) los cuatro archivos horarios de la EPA. Los campos de
// contaminante son punteros porque una estación no siempre mide los cuatro
// gases en la misma hora.
type RegistroProcesado struct {
	EstacionID   string   `bson:"estacion_id" json:"estacion_id"`
	TimestampUTC string   `bson:"timestamp_utc" json:"timestamp_utc"`
	Latitud      float64  `bson:"latitud" json:"latitud"`
	Longitud     float64  `bson:"longitud" json:"longitud"`
	O3Ppm        *float64 `bson:"o3_ppm" json:"o3_ppm"`
	NO2Ppb       *float64 `bson:"no2_ppb" json:"no2_ppb"`
	SO2Ppb       *float64 `bson:"so2_ppb" json:"so2_ppb"`
	COPpm        *float64 `bson:"co_ppm" json:"co_ppm"`
	Estado       string   `bson:"estado" json:"estado"`
	Condado      string   `bson:"condado" json:"condado"`

	// POC es el número de monitor (Parameter Occurrence Code de AQS) del
	// que salió cada gas, en el orden O3, NO2, SO2, CO. Solo lo usa el
	// cargador para elegir entre monitores repetidos; no se guarda en
	// MongoDB.
	POC [4]uint8 `bson:"-" json:"-"`
}
