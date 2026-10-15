// Package entrenador reúne el flujo común de los ejecutables de
// entrenamiento (secuencial y paralelo): flags, preparación de la muestra
// con partición temporal, evaluación del modelo y registro de resultados.
// Así ambos entrenan exactamente con los mismos datos y solo difieren en
// cómo calculan el gradiente.
package entrenador

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"proyecto9/internal/dataset"
	"proyecto9/internal/logistica"
	"proyecto9/internal/metricas"
	"proyecto9/internal/persistencia"
)

// HorizonteHoras fijo en 1 por ahora; el enunciado permite un horizonte
// configurable entre 1 y 24 horas (ver .context/proyecto9.md §4).
const HorizonteHoras = 1

type Config struct {
	MongoURI        string
	BaseDatos       string
	TamanoMuestra   int
	TasaAprendizaje float64
	Epocas          int
	MesCorte        int
	Cache           string
	Resultado       string
	RutaModelo      string
}

// RegistrarFlags declara los flags comunes; el llamador hace flag.Parse.
func RegistrarFlags() *Config {
	c := &Config{}
	flag.StringVar(&c.MongoURI, "mongo-uri", "mongodb://localhost:27017", "cadena de conexión a MongoDB")
	flag.StringVar(&c.BaseDatos, "db", "proyecto9", "nombre de la base de datos")
	flag.IntVar(&c.TamanoMuestra, "muestra", 200000, "registros con NO2 sorteados de MongoDB antes de descartar los que no tienen etiqueta; 0 = todos (la muestra del clúster)")
	flag.Float64Var(&c.TasaAprendizaje, "tasa", 0.1, "tasa de aprendizaje")
	flag.IntVar(&c.Epocas, "epocas", 200, "numero de epocas")
	flag.IntVar(&c.MesCorte, "mes-corte", 10, "los meses >= mes-corte se reservan para evaluar (partición temporal)")
	flag.StringVar(&c.Cache, "cache", "data/muestra_prueba.gob", "caché de la muestra en disco (vacío para no usarla); la del clúster es data/muestra.gob")
	flag.StringVar(&c.Resultado, "resultado", "", "CSV al que se agrega una fila con los tiempos y métricas de esta corrida")
	flag.StringVar(&c.RutaModelo, "modelo", "", "JSON donde guardar el modelo entrenado (pesos y normalización), p. ej. data/modelo.json")
	return c
}

// Datos es la muestra lista para entrenar: normalizada con las constantes
// del conjunto de entrenamiento y con los pesos de clase calculados.
type Datos struct {
	Entrenamiento *dataset.Muestra
	Prueba        *dataset.Muestra
	PesosClase    logistica.PesosClase
	Normalizador  dataset.Normalizador
}

func Preparar(ctx context.Context, c *Config) (*Datos, error) {
	inicio := time.Now()
	coleccion := func() (*mongo.Collection, error) {
		cliente, err := mongo.Connect(options.Client().ApplyURI(c.MongoURI))
		if err != nil {
			return nil, err
		}
		return cliente.Database(c.BaseDatos).Collection(persistencia.Coleccion), nil
	}
	muestra, desdeCache, err := dataset.CargarOConstruir(ctx, coleccion, c.Cache, c.TamanoMuestra, HorizonteHoras)
	if err != nil {
		return nil, err
	}
	origen := "MongoDB"
	if desdeCache {
		origen = "caché " + c.Cache
	}
	log.Printf("muestra: %d ejemplos etiquetados desde %s (%s)", muestra.Len(), origen, time.Since(inicio).Round(time.Millisecond))
	return PrepararDesde(muestra, c.MesCorte)
}

// PrepararDesde aplica la partición temporal, la normalización y los pesos
// de clase a una muestra ya cargada. Es determinista: los nodos del
// entrenamiento distribuido la llaman cada uno con la misma muestra y
// obtienen exactamente la misma normalización, sin comunicarse.
func PrepararDesde(muestra *dataset.Muestra, mesCorte int) (*Datos, error) {
	if muestra.Len() == 0 {
		return nil, fmt.Errorf("no se encontraron ejemplos etiquetables, revisa la coleccion")
	}

	entrenamiento, prueba := muestra.DividirPorMes(mesCorte)
	if entrenamiento.Len() == 0 || prueba.Len() == 0 {
		return nil, fmt.Errorf("la partición temporal dejó un conjunto vacío (entrenamiento=%d, prueba=%d)", entrenamiento.Len(), prueba.Len())
	}
	normalizador := dataset.AjustarNormalizador(entrenamiento.X)
	normalizador.Aplicar(entrenamiento.X)
	normalizador.Aplicar(prueba.X)

	d := &Datos{Entrenamiento: entrenamiento, Prueba: prueba, PesosClase: logistica.CalcularPesosClase(entrenamiento.Y), Normalizador: normalizador}
	log.Printf("entrenamiento (meses 1-%d): %d ejemplos, %.2f%% positivos | prueba (meses %d-12): %d ejemplos, %.2f%% positivos",
		mesCorte-1, entrenamiento.Len(), 100*float64(entrenamiento.Positivos())/float64(entrenamiento.Len()),
		mesCorte, prueba.Len(), 100*float64(prueba.Positivos())/float64(prueba.Len()))
	log.Printf("pesos de clase: clase 0=%.4f, clase 1=%.2f", d.PesosClase.Cero, d.PesosClase.Uno)
	return d, nil
}

// Corrida resume un entrenamiento para el log y para el CSV de resultados.
type Corrida struct {
	Modo       string
	Goroutines int
	Duracion   time.Duration
	Epocas     int
	Perdidas   []float64
	Evaluacion metricas.Clasificacion
}

// Evaluar mide el modelo sobre el conjunto de prueba (meses no vistos).
func Evaluar(m *logistica.Modelo, prueba *dataset.Muestra) metricas.Clasificacion {
	probabilidades := make([]float64, prueba.Len())
	for i, x := range prueba.X {
		probabilidades[i] = m.Predecir(x)
	}
	return metricas.Evaluar(prueba.Y, probabilidades, 0.5)
}

func Reportar(c *Config, r Corrida, m *logistica.Modelo, n dataset.Normalizador) error {
	paso := max(len(r.Perdidas)/10, 1)
	for e := 0; e < len(r.Perdidas); e += paso {
		log.Printf("epoca %d: perdida=%.4f", e, r.Perdidas[e])
	}
	log.Printf("perdida final: %.4f", r.Perdidas[len(r.Perdidas)-1])
	log.Printf("pesos: %v", m.Pesos)
	log.Printf("sesgo: %.4f", m.Sesgo)
	log.Printf("evaluacion en prueba: %s", r.Evaluacion)
	log.Printf("T(%d) entrenamiento %s (%d epocas): %s (%.3f ms por epoca)", r.Goroutines, r.Modo, r.Epocas,
		r.Duracion.Round(time.Millisecond), float64(r.Duracion.Microseconds())/1000/float64(r.Epocas))

	if c.RutaModelo != "" {
		guardado := dataset.ModeloGuardado{
			Features:       dataset.NombresFeatures,
			Pesos:          m.Pesos,
			Sesgo:          m.Sesgo,
			Normalizador:   n,
			HorizonteHoras: HorizonteHoras,
			UmbralNO2:      dataset.UmbralNO2,
			Modo:           r.Modo,
			Epocas:         r.Epocas,
			F1:             r.Evaluacion.F1,
			AUC:            r.Evaluacion.AUC,
			Precision:      r.Evaluacion.Precision,
			Exhaustividad:  r.Evaluacion.Exhaustividad,
		}
		if err := guardado.Guardar(c.RutaModelo); err != nil {
			return err
		}
		log.Printf("modelo guardado en %s", c.RutaModelo)
	}
	if c.Resultado == "" {
		return nil
	}
	return agregarCSV(c.Resultado, r)
}

func agregarCSV(ruta string, r Corrida) error {
	_, errStat := os.Stat(ruta)
	nuevo := os.IsNotExist(errStat)
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if nuevo {
		w.Write([]string{"modo", "goroutines", "epocas", "t_total_s", "t_epoca_ms", "perdida_final", "f1", "auc"})
	}
	num := func(v float64, dec int) string { return strconv.FormatFloat(v, 'f', dec, 64) }
	w.Write([]string{
		r.Modo,
		strconv.Itoa(r.Goroutines),
		strconv.Itoa(r.Epocas),
		num(r.Duracion.Seconds(), 4),
		num(float64(r.Duracion.Microseconds())/1000/float64(r.Epocas), 4),
		num(r.Perdidas[len(r.Perdidas)-1], 6),
		num(r.Evaluacion.F1, 4),
		num(r.Evaluacion.AUC, 4),
	})
	w.Flush()
	return w.Error()
}
