package distribuido

import "time"

// Instantanea es el estado del maestro que ve la página: la sala de
// espera, la corrida en curso (o la última) con sus series por época, y el
// historial de corridas de la sesión. Se serializa tal cual a JSON.
type Instantanea struct {
	Huella                string           `json:"huella"`
	EjemplosEntrenamiento int              `json:"ejemplos_entrenamiento"`
	Capacidad             int              `json:"capacidad"`
	T1EpocaMs             float64          `json:"t1_epoca_ms"` // una época en secuencial (1 hilo) en la PC del maestro
	Ocupado               bool             `json:"ocupado"`     // hay una corrida en curso
	Sala                  []NodoVista      `json:"sala"`
	Corrida               *EstadoCorrida   `json:"corrida"`
	Historial             []ResumenCorrida `json:"historial"`
}

// NodoVista es un nodo conectado al maestro.
type NodoVista struct {
	ID             string    `json:"id"`
	Motor          string    `json:"motor"`
	Dispositivo    string    `json:"dispositivo"`
	Remoto         string    `json:"remoto"`
	Estado         string    `json:"estado"` // "libre" o "en_corrida"
	ConectadoDesde time.Time `json:"conectado_desde"`
}

// Estados de una corrida.
const (
	EstadoEntrenando  = "entrenando"
	EstadoVerificando = "verificando"
	EstadoTerminado   = "terminado"
	EstadoAbortado    = "abortado"
	EstadoCancelado   = "cancelado"
)

// EstadoCorrida es una corrida vista época a época.
type EstadoCorrida struct {
	ID      int    `json:"id"`
	Estado  string `json:"estado"`
	Mensaje string `json:"mensaje,omitempty"`

	Nodos  []NodoCorrida `json:"nodos"`
	Epocas int           `json:"epocas"`
	Epoca  int           `json:"epoca"` // épocas completadas
	Inicio time.Time     `json:"inicio"`

	Perdidas   []float64 `json:"perdidas"`
	TEpocaMs   []float64 `json:"t_epoca_ms"`
	TCalculoMs []float64 `json:"t_calculo_ms"` // del nodo más lento en cada época
	TRedMs     []float64 `json:"t_red_ms"`

	Resultado *ResumenCorrida `json:"resultado,omitempty"`
}

// NodoCorrida es la parte de un nodo en una corrida.
type NodoCorrida struct {
	ID          string    `json:"id"`
	Motor       string    `json:"motor"`
	Dispositivo string    `json:"dispositivo"`
	Remoto      string    `json:"remoto"`
	Particion   int       `json:"particion"`
	Ejemplos    int       `json:"ejemplos"`
	CalculoMs   []float64 `json:"calculo_ms"` // por época
}

// ResumenCorrida es una fila del historial (y el resultado de una corrida).
type ResumenCorrida struct {
	ID              int      `json:"id"`
	Nodos           []string `json:"nodos"`
	Motores         []string `json:"motores"`
	Epocas          int      `json:"epocas"`
	Estado          string   `json:"estado"`
	TTotalS         float64  `json:"t_total_s"`
	TEpocaMs        float64  `json:"t_epoca_ms"`
	TCalculoMs      float64  `json:"t_calculo_ms"`
	TComunicacionMs float64  `json:"t_comunicacion_ms"`
	Speedup         float64  `json:"speedup"` // contra T1EpocaMs × épocas; 0 si no se conoce
	Eficiencia      float64  `json:"eficiencia"`
	PerdidaFinal    float64  `json:"perdida_final"`
	AUC             float64  `json:"auc"`
	F1              float64  `json:"f1"`
	Verificado      bool     `json:"verificado"`
	Diferencia      float64  `json:"diferencia"`     // |pérdida distribuida − secuencial|
	TSecuencialS    float64  `json:"t_secuencial_s"` // si se verificó
}

func copiarSerie(s []float64) []float64 { return append([]float64(nil), s...) }

func (e *EstadoCorrida) copia() *EstadoCorrida {
	if e == nil {
		return nil
	}
	c := *e
	c.Perdidas = copiarSerie(e.Perdidas)
	c.TEpocaMs = copiarSerie(e.TEpocaMs)
	c.TCalculoMs = copiarSerie(e.TCalculoMs)
	c.TRedMs = copiarSerie(e.TRedMs)
	c.Nodos = make([]NodoCorrida, len(e.Nodos))
	for i, n := range e.Nodos {
		n.CalculoMs = copiarSerie(n.CalculoMs)
		c.Nodos[i] = n
	}
	if e.Resultado != nil {
		r := *e.Resultado
		c.Resultado = &r
	}
	return &c
}
