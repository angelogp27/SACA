// Resume el CSV del Experimento 8 (CUDA frente a OpenMP, de 10^3 a 10^7
// ejemplos) en un JSON para el panel de tecnologías de la página.
//
// A diferencia de los demás experimentos, aquí la variable no es el número de
// hilos sino el tamaño del problema. Cada fila del CSV lleva el equipo y el
// tamaño en la columna "maquina" ("<equipo> · <N> ejemplos"). La salida agrupa
// por equipo y por tamaño y toma la mediana de t_epoca_ms de las
// repeticiones: son solo mediciones reales del CSV, nada se inventa ni se
// rellena.
//
//	go run ./experimentos/cmd/gpu -entrada resultados/experimento8.csv -json frontend/public/datos/experimento8.json
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"
)

type Medicion struct {
	TEpocaMs     float64 `json:"t_epoca_ms"`
	Repeticiones int     `json:"repeticiones"`
	PerdidaFinal float64 `json:"perdida_final"`
}

type Tamano struct {
	Ejemplos int                 `json:"ejemplos"`
	Modos    map[string]Medicion `json:"modos"`
}

type Equipo struct {
	Nombre  string   `json:"nombre"`
	Tamanos []Tamano `json:"tamanos"`
}

type Resultado struct {
	Experimento string   `json:"experimento"`
	Descripcion string   `json:"descripcion"`
	Generado    string   `json:"generado"`
	Base        string   `json:"modo_base"`
	Equipos     []Equipo `json:"equipos"`
}

var reMaquina = regexp.MustCompile(`^(.*) · (\d+) ejemplos$`)

func main() {
	entrada := flag.String("entrada", "resultados/experimento8.csv", "CSV de corridas")
	salida := flag.String("json", "frontend/public/datos/experimento8.json", "JSON de salida")
	flag.Parse()

	res, err := resumir(*entrada)
	if err != nil {
		log.Fatal(err)
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(*salida, b, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("%s: %d equipo(s) → %s", *entrada, len(res.Equipos), *salida)
}

type clave struct {
	equipo   string
	ejemplos int
	modo     string
}

func resumir(ruta string) (*Resultado, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	filas, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ruta, err)
	}
	if len(filas) < 2 {
		return nil, fmt.Errorf("%s no tiene corridas", ruta)
	}
	col := map[string]int{}
	for i, nombre := range filas[0] {
		col[nombre] = i
	}
	for _, c := range []string{"modo", "t_epoca_ms", "perdida_final", "maquina"} {
		if _, ok := col[c]; !ok {
			return nil, fmt.Errorf("%s: falta la columna %q", ruta, c)
		}
	}

	tiempos := map[clave][]float64{}
	perdidas := map[clave]float64{}
	for _, fila := range filas[1:] {
		m := reMaquina.FindStringSubmatch(fila[col["maquina"]])
		if m == nil {
			continue // fila sin tamaño de problema: no es de este experimento
		}
		n, _ := strconv.Atoi(m[2])
		t, err1 := strconv.ParseFloat(fila[col["t_epoca_ms"]], 64)
		p, err2 := strconv.ParseFloat(fila[col["perdida_final"]], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		k := clave{equipo: m[1], ejemplos: n, modo: fila[col["modo"]]}
		tiempos[k] = append(tiempos[k], t)
		perdidas[k] = p
	}

	porEquipo := map[string]map[int]map[string]Medicion{}
	for k, ts := range tiempos {
		if porEquipo[k.equipo] == nil {
			porEquipo[k.equipo] = map[int]map[string]Medicion{}
		}
		if porEquipo[k.equipo][k.ejemplos] == nil {
			porEquipo[k.equipo][k.ejemplos] = map[string]Medicion{}
		}
		porEquipo[k.equipo][k.ejemplos][k.modo] = Medicion{TEpocaMs: mediana(ts), Repeticiones: len(ts), PerdidaFinal: perdidas[k]}
	}

	res := &Resultado{
		Experimento: "Experimento 8",
		Descripcion: "Entrenamiento en GPU (CUDA) frente a CPU (OpenMP), según el tamaño del problema",
		Generado:    time.Now().Format(time.RFC3339),
		Base:        "secuencial_c",
	}
	nombres := make([]string, 0, len(porEquipo))
	for n := range porEquipo {
		nombres = append(nombres, n)
	}
	sort.Strings(nombres)
	for _, nombre := range nombres {
		eq := Equipo{Nombre: nombre}
		for n, modos := range porEquipo[nombre] {
			eq.Tamanos = append(eq.Tamanos, Tamano{Ejemplos: n, Modos: modos})
		}
		sort.Slice(eq.Tamanos, func(i, j int) bool { return eq.Tamanos[i].Ejemplos < eq.Tamanos[j].Ejemplos })
		res.Equipos = append(res.Equipos, eq)
	}
	if len(res.Equipos) == 0 {
		return nil, fmt.Errorf("%s no tiene filas con el tamaño del problema en la columna maquina", ruta)
	}
	return res, nil
}

func mediana(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if n := len(s); n%2 == 1 {
		return s[n/2]
	} else {
		return (s[n/2-1] + s[n/2]) / 2
	}
}
