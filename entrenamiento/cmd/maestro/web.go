package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/distribuido"
)

// difusor reparte la última instantánea del maestro a los navegadores
// conectados por Server-Sent Events. El maestro puede publicar en cada
// época; el difusor envía como mucho una cada intervalo (la última), así
// la página se ve en vivo sin saturar la red.
type difusor struct {
	mu        sync.Mutex
	ultima    []byte
	pendiente bool
	subs      map[chan []byte]struct{}
}

const intervaloEnvio = 150 * time.Millisecond

func nuevoDifusor() *difusor {
	d := &difusor{subs: map[chan []byte]struct{}{}}
	go func() {
		for range time.Tick(intervaloEnvio) {
			d.enviarPendiente()
		}
	}()
	return d
}

// Publicar es el Notificar del maestro: solo guarda la instantánea.
func (d *difusor) Publicar(i distribuido.Instantanea) {
	datos, err := json.Marshal(i)
	if err != nil {
		log.Printf("no se pudo serializar el estado: %v", err)
		return
	}
	d.mu.Lock()
	d.ultima, d.pendiente = datos, true
	d.mu.Unlock()
}

func (d *difusor) enviarPendiente() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.pendiente {
		return
	}
	d.pendiente = false
	for ch := range d.subs {
		// Canal con buffer 1: si el navegador no alcanzó a leer la anterior,
		// se reemplaza por la nueva (solo importa la última).
		select {
		case <-ch:
		default:
		}
		ch <- d.ultima
	}
}

func (d *difusor) suscribir() (chan []byte, func()) {
	ch := make(chan []byte, 1)
	d.mu.Lock()
	d.subs[ch] = struct{}{}
	d.mu.Unlock()
	return ch, func() {
		d.mu.Lock()
		delete(d.subs, ch)
		d.mu.Unlock()
	}
}

// api atiende la página: estado, eventos en vivo y comandos.
type api struct {
	m       *distribuido.Maestro
	difusor *difusor
	cfg     configuracion
	base    context.Context

	mu       sync.Mutex
	cancelar context.CancelFunc // de la corrida en curso
}

func (a *api) rutas() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/estado", a.estado)
	mux.HandleFunc("GET /api/eventos", a.eventos)
	mux.HandleFunc("POST /api/corridas", a.nuevaCorrida)
	mux.HandleFunc("POST /api/corridas/cancelar", a.cancelarCorrida)
	mux.HandleFunc("POST /api/nodos/quitar", a.quitarNodo)
	mux.HandleFunc("POST /api/sesion/terminar", a.terminarSesion)
	mux.HandleFunc("POST /api/limpiezas", a.nuevaLimpieza)
	mux.HandleFunc("POST /api/limpiezas/cancelar", a.cancelarLimpieza)
	mux.HandleFunc("POST /api/huellas/verificar", a.verificarHuellas)
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		responder(w, http.StatusOK, map[string]any{"epocas": a.cfg.epocas, "tasa": a.cfg.tasa, "verificar": a.cfg.verificar})
	})
	return mux
}

func responder(w http.ResponseWriter, codigo int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(v)
}

func fallo(w http.ResponseWriter, codigo int, err error) {
	responder(w, codigo, map[string]string{"error": err.Error()})
}

func (a *api) estado(w http.ResponseWriter, r *http.Request) {
	responder(w, http.StatusOK, a.m.Instantanea())
}

func (a *api) eventos(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fallo(w, http.StatusInternalServerError, errors.New("el servidor no admite eventos en vivo"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, desuscribir := a.difusor.suscribir()
	defer desuscribir()

	enviar := func(datos []byte) bool {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", datos); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	inicial, _ := json.Marshal(a.m.Instantanea())
	if !enviar(inicial) {
		return
	}
	latido := time.NewTicker(15 * time.Second)
	defer latido.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case datos := <-ch:
			if !enviar(datos) {
				return
			}
		case <-latido.C:
			// Comentario SSE: mantiene viva la conexión a través de proxies.
			if _, err := fmt.Fprint(w, ": latido\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

type pedidoCorrida struct {
	Nodos     []string `json:"nodos"`
	Epocas    int      `json:"epocas"`
	Verificar bool     `json:"verificar"`
}

func (a *api) nuevaCorrida(w http.ResponseWriter, r *http.Request) {
	var p pedidoCorrida
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		fallo(w, http.StatusBadRequest, fmt.Errorf("pedido inválido: %w", err))
		return
	}
	if p.Epocas < 1 || p.Epocas > 100000 {
		fallo(w, http.StatusBadRequest, errors.New("las épocas deben estar entre 1 y 100000"))
		return
	}
	if len(p.Nodos) == 0 {
		fallo(w, http.StatusBadRequest, distribuido.ErrSinNodos)
		return
	}
	corrida, err := a.m.Iniciar(distribuido.OpcionesCorrida{Nodos: p.Nodos, Epocas: p.Epocas, Tasa: a.cfg.tasa, Verificar: p.Verificar, Continuar: true})
	switch {
	case errors.Is(err, distribuido.ErrOcupado), errors.Is(err, distribuido.ErrLimpiando),
		errors.Is(err, distribuido.ErrMuestraDistinta), errors.Is(err, distribuido.ErrSinMuestra):
		fallo(w, http.StatusConflict, err)
		return
	case err != nil:
		fallo(w, http.StatusBadRequest, err)
		return
	}

	ctx, cancelar := context.WithCancel(a.base)
	a.mu.Lock()
	a.cancelar = cancelar
	a.mu.Unlock()
	go func() {
		defer cancelar()
		res, err := corrida.Ejecutar(ctx)
		if err != nil {
			log.Printf("corrida terminada sin completarse: %v", err)
			return
		}
		reportar(res, p.Epocas)
		regenerarMapa(a.cfg, res, p.Epocas)
		if a.cfg.resultado == "" {
			return
		}
		modo, maquina := combinacion(a.cfg.modo, res.Nodos, a.m.Instantanea().EjemplosEntrenamiento)
		if a.cfg.maquina != "" {
			maquina = a.cfg.maquina
		}
		if err := agregarCSV(a.cfg.resultado, filasCSV(res, modo, maquina, p.Epocas)); err != nil {
			log.Printf("no se pudo escribir %s: %v", a.cfg.resultado, err)
		} else {
			log.Printf("resultados agregados a %s", a.cfg.resultado)
		}
	}()
	responder(w, http.StatusAccepted, map[string]any{"ok": true})
}

// combinacion nombra la corrida en el CSV por su combinación exacta de
// nodos, en orden alfabético (p. ej. "tcp_tailscale:augusto-pc+josue" y
// "augusto-pc(cuda)+josue(cuda) · 2499636 ejemplos"). Así el análisis
// agrupa cada combinación por separado: dos corridas de 2 nodos con PCs
// distintas no se mezclan. El tamaño de la muestra queda anotado: cada
// muestra va en su propio CSV, porque su T(1) es distinto.
func combinacion(base string, nodos []distribuido.InfoNodo, ejemplos int) (modo, maquina string) {
	ordenados := append([]distribuido.InfoNodo(nil), nodos...)
	sort.Slice(ordenados, func(i, j int) bool { return ordenados[i].ID < ordenados[j].ID })
	ids := make([]string, len(ordenados))
	partes := make([]string, len(ordenados))
	for i, n := range ordenados {
		ids[i] = n.ID
		partes[i] = n.ID + "(" + n.Motor + ")"
		if n.Hardware != nil {
			partes[i] += "[" + n.Hardware.Resumen() + "]"
		}
	}
	return base + ":" + strings.Join(ids, "+"), fmt.Sprintf("%s · %d ejemplos", strings.Join(partes, "+"), ejemplos)
}

func (a *api) cancelarCorrida(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	cancelar := a.cancelar
	a.mu.Unlock()
	if cancelar != nil {
		cancelar()
	}
	responder(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *api) quitarNodo(w http.ResponseWriter, r *http.Request) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.ID == "" {
		fallo(w, http.StatusBadRequest, errors.New("falta el id del nodo"))
		return
	}
	if err := a.m.Quitar(p.ID); err != nil {
		fallo(w, http.StatusConflict, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *api) terminarSesion(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	if a.cancelar != nil {
		a.cancelar()
	}
	a.mu.Unlock()
	a.m.Despedir(distribuido.MotivoSesionTerminada)
	responder(w, http.StatusOK, map[string]any{"ok": true})
}

type pedidoLimpieza struct {
	Nodos       []string `json:"nodos"`
	Tecnologias []string `json:"tecnologias"` // vacío = pthreads y luego go
	Hilos       int      `json:"hilos"`       // 0 = todos los núcleos de cada nodo
}

// nuevaLimpieza empieza una limpieza o suma nodos a la que está en curso.
func (a *api) nuevaLimpieza(w http.ResponseWriter, r *http.Request) {
	var p pedidoLimpieza
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		fallo(w, http.StatusBadRequest, fmt.Errorf("pedido inválido: %w", err))
		return
	}
	if p.Hilos < 0 || p.Hilos > 1024 {
		fallo(w, http.StatusBadRequest, errors.New("los hilos deben estar entre 0 (todos) y 1024"))
		return
	}
	err := a.m.IniciarLimpieza(p.Nodos, p.Tecnologias, p.Hilos)
	switch {
	case errors.Is(err, distribuido.ErrOcupado):
		fallo(w, http.StatusConflict, err)
	case err != nil:
		fallo(w, http.StatusBadRequest, err)
	default:
		responder(w, http.StatusAccepted, map[string]any{"ok": true})
	}
}

// cancelarLimpieza: botón "Parar limpieza"; guardar = respuesta a
// "¿Quieres guardar los resultados completados?".
func (a *api) cancelarLimpieza(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Guardar bool `json:"guardar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		fallo(w, http.StatusBadRequest, fmt.Errorf("pedido inválido: %w", err))
		return
	}
	if err := a.m.CancelarLimpieza(p.Guardar); err != nil {
		fallo(w, http.StatusConflict, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"ok": true})
}

// verificarHuellas pregunta a los nodos la huella de su muestra.bin y
// devuelve el estado actualizado (antes de "Iniciar entrenamiento" o de
// "Saltar limpieza").
func (a *api) verificarHuellas(w http.ResponseWriter, r *http.Request) {
	if err := a.m.VerificarHuellas(r.Context(), 5*time.Second); err != nil {
		fallo(w, http.StatusInternalServerError, err)
		return
	}
	responder(w, http.StatusOK, a.m.Instantanea())
}

// servir es el modo servidor: la API queda escuchando hasta Ctrl+C.
func servir(ctx context.Context, direccion string, m *distribuido.Maestro, d *difusor, cfg configuracion) error {
	if t1 := m.MedirT1(5); t1 > 0 {
		log.Printf("T(1) de referencia: una época en secuencial (1 hilo) en esta PC tarda ~%.1f ms", t1)
	}

	a := &api{m: m, difusor: d, cfg: cfg, base: ctx}
	oyente, err := net.Listen("tcp", direccion)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la API en %s: %w", direccion, err)
	}
	servidor := &http.Server{Handler: a.rutas(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		log.Print("cerrando el maestro: se despide a los nodos")
		m.Despedir(distribuido.MotivoMaestroCerrando)
		apagar, listo := context.WithTimeout(context.Background(), 2*time.Second)
		defer listo()
		servidor.Shutdown(apagar)
	}()
	log.Printf("API de la página en http://%s (abre la página con: cd frontend; npm run dev)", oyente.Addr())
	d.Publicar(m.Instantanea())
	if err := servidor.Serve(oyente); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// regenerarMapa guarda el modelo recién entrenado y vuelve a exportar los
// datos del mapa y de la línea de tiempo (las probabilidades salen del
// modelo). Es lo único que sigue usando MongoDB: si no está corriendo, se
// avisa y la página conserva el mapa anterior.
func regenerarMapa(cfg configuracion, res *distribuido.Resultado, epocas int) {
	if cfg.rutaModelo == "" {
		return
	}
	modelo := dataset.ModeloGuardado{
		Features: dataset.NombresFeatures, Pesos: res.Modelo.Pesos, Sesgo: res.Modelo.Sesgo,
		Normalizador: res.Normalizador, HorizonteHoras: 1, UmbralNO2: dataset.UmbralNO2,
		Modo: cfg.modo, Epocas: epocas, F1: res.Evaluacion.F1, AUC: res.Evaluacion.AUC,
		Precision: res.Evaluacion.Precision, Exhaustividad: res.Evaluacion.Exhaustividad,
	}
	if err := modelo.Guardar(cfg.rutaModelo); err != nil {
		log.Printf("no se pudo guardar el modelo en %s: %v", cfg.rutaModelo, err)
		return
	}
	log.Printf("modelo guardado en %s (AUC %.3f)", cfg.rutaModelo, res.Evaluacion.AUC)
	if cfg.exportar == "" {
		return
	}
	tmp, err := os.MkdirTemp("", "saca-mapa-")
	if err != nil {
		log.Printf("mapa no regenerado: %v", err)
		return
	}
	defer os.RemoveAll(tmp)
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelar()
	inicio := time.Now()
	salida, err := exec.CommandContext(ctx, cfg.exportar, "-modelo", cfg.rutaModelo, "-salida", tmp).CombinedOutput()
	if err != nil {
		log.Printf("mapa no regenerado (¿MongoDB está corriendo?): %v: %s", err, ultimaLinea(string(salida)))
		return
	}
	archivos, _ := os.ReadDir(tmp)
	for _, dir := range cfg.datosPagina {
		dir = strings.TrimSpace(dir)
		if info, err := os.Stat(dir); dir == "" || err != nil || !info.IsDir() {
			continue
		}
		for _, a := range archivos {
			datos, err := os.ReadFile(filepath.Join(tmp, a.Name()))
			if err == nil {
				err = os.WriteFile(filepath.Join(dir, a.Name()), datos, 0o644)
			}
			if err != nil {
				log.Printf("no se pudo copiar %s a %s: %v", a.Name(), dir, err)
			}
		}
	}
	log.Printf("mapa regenerado con el modelo nuevo en %s (recarga el Tablero)", time.Since(inicio).Round(time.Second))
}

func ultimaLinea(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}
