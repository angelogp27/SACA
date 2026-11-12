// Package protocolo implementa los mensajes del protocolo TCP
// maestro-trabajador de docs/contratos/contrato_protocolo_tcp.md: un objeto JSON por
// línea, con un campo "tipo" que identifica su forma. Es el equivalente
// del send/recv de MPI, pero sobre conexiones TCP entre procesos (o
// contenedores) que no comparten memoria.
package protocolo

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"proyecto9/internal/hardware"
)

const (
	TipoRegistro                = "registro"
	TipoParticionAsignada       = "particion_asignada"
	TipoParametros              = "parametros"
	TipoGradienteParcial        = "gradiente_parcial"
	TipoEntrenamientoFinalizado = "entrenamiento_finalizado"
	TipoBaja                    = "baja"

	// Limpieza (docs/contratos/contrato_protocolo_tcp.md, "Limpieza"): cada nodo
	// limpia sus propios CSV y genera su data/muestra.bin; por la red solo
	// viajan avisos, avances y huellas.
	TipoIniciarLimpieza   = "iniciar_limpieza"   // maestro → trabajador
	TipoCancelarLimpieza  = "cancelar_limpieza"  // maestro → trabajador
	TipoProgresoLimpieza  = "progreso_limpieza"  // trabajador → maestro
	TipoLimpiezaTerminada = "limpieza_terminada" // trabajador → maestro, una por tecnología
	TipoConsultarHuella   = "consultar_huella"   // maestro → trabajador
	TipoHuellaMuestra     = "huella_muestra"     // trabajador → maestro

	// Predicción distribuida (Fase 5): el maestro manda el modelo y la
	// partición; cada nodo predice todas las horas de sus estaciones y le
	// devuelve los resultados en trozos binarios (el gather).
	TipoPredecir     = "predecir"     // maestro → trabajador
	TipoPredicciones = "predicciones" // trabajador → maestro, uno por trozo
)

// FiltroMongo indica qué porción de los datos le toca a un trabajador:
// las estaciones cuyo hash (internal/particion) módulo Mod es Resto.
//
// Con Restos (reparto proporcional) el nodo recibe varias cubetas del hash y
// no una sola: es el scatter con tamaños distintos (MPI_Scatterv).
type FiltroMongo struct {
	Mod    int   `json:"estacion_id_hash_mod"`
	Resto  int   `json:"estacion_id_hash_resto"`
	Restos []int `json:"estacion_id_hash_restos,omitempty"`
}

// Mensaje es la unión de todos los tipos del contrato; cada tipo usa solo
// sus campos y el resto se omite al serializar.
type Mensaje struct {
	Tipo     string `json:"tipo"`
	WorkerID string `json:"worker_id,omitempty"`

	// registro: huella (SHA-256) de la muestra que cargó el nodo, para que
	// el maestro rechace a quien tenga otros datos, y con qué calcula.
	Huella      string `json:"huella,omitempty"`
	Motor       string `json:"motor,omitempty"`
	Dispositivo string `json:"dispositivo,omitempty"`
	// Hardware es la huella de la máquina (CPU, núcleos, RAM, GPU): con
	// ella el maestro deja constancia de en qué equipos se midió.
	Hardware *hardware.Info `json:"hardware,omitempty"`

	// particion_asignada
	FiltroMongo *FiltroMongo `json:"filtro_mongo,omitempty"`

	// parametros y gradiente_parcial. Corrida identifica el entrenamiento
	// (el maestro en modo servidor lanza varios con los mismos nodos): el
	// trabajador la repite en su gradiente para que el maestro descarte uno
	// atrasado de una corrida anterior.
	Corrida        int       `json:"corrida,omitempty"`
	Epoca          int       `json:"epoca,omitempty"`
	Pesos          []float64 `json:"pesos,omitempty"`
	Sesgo          float64   `json:"sesgo,omitempty"`
	Gradiente      []float64 `json:"gradiente,omitempty"`
	GradienteSesgo float64   `json:"gradiente_sesgo,omitempty"`
	NMuestras      int       `json:"n_muestras,omitempty"`
	// Perdida es la suma de la pérdida ponderada sobre la partición y
	// TCalculoMs lo que tardó el nodo en calcular, sin la red: el maestro
	// los usa para la pérdida de la época y para separar cálculo de
	// comunicación.
	Perdida    float64 `json:"perdida,omitempty"`
	TCalculoMs float64 `json:"t_calculo_ms,omitempty"`

	// entrenamiento_finalizado. Con Continuar el nodo vuelve a la sala de
	// espera del maestro en modo servidor; sin él (maestro de una sola
	// corrida) el nodo termina.
	EpocaFinal   int     `json:"epoca_final,omitempty"`
	PerdidaFinal float64 `json:"perdida_final,omitempty"`
	Continuar    bool    `json:"continuar,omitempty"`

	// baja y entrenamiento_finalizado (motivo de una corrida cancelada);
	// en limpieza_terminada, el error si la tecnología no terminó.
	Motivo string `json:"motivo,omitempty"`

	// Limpieza. Corrida es aquí el número de limpieza. Tecnologias se
	// ejecutan una tras otra (nunca a la vez, para no pelear por la CPU);
	// Hilos = 0 usa todos los núcleos lógicos del nodo. Avance va de 0 a 1
	// y CPU es el uso del proceso de limpieza en % de toda la CPU del nodo.
	Tecnologias []string `json:"tecnologias,omitempty"`
	Tecnologia  string   `json:"tecnologia,omitempty"`
	Hilos       int      `json:"hilos,omitempty"`
	Avance      float64  `json:"avance,omitempty"`
	CPU         float64  `json:"cpu,omitempty"`
	TTotalS     float64  `json:"t_total_s,omitempty"`
	// Red neuronal por mini-lotes (Fase 2 de docs/plan_avance.md). En
	// particion_asignada: Modelo ("red"; vacío = regresión logística),
	// Capas y PasosPorEpoca. En parametros y gradiente_parcial: Paso dentro
	// de la época y los vectores en binario (float64 little-endian; en el
	// JSON van en base64), en vez de Pesos/Gradiente como texto.
	Modelo        string `json:"modelo,omitempty"`
	Capas         []int  `json:"capas,omitempty"`
	PasosPorEpoca int    `json:"pasos_por_epoca,omitempty"`
	Paso          int    `json:"paso,omitempty"`
	ParamsBin     []byte `json:"params_bin,omitempty"`
	GradBin       []byte `json:"grad_bin,omitempty"`

	// Comunicación entre nodos (Fase 4). En registro: PuertoPares, donde el
	// nodo escucha a los otros nodos. En particion_asignada: Comunicacion
	// ("arbol" o "anillo"; vacío = todo por el maestro), el Rango del nodo,
	// la dirección de pares de cada rango, las Epocas y la Tasa (cada nodo
	// aplica Adam por su cuenta). En gradiente_parcial: TRedMs (lo que tardó
	// la suma entre nodos) y BytesPares (lo que envió a otros nodos).
	PuertoPares  int      `json:"puerto_pares,omitempty"`
	Comunicacion string   `json:"comunicacion,omitempty"`
	Rango        int      `json:"rango,omitempty"`
	Pares        []string `json:"pares,omitempty"`
	Epocas       int      `json:"epocas,omitempty"`
	Tasa         float64  `json:"tasa,omitempty"`
	TRedMs       float64  `json:"t_red_ms,omitempty"`
	BytesPares   int64    `json:"bytes_pares,omitempty"`

	// Predicciones: filas de 20 bytes (estación uint32 = índice en la lista
	// ordenada de estaciones de muestra.bin, hora int64 desde 1970, prob
	// float32, NO2 observado float32), Total de filas del nodo y Ultimo en
	// el último trozo.
	PrediccionesBin []byte `json:"predicciones_bin,omitempty"`
	Total           int    `json:"total,omitempty"`
	Ultimo          bool   `json:"ultimo,omitempty"`

	// Fase de la limpieza a la que se refiere el avance ("limpieza" o
	// "variables"), y el tiempo de cada una al terminar.
	Fase        string  `json:"fase,omitempty"`
	TLimpiezaS  float64 `json:"t_limpieza_s,omitempty"`
	TVariablesS float64 `json:"t_variables_s,omitempty"`
}

// Conexion envuelve un extremo del protocolo. Enviar es seguro para varias
// goroutines (p. ej. el maestro difundiendo parámetros mientras otra
// goroutine responde a un registro); Recibir debe llamarlo una sola.
type Conexion struct {
	mu      sync.Mutex
	w       io.Writer
	scanner *bufio.Scanner
}

func NuevaConexion(rw io.ReadWriter) *Conexion {
	s := bufio.NewScanner(rw)
	// Un vector de pesos o gradientes es pequeño, pero se deja margen.
	s.Buffer(make([]byte, 64*1024), 16*1024*1024)
	return &Conexion{w: rw, scanner: s}
}

// Cerrar cierra el extremo subyacente si se puede (una conexión de red).
// Sirve para soltar una conexión que el otro lado ya no atiende; la
// goroutine que está en Recibir recibe un error y termina.
func (c *Conexion) Cerrar() error {
	if cerrador, ok := c.w.(io.Closer); ok {
		return cerrador.Close()
	}
	return nil
}

func (c *Conexion) Enviar(m Mensaje) error {
	datos, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err = c.w.Write(append(datos, '\n'))
	return err
}

// Recibir bloquea hasta el siguiente mensaje. Devuelve io.EOF si el otro
// extremo cerró la conexión.
func (c *Conexion) Recibir() (Mensaje, error) {
	if !c.scanner.Scan() {
		if err := c.scanner.Err(); err != nil {
			return Mensaje{}, err
		}
		return Mensaje{}, io.EOF
	}
	var m Mensaje
	if err := json.Unmarshal(c.scanner.Bytes(), &m); err != nil {
		return Mensaje{}, fmt.Errorf("mensaje inválido %q: %w", c.scanner.Text(), err)
	}
	if m.Tipo == "" {
		return Mensaje{}, fmt.Errorf("mensaje sin tipo: %q", c.scanner.Text())
	}
	return m, nil
}
