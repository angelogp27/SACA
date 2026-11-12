package distribuido

import (
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"proyecto9/internal/hardware"
	"proyecto9/internal/metricas"
	"proyecto9/internal/protocolo"
	"proyecto9/internal/red"
)

// Estados de la predicción distribuida.
const (
	PrediccionEnCurso   = "prediciendo"
	PrediccionTerminada = "terminada"
	PrediccionError     = "error"
)

// ModeloEntrenado es el último modelo que terminó de entrenar el clúster:
// con él se hace la predicción distribuida (Fase 5).
type ModeloEntrenado struct {
	Nombre     string    // "logistica" o "red(64-32)"
	Capas      []int     // nil = regresión logística
	Params     []float64 // red: el vector; logística: pesos y el sesgo al final
	Evaluacion metricas.Clasificacion
	Huella     string // de la muestra con que se entrenó
}

// EstadoPrediccion: la predicción distribuida vista desde la página.
type EstadoPrediccion struct {
	ID      int              `json:"id"`
	Estado  string           `json:"estado"`
	Mensaje string           `json:"mensaje,omitempty"`
	Modelo  string           `json:"modelo"`
	Inicio  time.Time        `json:"inicio"`
	TTotalS float64          `json:"t_total_s"`
	Filas   int              `json:"filas"`
	MB      float64          `json:"mb"`
	Nodos   []NodoPrediccion `json:"nodos"`
}

// NodoPrediccion: la parte de un nodo. TCalculoMs es lo que tardó en
// predecir sus filas; TTotalMs, hasta que llegó su último trozo.
type NodoPrediccion struct {
	ID         string         `json:"id"`
	Hardware   *hardware.Info `json:"hardware,omitempty"`
	Estado     string         `json:"estado"`
	Filas      int            `json:"filas"`
	Total      int            `json:"total"`
	MB         float64        `json:"mb"`
	TCalculoMs float64        `json:"t_calculo_ms"`
	TTotalMs   float64        `json:"t_total_ms"`
	Mensaje    string         `json:"mensaje,omitempty"`
}

func (e *EstadoPrediccion) copia() *EstadoPrediccion {
	if e == nil {
		return nil
	}
	c := *e
	c.Nodos = slices.Clone(e.Nodos)
	return &c
}

// prediccionEnCurso requiere m.mu.
func (m *Maestro) prediccionEnCurso() bool {
	return m.prediccion != nil && m.prediccion.Estado == PrediccionEnCurso
}

// IniciarPrediccion reparte las estaciones entre los nodos (como en el
// entrenamiento) y les pide predecir todas sus horas con el último modelo.
func (m *Maestro) IniciarPrediccion(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.activa != nil:
		return ErrOcupado
	case m.limpiezaEnCurso():
		return ErrLimpiando
	case m.prediccionEnCurso():
		return errors.New("ya hay una predicción en curso")
	case m.ultimoModelo == nil:
		return errors.New("todavía no hay un modelo entrenado: entrena primero")
	case m.datos == nil || m.ultimoModelo.Huella != m.huella:
		return errors.New("la muestra cambió desde el último entrenamiento: vuelve a entrenar")
	}
	if len(ids) == 0 {
		for _, n := range m.ordenSala() {
			ids = append(ids, n.vista.ID)
		}
	}
	if len(ids) == 0 {
		return ErrSinNodos
	}
	var nodos []*nodoSala
	for _, id := range ids {
		n, ok := m.nodos[id]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNodoNoDisponible, id)
		}
		if n.vista.Huella != m.huella {
			return fmt.Errorf("%w: %s no tiene la misma muestra.bin que el maestro", ErrMuestraDistinta, id)
		}
		nodos = append(nodos, n)
	}
	m.siguientePrediccion++
	e := &EstadoPrediccion{ID: m.siguientePrediccion, Estado: PrediccionEnCurso, Modelo: m.ultimoModelo.Nombre, Inicio: time.Now()}
	m.predFilas = map[string][]FilaPrediccion{}
	m.predLista = listaEstaciones(m.datos)
	params := red.Codificar(m.ultimoModelo.Params)
	for i, n := range nodos {
		e.Nodos = append(e.Nodos, NodoPrediccion{ID: n.vista.ID, Hardware: n.vista.Hardware, Estado: PrediccionEnCurso})
		err := n.conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoPredecir, WorkerID: n.vista.ID, Corrida: e.ID,
			FiltroMongo: &protocolo.FiltroMongo{Mod: len(nodos), Resto: i}, Capas: m.ultimoModelo.Capas, ParamsBin: params})
		if err != nil {
			e.Nodos[i].Estado, e.Nodos[i].Mensaje = PrediccionError, err.Error()
		}
	}
	m.prediccion = e
	log.Printf("predicción %d: %d nodos predicen todas las horas de sus estaciones con el modelo %s", e.ID, len(nodos), e.Modelo)
	go m.publicar()
	return nil
}

// mensajePrediccion procesa un trozo de predicciones de un nodo.
func (m *Maestro) mensajePrediccion(id string, msg protocolo.Mensaje) {
	m.mu.Lock()
	e := m.prediccion
	if e == nil || e.ID != msg.Corrida || e.Estado != PrediccionEnCurso {
		m.mu.Unlock()
		return
	}
	var n *NodoPrediccion
	for i := range e.Nodos {
		if e.Nodos[i].ID == id {
			n = &e.Nodos[i]
		}
	}
	if n == nil || n.Estado != PrediccionEnCurso {
		m.mu.Unlock()
		return
	}
	if msg.Motivo != "" {
		n.Estado, n.Mensaje = PrediccionError, msg.Motivo
	} else if filas, err := decodificarPredicciones(msg.PrediccionesBin, m.predLista); err != nil {
		n.Estado, n.Mensaje = PrediccionError, err.Error()
	} else {
		m.predFilas[id] = append(m.predFilas[id], filas...)
		n.Filas += len(filas)
		n.Total = msg.Total
		n.MB += float64(len(msg.PrediccionesBin)) / (1 << 20)
		n.TCalculoMs = msg.TCalculoMs
		n.TTotalMs = ms(time.Since(e.Inicio))
		e.Filas += len(filas)
		e.MB += float64(len(msg.PrediccionesBin)) / (1 << 20)
		if msg.Ultimo {
			n.Estado = PrediccionTerminada
		}
	}
	termino := m.cerrarPrediccionSiTermino()
	m.mu.Unlock()
	m.publicar()
	if termino {
		m.prediccionCerrada()
	}
}

// cerrarPrediccionSiTermino: requiere m.mu.
func (m *Maestro) cerrarPrediccionSiTermino() bool {
	e := m.prediccion
	if e == nil || e.Estado != PrediccionEnCurso {
		return false
	}
	errores := 0
	for _, n := range e.Nodos {
		switch n.Estado {
		case PrediccionEnCurso:
			return false
		case PrediccionError:
			errores++
		}
	}
	e.TTotalS = time.Since(e.Inicio).Seconds()
	e.Estado = PrediccionTerminada
	if errores > 0 {
		e.Estado, e.Mensaje = PrediccionError, fmt.Sprintf("%d nodo(s) no terminaron: el mapa no se actualiza con una predicción incompleta", errores)
	}
	return true
}

func (m *Maestro) prediccionCerrada() {
	m.mu.Lock()
	e := m.prediccion.copia()
	var filas []FilaPrediccion
	if e.Estado == PrediccionTerminada {
		for _, n := range e.Nodos {
			filas = append(filas, m.predFilas[n.ID]...)
		}
	}
	modelo := m.ultimoModelo
	m.predFilas = nil
	m.mu.Unlock()
	log.Printf("predicción %d %s: %d filas, %.1f MB por la red, %.2f s", e.ID, e.Estado, e.Filas, e.MB, e.TTotalS)
	if e.Estado == PrediccionTerminada && m.cfg.PrediccionTerminada != nil {
		go m.cfg.PrediccionTerminada(*e, filas, *modelo)
	}
}

// nodoSalioDePrediccion marca como error al nodo que se desconectó.
func (m *Maestro) nodoSalioDePrediccion(id, motivo string) {
	m.mu.Lock()
	if !m.prediccionEnCurso() {
		m.mu.Unlock()
		return
	}
	for i := range m.prediccion.Nodos {
		if n := &m.prediccion.Nodos[i]; n.ID == id && n.Estado == PrediccionEnCurso {
			n.Estado, n.Mensaje = PrediccionError, "nodo desconectado: "+motivo
		}
	}
	termino := m.cerrarPrediccionSiTermino()
	m.mu.Unlock()
	m.publicar()
	if termino {
		m.prediccionCerrada()
	}
}
