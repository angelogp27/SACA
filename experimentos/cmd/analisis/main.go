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
	ColumnasSerial []string           `json:"columnas_serial"`
	SpeedupMaximo  float64            `json:"speedup_maximo_amdahl"`
	Puntos         []Punto            `json:"puntos"`
}

func main() {
	entrada := flag.String("entrada", "resultados/experimento1.csv", "CSV de corridas")
	salidaJSON := flag.String("json", "frontend/public/datos/experimento1.json", "JSON de salida para el frontend")
	salidaMD := flag.String("md", "resultados/experimento1.md", "tabla Markdown de salida")
	nombre := flag.String("nombre", "Experimento 1", "nombre del experimento")
	descripcion := flag.String("descripcion", "Cargador: ingesta de los 4 CSV horarios de la EPA", "descripción corta")
	modoBase := flag.String("base", "secuencial", "modo que da T(1)")
	modoParalelo := flag.String("paralelo", "concurrente", "modo cuyas corridas se comparan contra T(1)")
	columnasSerial := flag.String("serial", "t_indexado_s,t_mongo_s", "columnas que suman la parte serial del T(1), para Amdahl")
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

	// Fracción serial de Amdahl, estimada con las fases que por diseño
	// solo puede hacer una goroutine (indexado y escritura en Mongo en el
	// cargador). Ver docs/experimento1.md.
	var serial float64
	columnas := strings.Split(*columnasSerial, ",")
	for _, c := range columnas {
		serial += fasesBase[strings.TrimSpace(c)]
	}
	f := serial / t1

	r := Resultado{
		Experimento:    *nombre,
		Descripcion:    *descripcion,
		Generado:       time.Now().Format(time.RFC3339),
		CPUsLogicas:    runtime.NumCPU(),
		ModoBase:       *modoBase,
		ModoParalelo:   *modoParalelo,
		T1:             t1,
		FasesBase:      fasesBase,
		FraccionSerial: f,
		ColumnasSerial: columnas,
		SpeedupMaximo:  1 / f,
	}

	for k, filas := range corridas {
		if k.modo != *modoParalelo {
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
		p.Amdahl = 1 / (f + (1-f)/n)
		r.Puntos = append(r.Puntos, p)
	}
	sort.Slice(r.Puntos, func(i, j int) bool { return r.Puntos[i].Unidades < r.Puntos[j].Unidades })

	if err := escribirJSON(*salidaJSON, r); err != nil {
		log.Fatal(err)
	}
	md := tablaMarkdown(r)
	if err := os.WriteFile(*salidaMD, []byte(md), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Print(md)
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

func tablaMarkdown(r Resultado) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s — %s\n\n", r.Experimento, r.Descripcion)
	fmt.Fprintf(&b, "Máquina: %d CPUs lógicas. T(1) = %.2f s (modo %s, mediana).\n", r.CPUsLogicas, r.T1, r.ModoBase)
	fmt.Fprintf(&b, "Fracción serial medida (%s): f = %.3f → speedup máximo de Amdahl = %.2f.\n\n", strings.Join(r.ColumnasSerial, " + "), r.FraccionSerial, r.SpeedupMaximo)
	b.WriteString("| p | reps | T(p) [s] | S(p) | E(p) | Karp-Flatt | S Amdahl |\n|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, p := range r.Puntos {
		kf := "—"
		if p.Unidades > 1 {
			kf = fmt.Sprintf("%.3f", p.KarpFlatt)
		}
		fmt.Fprintf(&b, "| %d | %d | %.2f | %.2f | %.0f%% | %s | %.2f |\n", p.Unidades, p.Repeticiones, p.TTotal, p.Speedup, 100*p.Eficiencia, kf, p.Amdahl)
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
