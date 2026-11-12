package muestrabin

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/registro"
)

// rutaTabla es la tabla de seno/coseno que usa el cargador de Pthreads
// para calcular las variables cíclicas con los mismos bits que Go
// (dataset.VectorFeatures). Si cambia VectorFeatures, regenerarla con
//
//	SACA_REGENERAR=1 go test ./internal/muestrabin -run TestTablaCiclica
const rutaTabla = "../../nativo/comun/tabla_ciclica.h"

func tablaCiclica() string {
	var b strings.Builder
	b.WriteString("/* Generado por internal/muestrabin/tabla_test.go: NO EDITAR.\n" +
		" * Seno y coseno de hora (0-23), día de la semana (0 = domingo) y mes\n" +
		" * (1-12; la posición 0 no se usa) con los mismos bits que calcula Go en\n" +
		" * dataset.VectorFeatures, para que muestra.bin salga idéntico desde C. */\n" +
		"#ifndef SACA_TABLA_CICLICA_H\n#define SACA_TABLA_CICLICA_H\n\n")
	arreglo := func(nombre string, n, desde int, periodo float64, f func(float64) float64) {
		fmt.Fprintf(&b, "static const double %s[%d] = {", nombre, n)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			if i < desde {
				b.WriteString("0")
				continue
			}
			v := float64(i)
			b.WriteString(strconv.FormatFloat(f(2*math.Pi*v/periodo), 'x', -1, 64))
		}
		b.WriteString("};\n")
	}
	arreglo("SACA_SIN_HORA", 24, 0, 24, math.Sin)
	arreglo("SACA_COS_HORA", 24, 0, 24, math.Cos)
	arreglo("SACA_SIN_DIA", 7, 0, 7, math.Sin)
	arreglo("SACA_COS_DIA", 7, 0, 7, math.Cos)
	arreglo("SACA_SIN_MES", 13, 1, 12, math.Sin)
	arreglo("SACA_COS_MES", 13, 1, 12, math.Cos)
	b.WriteString("\n#endif\n")
	return b.String()
}

func TestTablaCiclica(t *testing.T) {
	quiero := tablaCiclica()
	if os.Getenv("SACA_REGENERAR") != "" {
		if err := os.WriteFile(rutaTabla, []byte(quiero), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tengo, err := os.ReadFile(rutaTabla)
	if err != nil {
		t.Fatal(err)
	}
	if string(tengo) != quiero {
		t.Fatalf("%s no coincide con dataset.VectorFeatures; regenérala (ver rutaTabla)", rutaTabla)
	}
}

// La fórmula de la tabla debe dar los mismos bits que VectorFeatures.
func TestTablaIgualAVectorFeatures(t *testing.T) {
	no2 := 1.0
	for h := 0; h < 24*7*60; h += 5 {
		ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(h) * time.Hour)
		x := dataset.VectorFeatures(&registro.RegistroProcesado{TimestampUTC: ts.Format(time.RFC3339), NO2Ppb: &no2})
		hora, dia, mes := float64(ts.Hour()), float64(ts.Weekday()), float64(ts.Month())
		quiero := []float64{math.Sin(2 * math.Pi * hora / 24), math.Cos(2 * math.Pi * hora / 24),
			math.Sin(2 * math.Pi * dia / 7), math.Cos(2 * math.Pi * dia / 7),
			math.Sin(2 * math.Pi * mes / 12), math.Cos(2 * math.Pi * mes / 12)}
		for j, v := range quiero {
			if math.Float64bits(x[6+j]) != math.Float64bits(v) {
				t.Fatalf("%s variable %d: %v != %v", ts, 6+j, x[6+j], v)
			}
		}
	}
}
