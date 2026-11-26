package distribuido

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"slices"
	"strings"
	"time"

	"proyecto9/internal/hardware"
	"proyecto9/internal/metricas"
	"proyecto9/internal/pares"
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
	// Fase 6: con qué calculó cada nodo y por dónde se juntó ("tcp" o "mpi").
	MotorCPU     string `json:"motor_cpu,omitempty"`
	CUDA         bool   `json:"cuda,omitempty"`
	Comunicacion string `json:"comunicacion,omitempty"`
	// Comparación: Motores de todo el pedido (uno tras otro, como en el
	// entrenamiento) y las predicciones Anteriores del pedido, ya terminadas.
	Motores    []string            `json:"motores,omitempty"`
	Anteriores []*EstadoPrediccion `json:"anteriores,omitempty"`
}

// restoPrediccion: lo que falta de un pedido con varios motores.
type restoPrediccion struct {
	ids []string
	op  OpcionesPrediccion // con indice en el motor que sigue
}

// OpcionesPrediccion de la Fase 6; en cero, Go y TCP.
type OpcionesPrediccion struct {
	MotorCPU     string // "go" (por defecto) u "openmp"
	CUDA         bool
	Comunicacion string // "tcp" (por defecto) o "mpi"
	// Comparacion: motores que se corren uno tras otro sobre la misma
	// predicción ("openmp", "go"); si hay, manda sobre MotorCPU.
	Comparacion []string
	indice      int // el motor de Comparacion que toca
	anteriores  []*EstadoPrediccion
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
	Motor      string         `json:"motor,omitempty"`
	ParteGPU   float64        `json:"parte_gpu,omitempty"`
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
func (m *Maestro) IniciarPrediccion(ids []string, op OpcionesPrediccion) error {
	if op.indice < len(op.Comparacion) {
		op.MotorCPU = op.Comparacion[op.indice]
	}
	if op.MotorCPU == "" {
		op.MotorCPU = MotorCPUGo
	}
	if op.Comunicacion == "" {
		op.Comunicacion = "tcp"
	}
	if !motorCPUValido(op.MotorCPU) || (op.Comunicacion != "tcp" && op.Comunicacion != pares.MPI) {
		return fmt.Errorf("opciones de predicción inválidas (%q, %q)", op.MotorCPU, op.Comunicacion)
	}
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
	if err := conMotor(nodos, op.MotorCPU); err != nil {
		return err
	}
	var mpi *pares.LanzadoMPI
	var dirs, rangos []string
	if op.Comunicacion == pares.MPI {
		if err := soloWindowsMPI(nodos); err != nil {
			return err
		}
		// El nodo del maestro, si participa, es el rango 0: el que junta.
		slices.SortStableFunc(nodos, func(a, b *nodoSala) int {
			return btoi(esLoopback(soloIP(b.vista.Remoto))) - btoi(esLoopback(soloIP(a.vista.Remoto)))
		})
		dirs = direcciones(nodos)
		for _, n := range nodos {
			rangos = append(rangos, n.vista.ID)
		}
		var err error
		if mpi, err = pares.LanzarMPI(m.siguientePrediccion+1+corridaPrediccion, nodosMPI(nodos)); err != nil {
			return err
		}
	}
	m.siguientePrediccion++
	m.predMPI = mpi
	e := &EstadoPrediccion{ID: m.siguientePrediccion, Estado: PrediccionEnCurso, Modelo: m.ultimoModelo.Nombre, Inicio: time.Now(),
		MotorCPU: op.MotorCPU, CUDA: op.CUDA, Comunicacion: op.Comunicacion, Motores: op.Comparacion, Anteriores: op.anteriores}
	m.predResto = nil
	if op.indice+1 < len(op.Comparacion) {
		sig := op
		sig.indice++
		m.predResto = &restoPrediccion{ids: ids, op: sig}
	}
	m.predFilas = map[string][]FilaPrediccion{}
	m.predLista = listaEstaciones(m.datos)
	params := red.Codificar(m.ultimoModelo.Params)
	for i, n := range nodos {
		e.Nodos = append(e.Nodos, NodoPrediccion{ID: n.vista.ID, Hardware: n.vista.Hardware, Estado: PrediccionEnCurso})
		err := n.conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoPredecir, WorkerID: n.vista.ID, Corrida: e.ID,
			FiltroMongo: &protocolo.FiltroMongo{Mod: len(nodos), Resto: i}, Capas: m.ultimoModelo.Capas, ParamsBin: params,
			MotorCPU: op.MotorCPU, CUDA: op.CUDA, Comunicacion: op.Comunicacion, Rango: i, Pares: dirs, Nodos: rangos})
		if err != nil {
			e.Nodos[i].Estado, e.Nodos[i].Mensaje = PrediccionError, err.Error()
		}
	}
	m.prediccion = e
	if mpi != nil {
		go m.vigilarMPI(e.ID, mpi)
	}
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
	if msg.Nodo != "" && e.Comunicacion == pares.MPI {
		id = msg.Nodo // el rango 0 entrega lo de todos
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
		if m.predMPI != nil { // los demás están esperándolo en el Gatherv
			go m.predMPI.Detener()
		}
	} else if filas, err := decodificarPredicciones(msg.PrediccionesBin, m.predLista); err != nil {
		n.Estado, n.Mensaje = PrediccionError, err.Error()
	} else {
		m.predFilas[id] = append(m.predFilas[id], filas...)
		n.Filas += len(filas)
		n.Total = msg.Total
		n.MB += float64(len(msg.PrediccionesBin)) / (1 << 20)
		n.TCalculoMs, n.Motor, n.ParteGPU = msg.TCalculoMs, msg.Motor, msg.ParteGPU
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
	mpi := m.predMPI
	m.predMPI = nil
	resto := m.predResto
	m.predResto = nil
	m.mu.Unlock()
	log.Printf("predicción %d %s (%s): %d filas, %.1f MB por la red, %.2f s", e.ID, e.Estado, e.MotorCPU, e.Filas, e.MB, e.TTotalS)
	if mpi != nil {
		// El siguiente motor lanza su propio mpiexec: este debe acabar antes.
		mpi.Detener()
	}
	// Comparación: el siguiente motor, sobre los mismos nodos. El mapa y el
	// historial se actualizan una sola vez, al final, con todo el pedido.
	if e.Estado == PrediccionTerminada && resto != nil {
		op := resto.op
		op.anteriores = append(append([]*EstadoPrediccion{}, e.Anteriores...), e)
		if err := m.IniciarPrediccion(resto.ids, op); err != nil {
			log.Printf("no se pudo seguir la predicción con el motor %s: %v", op.Comparacion[op.indice], err)
		} else {
			return
		}
	}
	if e.Estado == PrediccionTerminada && m.cfg.PrediccionTerminada != nil {
		go m.cfg.PrediccionTerminada(*e, filas, *modelo)
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// vigilarMPI: si mpiexec falla con la predicción en curso, no va a terminar.
func (m *Maestro) vigilarMPI(id int, mpi *pares.LanzadoMPI) {
	<-mpi.Fin
	err := mpi.Err()
	if err == nil {
		return
	}
	m.mu.Lock()
	e := m.prediccion
	if e == nil || e.ID != id || e.Estado != PrediccionEnCurso {
		m.mu.Unlock()
		return
	}
	for i := range e.Nodos {
		if n := &e.Nodos[i]; n.Estado == PrediccionEnCurso {
			n.Estado, n.Mensaje = PrediccionError, err.Error()
		}
	}
	termino := m.cerrarPrediccionSiTermino()
	m.mu.Unlock()
	m.publicar()
	if termino {
		m.prediccionCerrada()
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

// prediccionMPISoloWindows: la predicción con MPI se usa solo entre laptops
// Windows (MS-MPI), con el maestro también en Windows. Las pruebas en Linux
// lo apagan.
var prediccionMPISoloWindows = true

func soloWindowsMPI(nodos []*nodoSala) error {
	if !prediccionMPISoloWindows {
		return nil
	}
	if runtime.GOOS != "windows" {
		return errors.New("la predicción con MPI es solo para Windows: el maestro debe ser una laptop Windows (usa TCP)")
	}
	for _, n := range nodos {
		if n.vista.Hardware == nil || !strings.HasPrefix(n.vista.Hardware.SO, "windows") {
			return fmt.Errorf("la predicción con MPI es solo para Windows: quita %s o usa TCP", n.vista.ID)
		}
	}
	return nil
}
