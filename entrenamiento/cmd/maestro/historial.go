package main

// Historial de procesos del clúster (pestaña Resultados de la página): cada
// limpieza, entrenamiento y predicción distribuida que termina se guarda
// en un archivo JSON en la laptop del maestro, con todos sus datos, para
// volver a verla después aunque se haya cerrado SACA.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"proyecto9/internal/distribuido"
)

// Tipos de proceso del historial (los mismos nombres que usa la página).
const (
	HistLimpieza      = "limpieza"
	HistEntrenamiento = "entrenamiento"
	HistPrediccion    = "prediccion"
)

// entradaHistorial es un proceso terminado. Datos es el estado completo
// (EstadoLimpieza, EstadoCorrida o EstadoPrediccion), tal como lo veía la
// página al terminar; el listado no lo incluye.
type entradaHistorial struct {
	ID          string          `json:"id"`
	Tipo        string          `json:"tipo"`
	Fecha       time.Time       `json:"fecha"`
	Maestro     string          `json:"maestro"`
	Descripcion string          `json:"descripcion"`
	Estado      string          `json:"estado"`
	DuracionS   float64         `json:"duracion_s"`
	Datos       json.RawMessage `json:"datos,omitempty"`
}

type historial struct {
	dir     string // vacío = no guardar
	maestro string
	mu      sync.Mutex
}

// idValido: solo nombres que genera guardar (evita leer fuera de dir).
var idValido = regexp.MustCompile(`^(limpieza|entrenamiento|prediccion)-\d{8}-\d{6}-\d+$`)

func nuevoHistorial(dir, maestro string) *historial {
	if maestro == "" {
		maestro, _ = os.Hostname()
	}
	return &historial{dir: dir, maestro: maestro}
}

// guardar escribe un proceso terminado; un error solo se registra en el log.
func (h *historial) guardar(tipo, descripcion, estado string, duracionS float64, datos any) {
	if h == nil || h.dir == "" {
		return
	}
	crudo, err := json.Marshal(datos)
	if err != nil {
		log.Printf("historial: %v", err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		log.Printf("historial: %v", err)
		return
	}
	ahora := time.Now()
	var id string
	for n := 1; ; n++ {
		id = fmt.Sprintf("%s-%s-%d", tipo, ahora.Format("20060102-150405"), n)
		if _, err := os.Stat(filepath.Join(h.dir, id+".json")); errors.Is(err, os.ErrNotExist) {
			break
		}
	}
	e := entradaHistorial{ID: id, Tipo: tipo, Fecha: ahora, Maestro: h.maestro, Descripcion: descripcion, Estado: estado, DuracionS: duracionS, Datos: crudo}
	b, err := json.Marshal(e)
	if err != nil {
		log.Printf("historial: %v", err)
		return
	}
	ruta := filepath.Join(h.dir, id+".json")
	tmp := ruta + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Printf("historial: %v", err)
		return
	}
	if err := os.Rename(tmp, ruta); err != nil {
		log.Printf("historial: %v", err)
		return
	}
	log.Printf("historial: %s guardado (%s)", id, descripcion)
}

// listar devuelve los procesos de un tipo ("" = todos), el más reciente
// primero, sin sus datos.
func (h *historial) listar(tipo string) ([]entradaHistorial, error) {
	lista := []entradaHistorial{}
	if h == nil || h.dir == "" {
		return lista, nil
	}
	archivos, err := os.ReadDir(h.dir)
	if errors.Is(err, os.ErrNotExist) {
		return lista, nil
	}
	if err != nil {
		return nil, err
	}
	for _, a := range archivos {
		id := strings.TrimSuffix(a.Name(), ".json")
		if a.IsDir() || !strings.HasSuffix(a.Name(), ".json") || !idValido.MatchString(id) {
			continue
		}
		if tipo != "" && !strings.HasPrefix(id, tipo+"-") {
			continue
		}
		e, err := h.leer(id)
		if err != nil {
			log.Printf("historial: %s ilegible: %v", a.Name(), err)
			continue
		}
		e.Datos = nil
		lista = append(lista, *e)
	}
	slices.SortFunc(lista, func(a, b entradaHistorial) int { return b.Fecha.Compare(a.Fecha) })
	return lista, nil
}

func (h *historial) leer(id string) (*entradaHistorial, error) {
	if !idValido.MatchString(id) {
		return nil, errors.New("proceso desconocido")
	}
	b, err := os.ReadFile(filepath.Join(h.dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var e entradaHistorial
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// Rutas de la API: GET /api/historial?tipo=… y GET /api/historial/{id}.
func (a *api) listarHistorial(w http.ResponseWriter, r *http.Request) {
	tipo := r.URL.Query().Get("tipo")
	if tipo != "" && tipo != HistLimpieza && tipo != HistEntrenamiento && tipo != HistPrediccion {
		fallo(w, http.StatusBadRequest, errors.New("tipo debe ser limpieza, entrenamiento o prediccion"))
		return
	}
	lista, err := a.cfg.historial.listar(tipo)
	if err != nil {
		fallo(w, http.StatusInternalServerError, err)
		return
	}
	responder(w, http.StatusOK, lista)
}

func (a *api) leerHistorial(w http.ResponseWriter, r *http.Request) {
	if a.cfg.historial == nil {
		fallo(w, http.StatusNotFound, errors.New("sin historial"))
		return
	}
	e, err := a.cfg.historial.leer(r.PathValue("id"))
	if err != nil {
		fallo(w, http.StatusNotFound, err)
		return
	}
	responder(w, http.StatusOK, e)
}

// ---------- descripciones de cada proceso ----------

var nombresTec = map[string]string{"pthreads": "Pthreads", "go": "Go", "openmp": "OpenMP"}

func describirLimpieza(e distribuido.EstadoLimpieza) string {
	tecs := make([]string, len(e.Tecnologias))
	for i, t := range e.Tecnologias {
		tecs[i] = nombresTec[t]
	}
	return fmt.Sprintf("Limpieza e ingeniería de features · %s · %s", strings.Join(tecs, " y "), plural(len(e.Nodos), "nodo", "nodos"))
}

func describirModeloTexto(modelo string) string {
	if strings.HasPrefix(modelo, "red") {
		return "Red Neuronal"
	}
	return "Regresión logística"
}

func describirEntrenamiento(c *distribuido.EstadoCorrida) string {
	partes := []string{describirModeloTexto(c.Modelo)}
	if strings.HasPrefix(c.Modelo, "red") {
		partes = append(partes, map[string]string{"central": "todo por el maestro", "arbol": "en árbol", "anillo": "en anillo", "mpi": "con MPI"}[c.Comunicacion])
		if c.PasosPorEpoca > 0 {
			partes = append(partes, fmt.Sprintf("%d mini-lotes", c.PasosPorEpoca))
		}
	}
	if r := map[string]string{"dinamico": "reparto dinámico", "capacidad": "según la capacidad", "iguales": "partes iguales"}[c.Reparto]; r != "" {
		partes = append(partes, r)
	}
	if t := describirMotores(c.Motores, c.CUDA); t != "" {
		partes = append(partes, t)
	}
	partes = append(partes, plural(len(c.Nodos), "nodo", "nodos"), plural(c.Epocas, "época", "épocas"))
	return "Entrenamiento · " + strings.Join(partes, " · ")
}

// describirMotores: "OpenMP y Go", "Go + CUDA"…
func describirMotores(motores []string, cuda bool) string {
	var nombres []string
	for _, m := range motores {
		if n := nombresTec[m]; n != "" {
			nombres = append(nombres, n)
		}
	}
	t := strings.Join(nombres, " y ")
	if t != "" && cuda {
		t += " + CUDA"
	}
	return t
}

func describirPrediccion(e distribuido.EstadoPrediccion) string {
	motor := describirMotores([]string{e.MotorCPU}, e.CUDA)
	if e.Comunicacion == "mpi" {
		motor += " · MPI"
	}
	return fmt.Sprintf("Predicción distribuida · %s · %s · %s · %.1f MB", describirModeloTexto(e.Modelo), motor, plural(len(e.Nodos), "nodo", "nodos"), e.MB)
}

func plural(n int, uno, varios string) string {
	if n == 1 {
		return "1 " + uno
	}
	return fmt.Sprintf("%d %s", n, varios)
}
