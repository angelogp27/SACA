package distribuido

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"time"

	"proyecto9/internal/logistica"
)

// Protocolo binario entre el trabajador Go y un motor externo (p. ej.
// nativo/cuda/gradiente_servidor.cu) por stdin/stdout del proceso. Todo
// es little-endian; los reales son float64.
//
//	motor → Go, al arrancar:   "SACA" | uint32 largo | nombre del dispositivo (UTF-8)
//	Go → motor, una vez:        uint32 d | uint64 n | float64 peso_clase_0, peso_clase_1 | X[n*d] por filas | y[n]
//	motor → Go:                 uint32 estado (0 = datos cargados; otro valor = error, detalle por stderr)
//	Go → motor, cada época:     uint32 orden (1) | pesos[d] | sesgo
//	motor → Go:                 grad[d] | grad_sesgo | perdida   (sumas sobre los n ejemplos)
//	Go → motor, otra partición: uint32 orden (2) | los mismos datos que la carga inicial
//	motor → Go:                 uint32 estado
//	Go → motor, al terminar:    uint32 orden (0)
//
// Por época solo viajan d+1 números de ida y d+2 de vuelta: los datos se
// copian a la GPU una vez por corrida. La orden 2 existe porque el maestro
// en modo servidor lanza varias corridas con distintos nodos, y en cada
// una a cada nodo le toca otra partición.
const (
	magiaMotor     = "SACA"
	ordenTerminar  = 0
	ordenGradiente = 1
	ordenRecargar  = 2
)

// MotorExterno es un Motor implementado por otro proceso.
type MotorExterno struct {
	nombre      string
	dispositivo string
	cmd         *exec.Cmd
	entrada     io.WriteCloser
	w           *bufio.Writer
	r           *bufio.Reader
	d           int
	cargado     bool
}

// IniciarExterno lanza el ejecutable y lee su presentación. No envía datos
// todavía: el trabajador aún no sabe qué partición le toca.
func IniciarExterno(nombre, ejecutable string, args ...string) (*MotorExterno, error) {
	cmd := exec.Command(ejecutable, args...)
	cmd.Stderr = os.Stderr
	entrada, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	salida, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("no se pudo lanzar %s: %w", ejecutable, err)
	}
	m := &MotorExterno{nombre: nombre, cmd: cmd, entrada: entrada, w: bufio.NewWriterSize(entrada, 1<<20), r: bufio.NewReaderSize(salida, 1<<16)}

	listo := make(chan error, 1)
	go func() { listo <- m.leerPresentacion() }()
	select {
	case err = <-listo:
	case <-time.After(30 * time.Second):
		err = errors.New("no respondió en 30 s")
	}
	if err != nil {
		m.matar()
		return nil, fmt.Errorf("motor %s (%s): %w", nombre, ejecutable, err)
	}
	return m, nil
}

func (m *MotorExterno) leerPresentacion() error {
	magia := make([]byte, 4)
	if _, err := io.ReadFull(m.r, magia); err != nil {
		return fmt.Errorf("sin presentación: %w", err)
	}
	if string(magia) != magiaMotor {
		return fmt.Errorf("presentación inválida %q", magia)
	}
	var largo uint32
	if err := binary.Read(m.r, binary.LittleEndian, &largo); err != nil {
		return err
	}
	if largo > 4096 {
		return fmt.Errorf("nombre de dispositivo demasiado largo (%d)", largo)
	}
	nombre := make([]byte, largo)
	if _, err := io.ReadFull(m.r, nombre); err != nil {
		return err
	}
	m.dispositivo = string(nombre)
	return nil
}

func (m *MotorExterno) Nombre() string      { return m.nombre }
func (m *MotorExterno) Dispositivo() string { return m.dispositivo }

func (m *MotorExterno) Cargar(d int, X [][]float64, y []float64, pc logistica.PesosClase) error {
	m.d = d
	if m.cargado {
		escribirU32(m.w, ordenRecargar)
	}
	escribirU32(m.w, uint32(d))
	escribirU64(m.w, uint64(len(X)))
	escribirF64(m.w, pc.Cero, pc.Uno)
	for _, fila := range X {
		if len(fila) != d {
			return fmt.Errorf("fila de %d variables, se esperaban %d", len(fila), d)
		}
		escribirF64(m.w, fila...)
	}
	escribirF64(m.w, y...)
	if err := m.w.Flush(); err != nil {
		return fmt.Errorf("enviando la partición al motor %s: %w", m.nombre, err)
	}
	var estado uint32
	if err := binary.Read(m.r, binary.LittleEndian, &estado); err != nil {
		return fmt.Errorf("el motor %s no confirmó la carga: %w", m.nombre, err)
	}
	if estado != 0 {
		return fmt.Errorf("el motor %s no pudo cargar los datos (estado %d, ver su salida de error)", m.nombre, estado)
	}
	m.cargado = true
	return nil
}

func (m *MotorExterno) Gradiente(pesos []float64, sesgo float64) ([]float64, float64, float64, error) {
	if len(pesos) != m.d {
		return nil, 0, 0, fmt.Errorf("se esperaban %d pesos, llegaron %d", m.d, len(pesos))
	}
	escribirU32(m.w, ordenGradiente)
	escribirF64(m.w, pesos...)
	escribirF64(m.w, sesgo)
	if err := m.w.Flush(); err != nil {
		return nil, 0, 0, fmt.Errorf("motor %s: %w", m.nombre, err)
	}
	res := make([]float64, m.d+2)
	if err := leerF64(m.r, res); err != nil {
		return nil, 0, 0, fmt.Errorf("motor %s: %w", m.nombre, err)
	}
	return res[:m.d], res[m.d], res[m.d+1], nil
}

func (m *MotorExterno) Cerrar() error {
	escribirU32(m.w, ordenTerminar)
	m.w.Flush()
	m.entrada.Close()
	fin := make(chan error, 1)
	go func() { fin <- m.cmd.Wait() }()
	select {
	case err := <-fin:
		return err
	case <-time.After(5 * time.Second):
		m.matar()
		return errors.New("el motor no terminó en 5 s; se cerró a la fuerza")
	}
}

func (m *MotorExterno) matar() {
	if m.cmd.Process != nil {
		m.cmd.Process.Kill()
		m.cmd.Wait()
	}
}

// ServirMotor implementa el lado del motor del protocolo en Go, calculando
// con GradienteParcial. Es la referencia del protocolo para quien escriba
// otro motor y lo usan los tests como un proceso externo falso.
func ServirMotor(entrada io.Reader, salida io.Writer, dispositivo string) error {
	r := bufio.NewReaderSize(entrada, 1<<20)
	w := bufio.NewWriter(salida)
	w.WriteString(magiaMotor)
	escribirU32(w, uint32(len(dispositivo)))
	w.WriteString(dispositivo)
	if err := w.Flush(); err != nil {
		return err
	}

	d, X, y, pc, err := leerDatos(r)
	if err != nil {
		return err
	}
	escribirU32(w, 0)
	if err := w.Flush(); err != nil {
		return err
	}

	for {
		var orden uint32
		if err := binary.Read(r, binary.LittleEndian, &orden); err != nil {
			return err
		}
		switch orden {
		case ordenTerminar:
			return nil
		case ordenRecargar:
			if d, X, y, pc, err = leerDatos(r); err != nil {
				return err
			}
			escribirU32(w, 0)
			if err := w.Flush(); err != nil {
				return err
			}
			continue
		}
		params := make([]float64, d+1)
		if err := leerF64(r, params); err != nil {
			return err
		}
		g, gs, p := logistica.GradienteParcial(params[:d], params[d], X, y, logistica.PesosClase{Cero: pc[0], Uno: pc[1]})
		escribirF64(w, g...)
		escribirF64(w, gs, p)
		if err := w.Flush(); err != nil {
			return err
		}
	}
}

// leerDatos lee una partición: d, n, pesos de clase, X por filas e y.
func leerDatos(r io.Reader) (d int, X [][]float64, y []float64, pc []float64, err error) {
	var d32 uint32
	var n uint64
	if err = binary.Read(r, binary.LittleEndian, &d32); err != nil {
		return
	}
	if err = binary.Read(r, binary.LittleEndian, &n); err != nil {
		return
	}
	d = int(d32)
	pc = make([]float64, 2)
	plano := make([]float64, int(n)*d)
	y = make([]float64, n)
	for _, dst := range [][]float64{pc, plano, y} {
		if err = leerF64(r, dst); err != nil {
			return
		}
	}
	X = make([][]float64, n)
	for i := range X {
		X[i] = plano[i*d : (i+1)*d]
	}
	return
}

func escribirU32(w *bufio.Writer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	w.Write(b[:])
}

func escribirU64(w *bufio.Writer, v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	w.Write(b[:])
}

func escribirF64(w *bufio.Writer, vs ...float64) {
	var b [8]byte
	for _, v := range vs {
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
		w.Write(b[:])
	}
}

func leerF64(r io.Reader, dst []float64) error {
	b := make([]byte, 8*len(dst))
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	for i := range dst {
		dst[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[8*i:]))
	}
	return nil
}
