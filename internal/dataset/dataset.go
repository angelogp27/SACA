// Package dataset arma el conjunto de entrenamiento de la Parte B a partir
// de la colección de registros procesados: sortea una muestra, etiqueta
// cada registro con la lectura de la misma estación horizonte horas más
// tarde y la convierte en vectores de features. Lo comparten el
// entrenamiento secuencial, el paralelo y (más adelante) los nodos
// trabajadores, para que todos entrenen exactamente con los mismos datos.
package dataset

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"proyecto9/internal/registro"
)

// UmbralNO2 es el corte de AQI 51 para NO2 en 1 hora (> 53 ppb), según la
// tabla de .context/proyecto9.md §4.
const UmbralNO2 = 53.0

// NombresFeatures documenta el orden del vector completo (muestra.bin v2
// con rezagos): las 12 variables de VectorFeatures y, al final, las 8 de
// VariablesRezago. La muestra antigua (muestra.gob) usa solo las 12
// primeras.
var NombresFeatures = []string{
	"no2_ppb", "o3_ppm", "so2_ppb", "co_ppm", "latitud", "longitud",
	"sin_hora", "cos_hora", "sin_dia_semana", "cos_dia_semana", "sin_mes", "cos_mes",
	"no2_t1", "no2_t2", "no2_t3", "no2_t6", "no2_t12", "no2_t24", "no2_media24", "no2_max24",
}

// NumBase es el número de variables de VectorFeatures.
const NumBase = 12

// Rezagos del NO2 que pide el enunciado (horas antes de t), y la ventana
// de la media y el máximo móviles.
var Rezagos = []int{1, 2, 3, 6, 12, 24}

const VentanaHoras = 24

// Historia devuelve el NO2 de la misma estación horasAtras horas antes de
// la lectura actual, y false si esa hora no se midió.
type Historia func(horasAtras int) (float64, bool)

// VariablesRezago calcula las 8 variables de la serie propia del NO2:
// t-1, t-2, t-3, t-6, t-12 y t-24, y la media y el máximo de las 24 horas
// anteriores. Una hora sin medición toma el valor actual (persistencia);
// la media y el máximo usan solo las horas medidas y, si no hay ninguna,
// el valor actual. El cargador de Pthreads aplica exactamente la misma
// regla, en el mismo orden de suma (de t-1 a t-24), para que muestra.bin
// salga idéntico.
func VariablesRezago(actual float64, h Historia) []float64 {
	x := make([]float64, 0, len(Rezagos)+2)
	for _, k := range Rezagos {
		v, ok := h(k)
		if !ok {
			v = actual
		}
		x = append(x, v)
	}
	suma, n, maximo := 0.0, 0, 0.0
	for k := 1; k <= VentanaHoras; k++ {
		v, ok := h(k)
		if !ok {
			continue
		}
		if n == 0 || v > maximo {
			maximo = v
		}
		suma += v
		n++
	}
	if n == 0 {
		return append(x, actual, actual)
	}
	return append(x, suma/float64(n), maximo)
}

// VectorCompleto son las 20 variables de una lectura con NO2.
func VectorCompleto(r *registro.RegistroProcesado, h Historia) []float64 {
	return append(VectorFeatures(r), VariablesRezago(*r.NO2Ppb, h)...)
}

// Muestra es un conjunto de ejemplos etiquetados. EstacionID y Mes se
// guardan junto a cada ejemplo para poder particionar por estación (Parte
// B distribuida) y por tiempo (evaluación honesta).
type Muestra struct {
	X          [][]float64
	Y          []float64
	EstacionID []string
	Mes        []int
	// Hora de cada ejemplo (horas desde 1970-01-01 UTC); solo en la muestra
	// leída de muestra.bin (sirve para la predicción distribuida).
	Hora []int64
}

func (m *Muestra) Len() int { return len(m.Y) }

func (m *Muestra) agregar(x []float64, y float64, estacion string, mes int) {
	m.X = append(m.X, x)
	m.Y = append(m.Y, y)
	m.EstacionID = append(m.EstacionID, estacion)
	m.Mes = append(m.Mes, mes)
}

// DividirPorMes separa la muestra en entrenamiento (meses < mesCorte) y
// prueba (meses >= mesCorte): nunca se entrena con el futuro de lo que se
// evalúa.
func (m *Muestra) DividirPorMes(mesCorte int) (entrenamiento, prueba *Muestra) {
	entrenamiento, prueba = &Muestra{}, &Muestra{}
	for i := range m.Y {
		destino := entrenamiento
		if m.Mes[i] >= mesCorte {
			destino = prueba
		}
		destino.agregar(m.X[i], m.Y[i], m.EstacionID[i], m.Mes[i])
	}
	return entrenamiento, prueba
}

// Positivos cuenta los ejemplos con etiqueta 1.
func (m *Muestra) Positivos() int {
	n := 0
	for _, v := range m.Y {
		if v == 1 {
			n++
		}
	}
	return n
}

// Vector de features por registro. No incluye los rezagos (t-1..t-24) ni
// las medias/máximos móviles de 24h del enunciado completo: eso exige
// recorrer la serie ordenada por estación, y queda pendiente para cuando
// se integre con el pipeline real de entrenamiento. Tampoco incluye tipo
// de zona urbana (requiere el archivo annual_conc_by_monitor). Aquí solo
// entra lo que ya vive en el registro procesado: concentración actual del
// contaminante objetivo y de los otros 3 gases, posición geográfica, y
// estacionalidad cíclica de hora/día/mes.
func VectorFeatures(r *registro.RegistroProcesado) []float64 {
	valor := func(p *float64) float64 {
		if p == nil {
			return 0
		}
		return *p
	}

	t, _ := time.Parse(time.RFC3339, r.TimestampUTC)
	hora := float64(t.Hour())
	diaSemana := float64(t.Weekday())
	mes := float64(t.Month())

	return []float64{
		valor(r.NO2Ppb),
		valor(r.O3Ppm),
		valor(r.SO2Ppb),
		valor(r.COPpm),
		r.Latitud,
		r.Longitud,
		math.Sin(2 * math.Pi * hora / 24),
		math.Cos(2 * math.Pi * hora / 24),
		math.Sin(2 * math.Pi * diaSemana / 7),
		math.Cos(2 * math.Pi * diaSemana / 7),
		math.Sin(2 * math.Pi * mes / 12),
		math.Cos(2 * math.Pi * mes / 12),
	}
}

// AsegurarIndice crea el índice (estacion_id, timestamp_utc) que necesita
// la búsqueda de la lectura futura.
func AsegurarIndice(ctx context.Context, coleccion *mongo.Collection) error {
	_, err := coleccion.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "estacion_id", Value: 1}, {Key: "timestamp_utc", Value: 1}},
	})
	return err
}

// Construir sortea n registros que tienen NO2 con $sample y etiqueta cada uno con el NO2
// de la misma estación horizonteHoras más tarde (1 si supera UmbralNO2).
// Con n <= 0 usa todos los registros con NO2, sin sortear: es la muestra
// completa del clúster. Los registros sin NO2, o sin lectura futura, se
// descartan porque no se pueden etiquetar.
//
// Las lecturas futuras se piden en lotes de tamanoLote con un $or sobre el
// índice compuesto, en vez de un FindOne por registro: con 200 000
// registros son 200 consultas en lugar de 200 000 viajes a la base.
func Construir(ctx context.Context, coleccion *mongo.Collection, n, horizonteHoras int) (*Muestra, error) {
	const tamanoLote = 1000

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "no2_ppb", Value: bson.D{{Key: "$ne", Value: nil}}}}}},
	}
	if n > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: n}}}})
	}
	cursor, err := coleccion.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	muestra := &Muestra{}
	lote := make([]registro.RegistroProcesado, 0, tamanoLote)
	leidos, inicio := 0, time.Now()
	for cursor.Next(ctx) {
		var r registro.RegistroProcesado
		if err := cursor.Decode(&r); err != nil {
			return nil, err
		}
		lote = append(lote, r)
		if len(lote) == tamanoLote {
			if err := etiquetarLote(ctx, coleccion, lote, horizonteHoras, muestra); err != nil {
				return nil, err
			}
			lote = lote[:0]
			if leidos += tamanoLote; leidos%100000 == 0 {
				log.Printf("muestra: %d registros leídos, %d etiquetados (%s)", leidos, muestra.Len(), time.Since(inicio).Round(time.Second))
			}
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	if len(lote) > 0 {
		if err := etiquetarLote(ctx, coleccion, lote, horizonteHoras, muestra); err != nil {
			return nil, err
		}
	}
	return muestra, nil
}

func etiquetarLote(ctx context.Context, coleccion *mongo.Collection, lote []registro.RegistroProcesado, horizonteHoras int, muestra *Muestra) error {
	claveFutura := make([]string, len(lote))
	condiciones := make(bson.A, 0, len(lote))
	for i, r := range lote {
		t, err := time.Parse(time.RFC3339, r.TimestampUTC)
		if err != nil {
			continue
		}
		futura := t.Add(time.Duration(horizonteHoras) * time.Hour).Format(time.RFC3339)
		claveFutura[i] = r.EstacionID + "|" + futura
		condiciones = append(condiciones, bson.D{{Key: "estacion_id", Value: r.EstacionID}, {Key: "timestamp_utc", Value: futura}})
	}

	cursor, err := coleccion.Find(ctx, bson.D{{Key: "$or", Value: condiciones}})
	if err != nil {
		return err
	}
	var futuros []registro.RegistroProcesado
	if err := cursor.All(ctx, &futuros); err != nil {
		return err
	}
	no2Futuro := make(map[string]float64, len(futuros))
	for _, f := range futuros {
		if f.NO2Ppb != nil {
			no2Futuro[f.EstacionID+"|"+f.TimestampUTC] = *f.NO2Ppb
		}
	}

	for i := range lote {
		valor, ok := no2Futuro[claveFutura[i]]
		if !ok {
			continue
		}
		etiqueta := 0.0
		if valor > UmbralNO2 {
			etiqueta = 1
		}
		t, _ := time.Parse(time.RFC3339, lote[i].TimestampUTC)
		muestra.agregar(VectorFeatures(&lote[i]), etiqueta, lote[i].EstacionID, int(t.Month()))
	}
	return nil
}

// Normalizador estandariza cada feature con la media y desviación del
// conjunto de entrenamiento; las mismas constantes se aplican después al
// de prueba para no filtrar información del futuro.
type Normalizador struct {
	Medias       []float64 `json:"medias"`
	Desviaciones []float64 `json:"desviaciones"`
}

func AjustarNormalizador(X [][]float64) Normalizador {
	numFeatures := len(X[0])
	medias := make([]float64, numFeatures)
	for _, x := range X {
		for j, v := range x {
			medias[j] += v
		}
	}
	for j := range medias {
		medias[j] /= float64(len(X))
	}

	desviaciones := make([]float64, numFeatures)
	for _, x := range X {
		for j, v := range x {
			d := v - medias[j]
			desviaciones[j] += d * d
		}
	}
	for j := range desviaciones {
		desviaciones[j] = math.Sqrt(desviaciones[j] / float64(len(X)))
		if desviaciones[j] == 0 {
			desviaciones[j] = 1
		}
	}
	return Normalizador{Medias: medias, Desviaciones: desviaciones}
}

// Aplicar normaliza X en el lugar.
func (n Normalizador) Aplicar(X [][]float64) {
	for _, x := range X {
		for j := range x {
			x[j] = (x[j] - n.Medias[j]) / n.Desviaciones[j]
		}
	}
}

// Guardar y Cargar persisten la muestra en disco (encoding/gob), para que
// los experimentos de entrenamiento midan solo el cómputo y no la
// construcción de la muestra contra MongoDB.
func (m *Muestra) Guardar(ruta string) error {
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	f, err := os.Create(ruta)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(m); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func Cargar(ruta string) (*Muestra, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var m Muestra
	if err := gob.NewDecoder(f).Decode(&m); err != nil {
		return nil, fmt.Errorf("leyendo la caché %s: %w", ruta, err)
	}
	return &m, nil
}

// CargarOConstruir usa la caché si existe; si no, construye la muestra
// desde MongoDB y la guarda en rutaCache (si no es vacía).
func CargarOConstruir(ctx context.Context, coleccion func() (*mongo.Collection, error), rutaCache string, n, horizonteHoras int) (*Muestra, bool, error) {
	if rutaCache != "" {
		if m, err := Cargar(rutaCache); err == nil {
			return m, true, nil
		}
	}
	col, err := coleccion()
	if err != nil {
		return nil, false, err
	}
	if err := AsegurarIndice(ctx, col); err != nil {
		return nil, false, fmt.Errorf("creando el indice: %w", err)
	}
	m, err := Construir(ctx, col, n, horizonteHoras)
	if err != nil {
		return nil, false, err
	}
	if rutaCache != "" {
		if err := m.Guardar(rutaCache); err != nil {
			return nil, false, err
		}
	}
	return m, false, nil
}

// ModeloGuardado es el modelo entrenado listo para predecir fuera del
// proceso que lo entrenó (exportador de datos del frontend y, más
// adelante, la API): pesos, sesgo y las constantes de normalización.
type ModeloGuardado struct {
	Features       []string     `json:"features"`
	Pesos          []float64    `json:"pesos"`
	Sesgo          float64      `json:"sesgo"`
	Normalizador   Normalizador `json:"normalizador"`
	HorizonteHoras int          `json:"horizonte_horas"`
	UmbralNO2      float64      `json:"umbral_no2_ppb"`
	Modo           string       `json:"modo"`
	Epocas         int          `json:"epocas"`
	F1             float64      `json:"f1_prueba"`
	AUC            float64      `json:"auc_prueba"`
	Precision      float64      `json:"precision_prueba"`
	Exhaustividad  float64      `json:"exhaustividad_prueba"`
}

func (m *ModeloGuardado) Guardar(ruta string) error {
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	datos, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ruta, append(datos, '\n'), 0o644)
}

func CargarModelo(ruta string) (*ModeloGuardado, error) {
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return nil, err
	}
	var m ModeloGuardado
	if err := json.Unmarshal(datos, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Probabilidad de que el NO2 de la estación supere el umbral
// HorizonteHoras después de la lectura r. Un modelo con rezagos (más de
// NumBase pesos) necesita la historia de la estación; uno de 12 no la usa.
func (m *ModeloGuardado) Probabilidad(r *registro.RegistroProcesado, h Historia) float64 {
	x := VectorFeatures(r)
	if len(m.Pesos) > NumBase {
		if h == nil || r.NO2Ppb == nil {
			h = func(int) (float64, bool) { return 0, false }
		}
		if r.NO2Ppb != nil {
			x = VectorCompleto(r, h)
		}
	}
	z := m.Sesgo
	for j, v := range x {
		z += m.Pesos[j] * (v - m.Normalizador.Medias[j]) / m.Normalizador.Desviaciones[j]
	}
	return 1 / (1 + math.Exp(-z))
}
