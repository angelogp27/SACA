package main

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/distribuido"
)

// datosDesdePrediccion regenera, con la predicción distribuida (Fase 5) y
// sin MongoDB, los datos de la página:
//   - series.json: el NO2 observado y la probabilidad del modelo nuevo, hora
//     por hora, del día que muestra la línea de tiempo, para cada estación;
//   - estaciones.json: "alerta" = la probabilidad más alta de ese día (el
//     mapa puede colorear por ella);
//   - resumen.json: las métricas del modelo y los datos de la predicción.
func datosDesdePrediccion(dirs []string, e distribuido.EstadoPrediccion, filas []distribuido.FilaPrediccion, modelo distribuido.ModeloEntrenado) {
	var existentes []string
	for _, d := range dirs {
		if d = strings.TrimSpace(d); d != "" {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				existentes = append(existentes, d)
			}
		}
	}
	if len(existentes) == 0 {
		log.Print("predicción: no hay carpeta de datos de la página para actualizar")
		return
	}
	leer := func(nombre string, v any) bool {
		datos, err := os.ReadFile(filepath.Join(existentes[0], nombre))
		return err == nil && json.Unmarshal(datos, v) == nil
	}
	resumen := map[string]any{}
	leer("resumen.json", &resumen)
	dia, _ := resumen["dia_series"].(string)
	if dia == "" {
		dia = "2025-12-18"
	}

	type serie struct {
		Fecha        string     `json:"fecha"`
		Contaminante string     `json:"contaminante"`
		Unidad       string     `json:"unidad"`
		Umbral       float64    `json:"umbral"`
		Observado    []*float64 `json:"observado"`
		Probabilidad []*float64 `json:"probabilidad"`
	}
	nuevas := map[string]*serie{}
	alerta := map[string]float64{}
	for _, f := range filas {
		t := time.Unix(f.Hora*3600, 0).UTC()
		if t.Format("2006-01-02") != dia {
			continue
		}
		s := nuevas[f.Estacion]
		if s == nil {
			s = &serie{Fecha: dia, Contaminante: "NO2", Unidad: "ppb", Umbral: dataset.UmbralNO2, Observado: make([]*float64, 24), Probabilidad: make([]*float64, 24)}
			nuevas[f.Estacion] = s
		}
		obs, p := math.Round(f.NO2*10)/10, math.Round(f.Prob*1e4)/1e4
		s.Observado[t.Hour()], s.Probabilidad[t.Hour()] = &obs, &p
		alerta[f.Estacion] = max(alerta[f.Estacion], p)
	}

	series := map[string]json.RawMessage{}
	leer("series.json", &series)
	for est, s := range nuevas {
		b, _ := json.Marshal(s)
		series[est] = b
	}
	var estaciones []map[string]any
	leer("estaciones.json", &estaciones)
	for _, est := range estaciones {
		id, _ := est["id"].(string)
		if a, ok := alerta[id]; ok {
			est["alerta"] = a
		} else {
			delete(est, "alerta")
		}
	}
	resumen["modelo"] = map[string]any{
		"nombre": modelo.Nombre, "auc": modelo.Evaluacion.AUC, "f1": modelo.Evaluacion.F1, "precision": modelo.Evaluacion.Precision,
		"exhaustividad": modelo.Evaluacion.Exhaustividad, "horizonte_horas": 1, "umbral_no2_ppb": dataset.UmbralNO2,
	}
	resumen["prediccion"] = map[string]any{
		"nodos": len(e.Nodos), "filas": e.Filas, "mb": math.Round(e.MB*10) / 10, "t_total_s": math.Round(e.TTotalS*100) / 100,
		"generado": time.Now().UTC().Format(time.RFC3339), "estaciones_del_dia": len(nuevas),
	}

	for _, d := range existentes {
		for nombre, v := range map[string]any{"series.json": series, "estaciones.json": estaciones, "resumen.json": resumen} {
			if nombre == "estaciones.json" && estaciones == nil {
				continue
			}
			datos, err := json.Marshal(v)
			if err == nil {
				tmp := filepath.Join(d, nombre+".tmp")
				if err = os.WriteFile(tmp, datos, 0o644); err == nil {
					os.Remove(filepath.Join(d, nombre))
					err = os.Rename(tmp, filepath.Join(d, nombre))
				}
			}
			if err != nil {
				log.Printf("predicción: no se pudo escribir %s en %s: %v", nombre, d, err)
			}
		}
	}
	log.Printf("predicción %d: mapa y línea de tiempo del %s actualizados (%d estaciones) en %s", e.ID, dia, len(nuevas), strings.Join(existentes, ", "))
}
