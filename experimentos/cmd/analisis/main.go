// Análisis de un experimento de escalabilidad: lee el CSV de corridas
// (una fila por ejecución, ver internal/medicion), toma la mediana de las
// repeticiones y calcula speedup, eficiencia, la métrica de Karp-Flatt y
// la predicción de la ley de Amdahl. Escribe un JSON para el panel de
// rendimiento del frontend y una tabla Markdown para el informe.
//
// Sirve para cualquier experimento cuyo CSV tenga las columnas modo,
// goroutines y t_total_s: el Experimento 1 (cargador) y el 1b
// (entrenamiento paralelo).
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Punto struct {
	Unidades     int     `json:"unidades"`
	Repeticiones int     `json:"repeticiones"`
	TTotal       float64 `json:"t_total_s"`
	Speedup      float64 `json:"speedup"`
	Eficiencia   float64 `json:"eficiencia"`
	KarpFlatt    float64 `json:"karp_flatt,omitempty"`
	Amdahl       float64 `json:"speedup_amdahl"`
	// Columnas adicionales del CSV (mediana), por ejemplo t_mongo_s.
	Fases map[string]float64 `json:"fases"`
}

type Resultado struct {
	Experimento    string             `json:"experimento"`
	Descripcion    string             `json:"descripcion"`
	Generado       string             `json:"generado"`
	CPUsLogicas    int                `json:"cpus_logicas"`
	ModoBase       string             `json:"modo_base"`
	ModoParalelo   string             `json:"modo_paralelo"`
	T1             float64            `json:"t1_s"`
	FasesBase      map[string]float64 `json:"fases_base"`
	FraccionSerial float64            `json:"fraccion_serial"`
	OrigenFraccion string             `json:"origen_fraccion_serial"`
	ColumnasSerial []string           `json:"columnas_serial,omitempty"`
	SpeedupMaximo  float64            `json:"speedup_maximo_amdahl"`
	Puntos         []Punto            `json:"puntos"`
	// Todas las series del experimento (la principal y, p. ej., otras
	// estrategias de sincronización), comparadas contra el mismo T(1).
	Series map[string][]Punto `json:"series"`
}

func main() {
	entrada := flag.String("entrada", "resultados/experimento1.csv", "CSV de corridas")
	salidaJSON := flag.String("json", "frontend/public/datos/experimento1.json", "JSON de salida para el frontend")
	salidaMD := flag.String("md", "resultados/experimento1.md", "tabla Markdown de salida")
	nombre := flag.String("nombre", "Experimento 1", "nombre del experimento")
	descripcion := flag.String("descripcion", "Cargador: ingesta de los 4 CSV horarios de la EPA", "descripción corta")
	modoBase := flag.String("base", "secuencial", "modo que da T(1)")
	modosParalelos := flag.String("paralelo", "concurrente", "modos (separados por coma) que se comparan contra T(1); el primero es la serie principal")
	columnasSerial := flag.String("serial", "t_indexado_s,t_mongo_s", "columnas que suman la parte serial del T(1), para Amdahl; vacío para estimarla con Karp-Flatt")
	flag.Parse()

	corridas, err := leer(*entrada)
	if err != nil {
		log.Fatal(err)
	}

	base := corridas[clave{*modoBase, 1}]
	if len(base) == 0 {
		log.Fatalf("no hay corridas del modo %q con 1 goroutine en %s", *modoBase, *entrada)
	}
	fasesBase := medianas(base)
	t1 := fasesBase["t_total_s"]

	modos := strings.Split(*modosParalelos, ",")
	series := map[string][]Punto{}
	for _, modo := range modos {
		if ps := puntos(corridas, modo, t1); len(ps) > 0 {
			series[modo] = ps
		}
	}
	principal := series[modos[0]]
	if len(principal) == 0 {
		log.Fatalf("no hay corridas del modo %q en %s", modos[0], *entrada)
	}

	// Fracción serial de Amdahl. Si se indican columnas, se mide con las
	// fases que por diseño hace una sola goroutine (indexado y escritura en
	// Mongo en el cargador original). Si no, se estima con Karp-Flatt en el
	// mayor p medido de la serie principal.
	var f float64
	var columnas []string
	origen := "fases_medidas"
	if strings.TrimSpace(*columnasSerial) != "" {
		var serial float64
		for _, c := range strings.Split(*columnasSerial, ",") {
			c = strings.TrimSpace(c)
			columnas = append(columnas, c)
			serial += fasesBase[c]
		}
		f = serial / t1
	} else {
		origen = "karp_flatt"
		f = principal[len(principal)-1].KarpFlatt
	}
	f = min(max(f, 1e-6), 1)
	for _, ps := range series {
		for i := range ps {
			ps[i].Amdahl = 1 / (f + (1-f)/float64(ps[i].Unidades))
		}
	}

	r := Resultado{
		Experimento:    *nombre,
		Descripcion:    *descripcion,
		Generado:       time.Now().Format(time.RFC3339),
		CPUsLogicas:    runtime.NumCPU(),
		ModoBase:       *modoBase,
		ModoParalelo:   modos[0],
		T1:             t1,
		FasesBase:      fasesBase,
		FraccionSerial: f,
		OrigenFraccion: origen,
		ColumnasSerial: columnas,
		SpeedupMaximo:  1 / f,
		Puntos:         principal,
		Series:         series,
	}

	if err := escribirJSON(*salidaJSON, r); err != nil {
		log.Fatal(err)
	}
	md := tablaMarkdown(r, modos)
	if err := os.WriteFile(*salidaMD, []byte(md), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Print(md)
}

// puntos resume las corridas de un modo: mediana por número de
// goroutines, speedup y eficiencia contra t1, y Karp-Flatt.
func puntos(corridas map[clave][]map[string]float64, modo string, t1 float64) []Punto {
	var res []Punto
	for k, filas := range corridas {
		if k.modo != modo {
			continue
		}
		fases := medianas(filas)
		p := Punto{Unidades: k.goroutines, Repeticiones: len(filas), TTotal: fases["t_total_s"], Fases: fases}
		p.Speedup = t1 / p.TTotal
		p.Eficiencia = p.Speedup / float64(p.Unidades)
		n := float64(p.Unidades)
		if p.Unidades > 1 {
			// Karp-Flatt: fracción serial "experimental" que explicaría el
			// speedup observado. Si crece con p, la pérdida no es solo la
			// parte serial sino sobrecarga de sincronización.
			p.KarpFlatt = (1/p.Speedup - 1/n) / (1 - 1/n)
		}
		res = append(res, p)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Unidades < res[j].Unidades })
	return res
}

type clave struct {
	modo       string
	goroutines int
}

func leer(ruta string) (map[clave][]map[string]float64, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	filas, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(filas) < 2 {
		return nil, fmt.Errorf("%s no tiene corridas", ruta)
	}
	encabezado := filas[0]

	corridas := map[clave][]map[string]float64{}
	for _, fila := range filas[1:] {
		valores := map[string]float64{}
		var k clave
		for i, col := range encabezado {
			switch col {
			case "modo":
				k.modo = fila[i]
			case "goroutines":
				k.goroutines, _ = strconv.Atoi(fila[i])
			default:
				if v, err := strconv.ParseFloat(fila[i], 64); err == nil {
					valores[col] = v
				}
			}
		}
		corridas[k] = append(corridas[k], valores)
	}
	return corridas, nil
}

// medianas toma, columna por columna, la mediana de las repeticiones: es
// robusta a una corrida aislada lenta (antivirus, OneDrive, etc.).
func medianas(filas []map[string]float64) map[string]float64 {
	res := map[string]float64{}
	for col := range filas[0] {
		vals := make([]float64, 0, len(filas))
		for _, f := range filas {
			vals = append(vals, f[col])
		}
		sort.Float64s(vals)
		m := len(vals) / 2
		if len(vals)%2 == 0 {
			res[col] = (vals[m-1] + vals[m]) / 2
		} else {
			res[col] = vals[m]
		}
	}
	return res
}

func tablaMarkdown(r Resultado, modos []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s — %s\n\n", r.Experimento, r.Descripcion)
	fmt.Fprintf(&b, "Máquina: %d CPUs lógicas. T(1) = %.3f s (modo %s, mediana).\n", r.CPUsLogicas, r.T1, r.ModoBase)
	if r.OrigenFraccion == "karp_flatt" {
		fmt.Fprintf(&b, "Fracción serial estimada con Karp-Flatt en el mayor p: f = %.3f → speedup máximo de Amdahl = %.2f.\n", r.FraccionSerial, r.SpeedupMaximo)
	} else {
		fmt.Fprintf(&b, "Fracción serial medida en T(1) (%s): f = %.3f → speedup máximo de Amdahl = %.2f.\n", strings.Join(r.ColumnasSerial, " + "), r.FraccionSerial, r.SpeedupMaximo)
	}
	for _, modo := range modos {
		ps, ok := r.Series[modo]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n### %s\n\n", modo)
		b.WriteString("| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |\n|---:|---:|---:|---:|---:|---:|---:|\n")
		for _, p := range ps {
			kf := "—"
			if p.Unidades > 1 {
				kf = fmt.Sprintf("%.3f", p.KarpFlatt)
			}
			fmt.Fprintf(&b, "| %d | %d | %.3f | %.2f | %.0f%% | %s | %.2f |\n", p.Unidades, p.Repeticiones, p.TTotal, p.Speedup, 100*p.Eficiencia, kf, p.Amdahl)
		}
	}
	return b.String()
}

func escribirJSON(ruta string, v any) error {
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	datos, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ruta, append(datos, '\n'), 0o644)
}
