// Package protocolo implementa los mensajes del protocolo TCP
// maestro-trabajador de docs/contrato_protocolo_tcp.md: un objeto JSON por
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
)

const (
	TipoRegistro                = "registro"
	TipoParticionAsignada       = "particion_asignada"
	TipoParametros              = "parametros"
	TipoGradienteParcial        = "gradiente_parcial"
	TipoEntrenamientoFinalizado = "entrenamiento_finalizado"
	TipoBaja                    = "baja"
)

// FiltroMongo indica qué porción de los datos le toca a un trabajador:
// las estaciones cuyo hash (internal/particion) módulo Mod es Resto.
type FiltroMongo struct {
	Mod   int `json:"estacion_id_hash_mod"`
	Resto int `json:"estacion_id_hash_resto"`
}

// Mensaje es la unión de todos los tipos del contrato; cada tipo usa solo
// sus campos y el resto se omite al serializar.
type Mensaje struct {
	Tipo     string `json:"tipo"`
	WorkerID string `json:"worker_id,omitempty"`

	// particion_asignada
	FiltroMongo *FiltroMongo `json:"filtro_mongo,omitempty"`

	// parametros y gradiente_parcial
	Epoca          int       `json:"epoca,omitempty"`
	Pesos          []float64 `json:"pesos,omitempty"`
	Sesgo          float64   `json:"sesgo,omitempty"`
	Gradiente      []float64 `json:"gradiente,omitempty"`
	GradienteSesgo float64   `json:"gradiente_sesgo,omitempty"`
	NMuestras      int       `json:"n_muestras,omitempty"`

	// entrenamiento_finalizado
	EpocaFinal   int     `json:"epoca_final,omitempty"`
	PerdidaFinal float64 `json:"perdida_final,omitempty"`

	// baja
	Motivo string `json:"motivo,omitempty"`
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
