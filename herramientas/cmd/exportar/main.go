// Exporta datos reales para el frontend (Parte D) mientras no exista la
// API (Parte C): lee la colección de registros procesados y el resumen
// anual por monitor de la EPA, y escribe JSON estáticos en
// frontend/public/datos:
//
//   - estaciones.json: cada estación con su ubicación, nombre local y un
//     índice de riesgo histórico (percentil 98 horario de cada gas dividido
//     por su umbral de AQI 51; el mayor manda).
//   - series.json: las 24 horas de un día real por estación (el día con más
//     horas de NO2 sobre el umbral en toda la red), con la probabilidad que
//     da el modelo entrenado de cruzar el umbral una hora después.
//   - resumen.json: totales del conjunto de datos y del modelo.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"proyecto9/internal/dataset"
	"proyecto9/internal/persistencia"
	"proyecto9/internal/registro"
)

// Umbrales de AQI 51 por contaminante (.context/proyecto9.md §4). Para O3
// y CO el estándar es sobre promedios de 8 horas; aquí se comparan con el
// percentil 98 horario como aproximación, y así se documenta en la SPA.
var gases = []struct {
	Campo, Nombre, Unidad string
	Umbral                float64
}{
	{"no2_ppb", "NO2", "ppb", 53},
	{"o3_ppm", "O3", "ppm", 0.054},
	{"so2_ppb", "SO2", "ppb", 35},
	{"co_ppm", "CO", "ppm", 4.4},
}

type Gas struct {
	Horas  int     `json:"horas"`
	P98    float64 `json:"p98"`
	Maximo float64 `json:"maximo"`
	Indice float64 `json:"indice"`
}

type Estacion struct {
	ID           string          `json:"id"`
	Nombre       string          `json:"nombre"`
	Ciudad       string          `json:"ciudad,omitempty"`
	Condado      string          `json:"condado"`
	Estado       string          `json:"estado"`
	CBSA         string          `json:"cbsa,omitempty"`
	Lat          float64         `json:"lat"`
	Lon          float64         `json:"lon"`
	Registros    int             `json:"registros"`
	Gases        map[string]*Gas `json:"gases"`
	Indice       float64         `json:"indice"`
	Contaminante string          `json:"contaminante"`
}

type Serie struct {
	Fecha        string     `json:"fecha"`
	Contaminante string     `json:"contaminante"`
	Unidad       string     `json:"unidad"`
	Umbral       float64    `json:"umbral"`
	Observado    []*float64 `json:"observado"`
	Probabilidad []*float64 `json:"probabilidad,omitempty"`
}

func main() {
	mongoURI := flag.String("mongo-uri", "mongodb://localhost:27017", "cadena de conexión a MongoDB")
	baseDatos := flag.String("db", "proyecto9", "nombre de la base de datos")
	rutaAnual := flag.String("anual", "data/raw/annual_conc_by_monitor_2025.csv", "resumen anual por monitor de la EPA")
	rutaModelo := flag.String("modelo", "data/modelo.json", "modelo entrenado (vacío para no calcular probabilidades)")
	rutaManifiesto := flag.String("manifiesto", "data/manifiesto.json", "manifiesto de los CSV crudos")
	fecha := flag.String("fecha", "", "día UTC (AAAA-MM-DD) para series.json; vacío = el de más horas sobre el umbral de NO2")
	salida := flag.String("salida", "frontend/public/datos", "directorio de salida")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	cliente, err := mongo.Connect(options.Client().ApplyURI(*mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	defer cliente.Disconnect(ctx)
	col := cliente.Database(*baseDatos).Collection(persistencia.Coleccion)
	if err := dataset.AsegurarIndice(ctx, col); err != nil {
		log.Fatal(err)
	}

	inicio := time.Now()
	estaciones, err := resumirEstaciones(ctx, col)
	if err != nil {
		log.Fatalf("resumiendo estaciones: %v", err)
	}
	log.Printf("%d estaciones resumidas (%s)", len(estaciones), time.Since(inicio).Round(time.Millisecond))

	if meta, err := leerMetadatos(*rutaAnual); err != nil {
		log.Printf("sin metadatos del resumen anual (%v); se usan condado y estado como nombre", err)
	} else {
		for _, e := range estaciones {
			if m, ok := meta[e.ID]; ok {
				e.Nombre, e.Ciudad, e.CBSA = m.nombre, m.ciudad, m.cbsa
			}
		}
	}
	for _, e := range estaciones {
		if e.Nombre == "" {
			e.Nombre = e.Condado + ", " + e.Estado
		}
	}

	var modelo *dataset.ModeloGuardado
	if *rutaModelo != "" {
		if modelo, err = dataset.CargarModelo(*rutaModelo); err != nil {
			log.Printf("sin modelo (%v); series.json no tendrá probabilidades", err)
		}
	}

	dia := *fecha
	if dia == "" {
		if dia, err = diaConMasExcedencias(ctx, col); err != nil {
			log.Fatalf("eligiendo el día: %v", err)
		}
	}
	series, err := construirSeries(ctx, col, estaciones, dia, modelo)
	if err != nil {
		log.Fatalf("construyendo series: %v", err)
	}
	log.Printf("series del %s para %d estaciones", dia, len(series))

	resumen := map[string]any{
		"generado":         time.Now().UTC().Format(time.RFC3339),
		"estaciones":       len(estaciones),
		"dia_series":       dia,
		"umbrales_aqi_51":  gases,
		"nota_umbrales":    "O3 y CO tienen estándar de 8 horas; el índice usa el percentil 98 horario como aproximación.",
		"registros_unicos": totalRegistros(estaciones),
	}
	if m, err := os.ReadFile(*rutaManifiesto); err == nil {
		var manifiesto map[string]any
		if json.Unmarshal(m, &manifiesto) == nil {
			resumen["filas_crudas"] = manifiesto["total_filas_horarias"]
		}
	}
	if modelo != nil {
		resumen["modelo"] = map[string]any{
			"horizonte_horas": modelo.HorizonteHoras, "umbral_no2_ppb": modelo.UmbralNO2,
			"f1": modelo.F1, "auc": modelo.AUC, "precision": modelo.Precision, "exhaustividad": modelo.Exhaustividad,
		}
	}

	lista := make([]*Estacion, 0, len(estaciones))
	for _, e := range estaciones {
		lista = append(lista, e)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Indice > lista[j].Indice })

	for nombre, v := range map[string]any{"estaciones.json": lista, "series.json": series, "resumen.json": resumen} {
		if err := escribirJSON(filepath.Join(*salida, nombre), v); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("JSON escritos en %s", *salida)
}

// resumirEstaciones agrupa los 10.8 M de registros por estación en MongoDB
// (no en Go): conteo de horas, percentil 98 y máximo de cada gas.
func resumirEstaciones(ctx context.Context, col *mongo.Collection) (map[string]*Estacion, error) {
	grupo := bson.D{
		{Key: "_id", Value: "$estacion_id"},
		{Key: "lat", Value: bson.D{{Key: "$first", Value: "$latitud"}}},
		{Key: "lon", Value: bson.D{{Key: "$first", Value: "$longitud"}}},
		{Key: "estado", Value: bson.D{{Key: "$first", Value: "$estado"}}},
		{Key: "condado", Value: bson.D{{Key: "$first", Value: "$condado"}}},
		{Key: "registros", Value: bson.D{{Key: "$sum", Value: 1}}},
	}
	for _, g := range gases {
		campo := "$" + g.Campo
		grupo = append(grupo,
			bson.E{Key: g.Campo + "_horas", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$type", Value: campo}}, "double"}}}, 1, 0}}}}}},
			bson.E{Key: g.Campo + "_max", Value: bson.D{{Key: "$max", Value: campo}}},
			bson.E{Key: g.Campo + "_p98", Value: bson.D{{Key: "$percentile", Value: bson.D{
				{Key: "input", Value: campo}, {Key: "p", Value: bson.A{0.98}}, {Key: "method", Value: "approximate"},
			}}}},
		)
	}

	cursor, err := col.Aggregate(ctx, mongo.Pipeline{{{Key: "$group", Value: grupo}}}, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	estaciones := map[string]*Estacion{}
	for cursor.Next(ctx) {
		var d bson.M
		if err := cursor.Decode(&d); err != nil {
			return nil, err
		}
		e := &Estacion{
			ID:        d["_id"].(string),
			Lat:       numero(d["lat"]),
			Lon:       numero(d["lon"]),
			Estado:    texto(d["estado"]),
			Condado:   texto(d["condado"]),
			Registros: int(numero(d["registros"])),
			Gases:     map[string]*Gas{},
		}
		for _, g := range gases {
			horas := int(numero(d[g.Campo+"_horas"]))
			if horas == 0 {
				continue
			}
			p98 := 0.0
			if arr, ok := d[g.Campo+"_p98"].(bson.A); ok && len(arr) > 0 {
				p98 = numero(arr[0])
			}
			gas := &Gas{Horas: horas, P98: redondear(p98, 4), Maximo: redondear(numero(d[g.Campo+"_max"]), 4), Indice: redondear(p98/g.Umbral, 3)}
			e.Gases[g.Nombre] = gas
			if gas.Indice > e.Indice {
				e.Indice, e.Contaminante = gas.Indice, g.Nombre
			}
		}
		estaciones[e.ID] = e
	}
	return estaciones, cursor.Err()
}

// diaConMasExcedencias devuelve el día UTC con más horas de NO2 sobre el
// umbral en toda la red: el más interesante para mostrar al modelo.
func diaConMasExcedencias(ctx context.Context, col *mongo.Collection) (string, error) {
	cursor, err := col.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "no2_ppb", Value: bson.D{{Key: "$gt", Value: dataset.UmbralNO2}}}}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$substrCP", Value: bson.A{"$timestamp_utc", 0, 10}}}}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "n", Value: -1}}}},
		{{Key: "$limit", Value: 1}},
	})
	if err != nil {
		return "", err
	}
	var res []bson.M
	if err := cursor.All(ctx, &res); err != nil {
		return "", err
	}
	if len(res) == 0 {
		return "", fmt.Errorf("ninguna hora supera el umbral de NO2")
	}
	log.Printf("día con más horas de NO2 > %.0f ppb: %s (%v horas en la red)", dataset.UmbralNO2, res[0]["_id"], res[0]["n"])
	return res[0]["_id"].(string), nil
}

func construirSeries(ctx context.Context, col *mongo.Collection, estaciones map[string]*Estacion, dia string, modelo *dataset.ModeloGuardado) (map[string]*Serie, error) {
	cursor, err := col.Find(ctx, bson.D{{Key: "timestamp_utc", Value: bson.D{{Key: "$gte", Value: dia}, {Key: "$lt", Value: dia + "U"}}}})
	if err != nil {
		return nil, err
	}
	var registros []registro.RegistroProcesado
	if err := cursor.All(ctx, &registros); err != nil {
		return nil, err
	}

	series := map[string]*Serie{}
	for i := range registros {
		r := &registros[i]
		e, ok := estaciones[r.EstacionID]
		if !ok {
			continue
		}
		s, ok := series[r.EstacionID]
		if !ok {
			// Con NO2 se muestra NO2 (el gas que predice el modelo); si no, el
			// gas dominante del índice de la estación.
			nombre := e.Contaminante
			if _, tieneNO2 := e.Gases["NO2"]; tieneNO2 {
				nombre = "NO2"
			}
			s = &Serie{Fecha: dia, Contaminante: nombre, Observado: make([]*float64, 24)}
			for _, g := range gases {
				if g.Nombre == nombre {
					s.Unidad, s.Umbral = g.Unidad, g.Umbral
				}
			}
			if nombre == "NO2" && modelo != nil {
				s.Probabilidad = make([]*float64, 24)
			}
			series[r.EstacionID] = s
		}

		t, err := time.Parse(time.RFC3339, r.TimestampUTC)
		if err != nil {
			continue
		}
		h := t.Hour()
		var valor *float64
		switch s.Contaminante {
		case "NO2":
			valor = r.NO2Ppb
		case "O3":
			valor = r.O3Ppm
		case "SO2":
			valor = r.SO2Ppb
		case "CO":
			valor = r.COPpm
		}
		s.Observado[h] = valor
		if s.Probabilidad != nil && r.NO2Ppb != nil {
			p := redondear(modelo.Probabilidad(r), 4)
			s.Probabilidad[h] = &p
		}
	}
	return series, nil
}

type metadato struct{ nombre, ciudad, cbsa string }

// leerMetadatos toma del resumen anual (una fila por monitor y parámetro)
// el nombre local, la ciudad y el área metropolitana de cada sitio.
func leerMetadatos(ruta string) (map[string]metadato, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lector := csv.NewReader(f)
	encabezado, err := lector.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, nombre := range encabezado {
		col[nombre] = i
	}
	for _, requerida := range []string{"State Code", "County Code", "Site Num", "Local Site Name", "City Name", "CBSA Name"} {
		if _, ok := col[requerida]; !ok {
			return nil, fmt.Errorf("falta la columna %q", requerida)
		}
	}

	meta := map[string]metadato{}
	for {
		fila, err := lector.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		id := fila[col["State Code"]] + "-" + fila[col["County Code"]] + "-" + fila[col["Site Num"]]
		if m, ok := meta[id]; ok && m.nombre != "" {
			continue
		}
		meta[id] = metadato{
			nombre: tituloLegible(fila[col["Local Site Name"]]),
			ciudad: fila[col["City Name"]],
			cbsa:   fila[col["CBSA Name"]],
		}
	}
	return meta, nil
}

// tituloLegible pasa "FAIRHOPE, Alabama" a "Fairhope, Alabama".
func tituloLegible(s string) string {
	palabras := strings.Fields(strings.ToLower(s))
	for i, p := range palabras {
		r := []rune(p)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		palabras[i] = string(r)
	}
	return strings.Join(palabras, " ")
}

func totalRegistros(estaciones map[string]*Estacion) int {
	n := 0
	for _, e := range estaciones {
		n += e.Registros
	}
	return n
}

func numero(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

func texto(v any) string {
	s, _ := v.(string)
	return s
}

func redondear(v float64, decimales int) float64 {
	f := math.Pow(10, float64(decimales))
	return math.Round(v*f) / f
}

func escribirJSON(ruta string, v any) error {
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	datos, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(ruta, append(datos, '\n'), 0o644)
}
