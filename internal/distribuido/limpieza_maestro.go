package distribuido

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"proyecto9/internal/hardware"
	"proyecto9/internal/protocolo"
)

// Estados de una limpieza y de cada tecnología dentro de ella.
const (
	LimpiezaPendiente = "pendiente"
	LimpiezaEnCurso   = "limpiando"
	LimpiezaTerminada = "terminada"
	LimpiezaError     = "error"
	LimpiezaCancelada = "cancelada"
)

// TecnologiasLimpieza son las que sabe ejecutar un nodo, en el orden en
// que se corren (una tras otra, para no pelear por la CPU).
var TecnologiasLimpieza = []string{"pthreads", "go"}

// maxPuntosSerie acota la serie de la gráfica (≈ 4 puntos por segundo).
const maxPuntosSerie = 4000

// EstadoLimpieza es una limpieza vista desde la página: por nodo y por
// tecnología, el avance, la serie (avance, % de CPU) y el tiempo total.
type EstadoLimpieza struct {
	ID          int            `json:"id"`
	Estado      string         `json:"estado"`
	Mensaje     string         `json:"mensaje,omitempty"`
	Inicio      time.Time      `json:"inicio"`
	Tecnologias []string       `json:"tecnologias"`
	Hilos       int            `json:"hilos"` // 0 = todos los núcleos de cada nodo
	Nodos       []NodoLimpieza `json:"nodos"`
}

// NodoLimpieza es la parte de un nodo en la limpieza.
type NodoLimpieza struct {
	ID          string               `json:"id"`
	Hardware    *hardware.Info       `json:"hardware,omitempty"`
	Tecnologias []TecnologiaLimpieza `json:"tecnologias"`
}

// TecnologiaLimpieza es una tecnología en un nodo. Serie son pares
// [avance 0-1, % de CPU del nodo].
type TecnologiaLimpieza struct {
	Tecnologia string       `json:"tecnologia"`
	Estado     string       `json:"estado"`
	Hilos      int          `json:"hilos"`
	Avance     float64      `json:"avance"`
	Serie      [][2]float64 `json:"serie"`
	TTotalS    float64      `json:"t_total_s"`
	Huella     string       `json:"huella,omitempty"`
	Mensaje    string       `json:"mensaje,omitempty"`
}

func final(estado string) bool {
	return estado == LimpiezaTerminada || estado == LimpiezaError || estado == LimpiezaCancelada
}

func (e *EstadoLimpieza) copia() *EstadoLimpieza {
	if e == nil {
		return nil
	}
	c := *e
	c.Tecnologias = slices.Clone(e.Tecnologias)
	c.Nodos = make([]NodoLimpieza, len(e.Nodos))
	for i, n := range e.Nodos {
		n.Tecnologias = slices.Clone(n.Tecnologias)
		for j := range n.Tecnologias {
			n.Tecnologias[j].Serie = append(make([][2]float64, 0, len(n.Tecnologias[j].Serie)), n.Tecnologias[j].Serie...)
		}
		c.Nodos[i] = n
	}
	return &c
}

func (e *EstadoLimpieza) nodo(id string) *NodoLimpieza {
	for i := range e.Nodos {
		if e.Nodos[i].ID == id {
			return &e.Nodos[i]
		}
	}
	return nil
}

func (n *NodoLimpieza) tecnologia(t string) *TecnologiaLimpieza {
	for i := range n.Tecnologias {
		if n.Tecnologias[i].Tecnologia == t {
			return &n.Tecnologias[i]
		}
	}
	return nil
}

func (n *NodoLimpieza) pendiente() bool {
	for _, t := range n.Tecnologias {
		if !final(t.Estado) {
			return true
		}
	}
	return false
}

// limpiezaEnCurso: requiere m.mu.
func (m *Maestro) limpiezaEnCurso() bool {
	return m.limpieza != nil && m.limpieza.Estado == LimpiezaEnCurso
}

// limpiandoNodo: el nodo tiene alguna tecnología sin terminar. Requiere m.mu.
func (m *Maestro) limpiandoNodo(id string) bool {
	if !m.limpiezaEnCurso() {
		return false
	}
	n := m.limpieza.nodo(id)
	return n != nil && n.pendiente()
}

// IniciarLimpieza pide a los nodos que limpien sus CSV con cada tecnología
// y generen su muestra.bin. Si ya hay una limpieza en curso, los nodos se
// suman a ella (los que llegan tarde también participan). tecnologias
// vacío = Pthreads y luego Go; hilos 0 = todos los núcleos de cada nodo.
func (m *Maestro) IniciarLimpieza(ids, tecnologias []string, hilos int) error {
	if len(ids) == 0 {
		return ErrSinNodos
	}
	if len(tecnologias) == 0 {
		tecnologias = TecnologiasLimpieza
	}
	for _, t := range tecnologias {
		if !slices.Contains(TecnologiasLimpieza, t) {
			return fmt.Errorf("tecnología desconocida %q (usa %v)", t, TecnologiasLimpieza)
		}
	}

	m.mu.Lock()
	if m.activa != nil {
		m.mu.Unlock()
		return ErrOcupado
	}
	if !m.limpiezaEnCurso() {
		m.siguienteLimpieza++
		m.limpieza = &EstadoLimpieza{ID: m.siguienteLimpieza, Estado: LimpiezaEnCurso, Inicio: time.Now(), Tecnologias: slices.Clone(tecnologias), Hilos: hilos}
	}
	e := m.limpieza
	type envio struct {
		id string
		c  *protocolo.Conexion
	}
	var envios []envio
	for _, id := range ids {
		n, ok := m.nodos[id]
		if !ok {
			m.mu.Unlock()
			return fmt.Errorf("%w: %s", ErrNodoNoDisponible, id)
		}
		if e.nodo(id) != nil {
			continue // ya está en esta limpieza
		}
		nl := NodoLimpieza{ID: id, Hardware: n.vista.Hardware}
		for _, t := range e.Tecnologias {
			nl.Tecnologias = append(nl.Tecnologias, TecnologiaLimpieza{Tecnologia: t, Estado: LimpiezaPendiente, Serie: [][2]float64{}})
		}
		e.Nodos = append(e.Nodos, nl)
		envios = append(envios, envio{id, n.conexion})
	}
	tecs, h, idLimpieza := slices.Clone(e.Tecnologias), e.Hilos, e.ID
	m.mu.Unlock()

	if len(envios) == 0 {
		return errors.New("esos nodos ya están en la limpieza en curso")
	}
	for _, env := range envios {
		err := env.c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoIniciarLimpieza, WorkerID: env.id, Corrida: idLimpieza, Tecnologias: tecs, Hilos: h})
		if err != nil {
			m.nodoSalioDeLimpieza(env.id, "no se le pudo enviar la orden: "+err.Error())
		}
		log.Printf("limpieza %d: %s empieza (%v)", idLimpieza, env.id, tecs)
	}
	m.publicar()
	return nil
}

// CancelarLimpieza para la limpieza en curso (botón "Parar limpieza").
// Las tecnologías ya terminadas conservan su resultado; con guardar, sus
// tiempos se guardan (LimpiezaTerminada).
func (m *Maestro) CancelarLimpieza(guardar bool) error {
	m.mu.Lock()
	if !m.limpiezaEnCurso() {
		m.mu.Unlock()
		return errors.New("no hay una limpieza en curso")
	}
	e := m.limpieza
	e.Estado, e.Mensaje = LimpiezaCancelada, "parada desde la página"
	var conexiones []*protocolo.Conexion
	for i := range e.Nodos {
		n := &e.Nodos[i]
		if !n.pendiente() {
			continue
		}
		for j := range n.Tecnologias {
			if !final(n.Tecnologias[j].Estado) {
				n.Tecnologias[j].Estado = LimpiezaCancelada
			}
		}
		if s, ok := m.nodos[n.ID]; ok {
			conexiones = append(conexiones, s.conexion)
		}
	}
	copia, id := e.copia(), e.ID
	m.mu.Unlock()

	for _, c := range conexiones {
		c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoCancelarLimpieza, Corrida: id})
	}
	log.Printf("limpieza %d parada (guardar resultados: %v)", id, guardar)
	m.publicar()
	if m.cfg.LimpiezaTerminada != nil {
		m.cfg.LimpiezaTerminada(*copia, guardar)
	}
	go m.recargar()
	return nil
}

// mensajeLimpieza procesa un progreso_limpieza o limpieza_terminada.
func (m *Maestro) mensajeLimpieza(id string, msg protocolo.Mensaje) {
	m.mu.Lock()
	// La huella nueva del nodo vale aunque la limpieza ya se haya parado:
	// su muestra.bin ya cambió.
	if msg.Tipo == protocolo.TipoLimpiezaTerminada && msg.Motivo == "" && msg.Huella != "" {
		if n, ok := m.nodos[id]; ok {
			n.vista.Huella = msg.Huella
		}
	}
	e := m.limpieza
	if e == nil || e.ID != msg.Corrida || e.Estado != LimpiezaEnCurso {
		m.mu.Unlock()
		m.publicar()
		return
	}
	n := e.nodo(id)
	var t *TecnologiaLimpieza
	if n != nil {
		t = n.tecnologia(msg.Tecnologia)
	}
	if t == nil || final(t.Estado) {
		m.mu.Unlock()
		return
	}
	switch msg.Tipo {
	case protocolo.TipoProgresoLimpieza:
		t.Estado, t.Hilos, t.Avance = LimpiezaEnCurso, msg.Hilos, msg.Avance
		if len(t.Serie) < maxPuntosSerie {
			t.Serie = append(t.Serie, [2]float64{msg.Avance, msg.CPU})
		}
	case protocolo.TipoLimpiezaTerminada:
		t.Hilos = max(t.Hilos, msg.Hilos)
		if msg.Motivo != "" {
			t.Estado, t.Mensaje = LimpiezaError, msg.Motivo
		} else {
			t.Estado, t.Avance, t.TTotalS, t.Huella = LimpiezaTerminada, 1, msg.TTotalS, msg.Huella
			log.Printf("limpieza %d: %s terminó %s en %.2f s con %d hilos (huella %s)", e.ID, id, t.Tecnologia, msg.TTotalS, t.Hilos, Corta(msg.Huella))
		}
	}
	termino := m.cerrarSiTermino()
	copia := e.copia()
	m.mu.Unlock()
	m.publicar()
	if termino {
		m.limpiezaCerrada(copia)
	}
}

// cerrarSiTermino marca la limpieza como terminada si ya no queda nada
// por hacer. Requiere m.mu.
func (m *Maestro) cerrarSiTermino() bool {
	e := m.limpieza
	if e == nil || e.Estado != LimpiezaEnCurso {
		return false
	}
	for _, n := range e.Nodos {
		if n.pendiente() {
			return false
		}
	}
	e.Estado = LimpiezaTerminada
	return true
}

func (m *Maestro) limpiezaCerrada(e *EstadoLimpieza) {
	log.Printf("limpieza %d terminada", e.ID)
	if m.cfg.LimpiezaTerminada != nil {
		m.cfg.LimpiezaTerminada(*e, true)
	}
	// El nodo del maestro (si participó) cambió data/muestra.bin.
	go m.recargar()
}

// nodoSalioDeLimpieza marca como error lo que le faltaba a un nodo que se
// desconectó.
func (m *Maestro) nodoSalioDeLimpieza(id, motivo string) {
	m.mu.Lock()
	if !m.limpiezaEnCurso() {
		m.mu.Unlock()
		return
	}
	n := m.limpieza.nodo(id)
	if n == nil || !n.pendiente() {
		m.mu.Unlock()
		return
	}
	for j := range n.Tecnologias {
		if !final(n.Tecnologias[j].Estado) {
			n.Tecnologias[j].Estado, n.Tecnologias[j].Mensaje = LimpiezaError, "nodo desconectado: "+motivo
		}
	}
	termino := m.cerrarSiTermino()
	copia := m.limpieza.copia()
	m.mu.Unlock()
	m.publicar()
	if termino {
		m.limpiezaCerrada(copia)
	}
}

// huellaRecibida actualiza la huella de un nodo (respuesta a
// consultar_huella, o aviso del nodo al recibir su partición). Si el nodo
// está en un entrenamiento y su muestra no es la de la corrida, la corrida
// no puede seguir.
func (m *Maestro) huellaRecibida(id, huella string) {
	m.mu.Lock()
	if n, ok := m.nodos[id]; ok {
		n.vista.Huella = huella
	}
	delete(m.consultas, id)
	activa := m.activa
	m.mu.Unlock()
	if activa != nil && huella != activa.huella {
		if _, participa := activa.indice[id]; participa {
			activa.avisar(evento{tipo: caida, id: id, motivo: "muestra_distinta: su muestra.bin cambió (" + cortaOVacia(huella) + ")"})
		}
	}
	m.publicar()
}

// VerificarHuellas pregunta a cada nodo libre la huella de su muestra.bin
// (por si cambió desde que se conectó) y relee la del maestro. Espera las
// respuestas hasta el plazo; quien no responde conserva la que tenía.
func (m *Maestro) VerificarHuellas(ctx context.Context, plazo time.Duration) error {
	m.mu.Lock()
	var conexiones []*protocolo.Conexion
	for id, n := range m.nodos {
		if m.ocupados[id] || m.limpiandoNodo(id) {
			continue
		}
		m.consultas[id] = true
		conexiones = append(conexiones, n.conexion)
	}
	m.mu.Unlock()
	for _, c := range conexiones {
		c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoConsultarHuella})
	}
	err := m.recargar()
	limite := time.Now().Add(plazo)
	for time.Now().Before(limite) {
		m.mu.Lock()
		faltan := 0
		for id := range m.consultas {
			if _, sigue := m.nodos[id]; sigue {
				faltan++
			} else {
				delete(m.consultas, id)
			}
		}
		m.mu.Unlock()
		if faltan == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	m.publicar()
	return err
}
