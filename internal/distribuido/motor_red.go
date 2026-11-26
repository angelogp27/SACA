package distribuido

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"proyecto9/internal/entrenador"
	"proyecto9/internal/logistica"
	"proyecto9/internal/particion"
	"proyecto9/internal/red"
)

// Motores de CPU del entrenamiento y la predicción (Fase 6). Cada uno puede
// cooperar con CUDA en la misma laptop.
const (
	MotorCPUGo     = "go"
	MotorCPUOpenMP = "openmp"
	nombreCUDA     = "cuda"
)

func motorCPUValido(m string) bool { return m == MotorCPUGo || m == MotorCPUOpenMP }

// motorRed calcula el gradiente y las predicciones de la red con los datos
// de todo el nodo ya cargados (entrenamiento y, a continuación, prueba). Una
// regresión logística es la red de capas {d, 1}.
type motorRed interface {
	nombre() string
	// particion fija las filas que le tocan; gradiente trabaja sobre el
	// tramo [ini, fin) de ellas y devuelve sumas, no promedios.
	particion(idx []uint32) error
	gradiente(capas []int, p []float64, ini, fin int) ([]float64, float64, error)
	predecir(capas []int, p []float64, idx []uint32) ([]float64, error)
	pid() int // proceso externo; 0 = dentro del trabajador
	cerrar()
}

// ---------- Go ----------

type motorRedGo struct {
	hilos int
	X     [][]float64
	y     []float64
	pc    logistica.PesosClase
	pX    [][]float64
	py    []float64
}

func (m *motorRedGo) nombre() string { return MotorCPUGo }
func (m *motorRedGo) pid() int       { return 0 }
func (m *motorRedGo) cerrar()        {}

func (m *motorRedGo) particion(idx []uint32) error {
	m.pX, m.py = make([][]float64, len(idx)), make([]float64, len(idx))
	for k, i := range idx {
		m.pX[k], m.py[k] = m.X[i], m.y[i]
	}
	return nil
}

func (m *motorRedGo) gradiente(capas []int, p []float64, ini, fin int) ([]float64, float64, error) {
	g, l := red.Gradiente(capas, p, m.pX[ini:fin], m.py[ini:fin], m.pc, m.hilos)
	return g, l, nil
}

func (m *motorRedGo) predecir(capas []int, p []float64, idx []uint32) ([]float64, error) {
	X := make([][]float64, len(idx))
	for k, i := range idx {
		X[k] = m.X[i]
	}
	prob := make([]float64, len(idx))
	red.PredecirLote(capas, p, X, prob, m.hilos)
	return prob, nil
}

// ---------- proceso externo (OpenMP o CUDA) ----------

// Protocolo binario con nativo/openmp/red y nativo/cuda/red: ver
// nativo/comun/motor_red.h.
const (
	magiaRed        = "SACR"
	ordenDatos      = 1
	ordenParticion  = 2
	ordenGradRed    = 3
	ordenPredecir   = 4
	esperaMotorRed  = 30 * time.Second
	filasTrozoMotor = 1 << 16
)

type motorRedExterno struct {
	nom         string
	dispositivo string
	cmd         *exec.Cmd
	entrada     io.WriteCloser
	w           *bufio.Writer
	r           *bufio.Reader
	roto        bool // falló o se cerró: no se puede reutilizar
}

func iniciarMotorRed(nombre, exe string, args ...string) (*motorRedExterno, error) {
	cmd := exec.Command(exe, args...)
	cmd.Stderr = os.Stderr
	// Que los hilos de OpenMP duerman entre mini-lotes: girando en espera le
	// quitarían la CPU a la red y a CUDA.
	cmd.Env = append(os.Environ(), "OMP_WAIT_POLICY=passive")
	entrada, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	salida, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	m := &motorRedExterno{nom: nombre, cmd: cmd, entrada: entrada, w: bufio.NewWriterSize(entrada, 1<<20), r: bufio.NewReaderSize(salida, 1<<16)}
	listo := make(chan error, 1)
	go func() { listo <- m.presentacion() }()
	select {
	case err = <-listo:
	case <-time.After(esperaMotorRed):
		err = errors.New("no respondió a tiempo")
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("motor %s (%s): %w", nombre, exe, err)
	}
	return m, nil
}

func (m *motorRedExterno) presentacion() error {
	var cab [8]byte
	if _, err := io.ReadFull(m.r, cab[:]); err != nil {
		return fmt.Errorf("sin presentación: %w", err)
	}
	largo := binary.LittleEndian.Uint32(cab[4:])
	if string(cab[:4]) != magiaRed || largo > 4096 {
		return errors.New("presentación inválida")
	}
	b := make([]byte, largo)
	if _, err := io.ReadFull(m.r, b); err != nil {
		return err
	}
	m.dispositivo = string(b)
	return nil
}

func (m *motorRedExterno) nombre() string { return m.nom }
func (m *motorRedExterno) pid() int       { return m.cmd.Process.Pid }

func (m *motorRedExterno) cerrar() {
	if m.roto {
		m.cmd.Process.Kill()
	}
	m.roto = true
	escribirU32(m.w, ordenTerminar)
	m.w.Flush()
	m.entrada.Close()
	fin := make(chan struct{})
	go func() { m.cmd.Wait(); close(fin) }()
	select {
	case <-fin:
	case <-time.After(5 * time.Second):
		m.cmd.Process.Kill()
		<-fin
	}
}

func (m *motorRedExterno) fallo(err error) error {
	m.roto = true
	return fmt.Errorf("motor %s: %w", m.nom, err)
}

func (m *motorRedExterno) estado() error {
	if err := m.w.Flush(); err != nil {
		return m.fallo(err)
	}
	var b [4]byte
	if _, err := io.ReadFull(m.r, b[:]); err != nil {
		return m.fallo(err)
	}
	if e := binary.LittleEndian.Uint32(b[:]); e != 0 {
		return m.fallo(fmt.Errorf("devolvió el estado %d", e))
	}
	return nil
}

func (m *motorRedExterno) datos(X [][]float64, y []float64, pc logistica.PesosClase) error {
	escribirU32(m.w, ordenDatos)
	escribirU32(m.w, uint32(len(X[0])))
	escribirU64(m.w, uint64(len(X)))
	escribirF64(m.w, pc.Cero, pc.Uno)
	for _, fila := range X {
		escribirF64(m.w, fila...)
	}
	escribirF64(m.w, y...)
	return m.estado()
}

func (m *motorRedExterno) indices(idx []uint32) {
	escribirU32(m.w, uint32(len(idx)))
	var b [4]byte
	for _, i := range idx {
		binary.LittleEndian.PutUint32(b[:], i)
		m.w.Write(b[:])
	}
}

func (m *motorRedExterno) modelo(orden uint32, capas []int, p []float64) {
	escribirU32(m.w, orden)
	escribirU32(m.w, uint32(len(capas)))
	for _, c := range capas {
		escribirU32(m.w, uint32(c))
	}
	m.w.Write(red.Codificar(p))
}

func (m *motorRedExterno) particion(idx []uint32) error {
	escribirU32(m.w, ordenParticion)
	m.indices(idx)
	return m.estado()
}

func (m *motorRedExterno) respuesta(n int) ([]float64, error) {
	if err := m.w.Flush(); err != nil {
		return nil, m.fallo(err)
	}
	v := make([]float64, n)
	if err := leerF64(m.r, v); err != nil {
		return nil, m.fallo(err)
	}
	return v, nil
}

func (m *motorRedExterno) gradiente(capas []int, p []float64, ini, fin int) ([]float64, float64, error) {
	m.modelo(ordenGradRed, capas, p)
	escribirU32(m.w, uint32(ini))
	escribirU32(m.w, uint32(fin))
	v, err := m.respuesta(len(p) + 1)
	if err != nil {
		return nil, 0, err
	}
	return v[:len(p)], v[len(p)], nil
}

func (m *motorRedExterno) predecir(capas []int, p []float64, idx []uint32) ([]float64, error) {
	m.modelo(ordenPredecir, capas, p)
	m.indices(idx)
	return m.respuesta(len(idx))
}

// ---------- CPU + GPU ----------

// hibrido reparte cada mini-lote entre el motor de CPU y el de GPU según la
// velocidad (filas por ms) que mostró cada uno: el reparto dinámico, dentro
// de una laptop. Si la GPU falla, sigue solo la CPU.
type hibrido struct {
	cpu, gpu   motorRed // gpu nil = solo CPU
	vCPU, vGPU float64
	filasGPU   int // acumuladas desde la última lectura
	filas      int
}

// Con menos filas, partir el mini-lote no compensa.
const minimoParaGPU = 256

func (h *hibrido) nombre() string {
	if h.gpu != nil {
		return h.cpu.nombre() + "+" + h.gpu.nombre()
	}
	return h.cpu.nombre()
}

func (h *hibrido) particion(idx []uint32) error {
	if h.gpu != nil {
		if err := h.gpu.particion(idx); err != nil {
			h.sinGPU(err)
		}
	}
	return h.cpu.particion(idx)
}

func (h *hibrido) sinGPU(err error) {
	log.Printf("AVISO: %v; se sigue solo con la CPU", err)
	h.gpu.cerrar()
	h.gpu = nil
}

func (h *hibrido) gradiente(capas []int, p []float64, ini, fin int) ([]float64, float64, error) {
	n := fin - ini
	h.filas += n
	if h.gpu == nil || n < minimoParaGPU {
		return h.cpu.gradiente(capas, p, ini, fin)
	}
	parte := 0.5
	if h.vCPU+h.vGPU > 0 {
		// Al menos un 2 % para cada uno: así se sigue midiendo.
		parte = min(0.98, max(0.02, h.vGPU/(h.vCPU+h.vGPU)))
	}
	nGPU := min(n-1, max(1, int(parte*float64(n))))
	medio := fin - nGPU // [ini, medio) en la CPU, [medio, fin) en la GPU

	type salida struct {
		g   []float64
		l   float64
		err error
		t   time.Duration
	}
	ch := make(chan salida, 1)
	go func() {
		t0 := time.Now()
		g, l, err := h.gpu.gradiente(capas, p, medio, fin)
		ch <- salida{g, l, err, time.Since(t0)}
	}()
	t0 := time.Now()
	g, l, err := h.cpu.gradiente(capas, p, ini, medio)
	tCPU := time.Since(t0)
	s := <-ch
	if err != nil {
		return nil, 0, err
	}
	if s.err != nil {
		h.sinGPU(s.err)
		if s.g, s.l, err = h.cpu.gradiente(capas, p, medio, fin); err != nil {
			return nil, 0, err
		}
	} else {
		suavizar(&h.vCPU, float64(medio-ini)/max(ms(tCPU), 1e-3))
		suavizar(&h.vGPU, float64(nGPU)/max(ms(s.t), 1e-3))
		h.filasGPU += nGPU
	}
	for j, v := range s.g {
		g[j] += v
	}
	return g, l + s.l, nil
}

func suavizar(v *float64, nuevo float64) {
	if *v == 0 {
		*v = nuevo
	} else {
		*v = 0.7**v + 0.3*nuevo
	}
}

// parteGPU: fracción de las filas que calculó la GPU desde la última lectura.
func (h *hibrido) parteGPU() float64 {
	f := float64(h.filasGPU) / float64(max(h.filas, 1))
	h.filasGPU, h.filas = 0, 0
	return f
}

// predecir reparte idx en trozos: la CPU y la GPU toman el siguiente trozo
// libre cada vez que terminan uno (autoprogramación).
func (h *hibrido) predecir(capas []int, p []float64, idx []uint32) ([]float64, error) {
	prob := make([]float64, len(idx))
	var mu sync.Mutex
	siguiente := 0
	var pendientes [][2]int // trozos que la GPU no pudo hacer
	tomar := func() (int, int) {
		mu.Lock()
		defer mu.Unlock()
		if n := len(pendientes); n > 0 {
			t := pendientes[n-1]
			pendientes = pendientes[:n-1]
			return t[0], t[1]
		}
		ini := siguiente
		siguiente = min(ini+filasTrozoMotor, len(idx))
		return ini, siguiente
	}
	correr := func(m motorRed) (filas int, fallo [2]int, err error) {
		for {
			ini, fin := tomar()
			if ini == fin {
				return filas, fallo, nil
			}
			v, err := m.predecir(capas, p, idx[ini:fin])
			if err != nil {
				return filas, [2]int{ini, fin}, err
			}
			copy(prob[ini:fin], v)
			filas += fin - ini
		}
	}
	var errGPU error
	listo := make(chan struct{})
	gpu := h.gpu
	go func() {
		defer close(listo)
		if gpu != nil {
			n, fallo, err := correr(gpu)
			h.filasGPU += n
			if err != nil {
				errGPU = err
				mu.Lock()
				pendientes = append(pendientes, fallo)
				mu.Unlock()
			}
		}
	}()
	_, _, err := correr(h.cpu)
	<-listo
	if errGPU != nil {
		h.sinGPU(errGPU)
		if err == nil { // por si el trozo fallido quedó pendiente al final
			_, _, err = correr(h.cpu)
		}
	}
	h.filas += len(idx)
	return prob, err
}

// ---------- los motores de un nodo ----------

// MotoresRed lanza y conserva los motores externos del nodo. Se usa desde
// una sola goroutine.
type MotoresRed struct {
	ExeOpenMP, ExeCUDA string
	Hilos              int

	procesos map[string]*motorRedExterno
	datos    *entrenador.Datos // los que tienen cargados
	X        [][]float64
	y        []float64
}

// Disponibles: "go" siempre, y los demás si su ejecutable existe.
func (r *MotoresRed) Disponibles() []string {
	lista := []string{MotorCPUGo}
	if r == nil {
		return lista
	}
	for _, e := range [][2]string{{MotorCPUOpenMP, r.ExeOpenMP}, {nombreCUDA, r.ExeCUDA}} {
		if _, err := os.Stat(e[1]); e[1] != "" && err == nil {
			lista = append(lista, e[0])
		}
	}
	return lista
}

// Cerrar termina los procesos.
func (r *MotoresRed) Cerrar() {
	if r == nil {
		return
	}
	for n, p := range r.procesos {
		p.cerrar()
		delete(r.procesos, n)
	}
	r.datos, r.X, r.y = nil, nil, nil
}

func (r *MotoresRed) preparar(d *entrenador.Datos) {
	if r.datos == d {
		return
	}
	r.Cerrar()
	r.datos = d
	r.X = append(append(make([][]float64, 0, d.Entrenamiento.Len()+d.Prueba.Len()), d.Entrenamiento.X...), d.Prueba.X...)
	r.y = append(append(make([]float64, 0, len(r.X)), d.Entrenamiento.Y...), d.Prueba.Y...)
}

func (r *MotoresRed) motorGo() *motorRedGo {
	hilos := r.Hilos
	if hilos < 1 {
		hilos = runtime.NumCPU()
	}
	return &motorRedGo{hilos: hilos, X: r.X, y: r.y, pc: r.datos.PesosClase}
}

// externo devuelve el proceso, lanzándolo, dándole los datos y comprobando
// que calcula lo mismo que Go la primera vez.
func (r *MotoresRed) externo(nombre string) (*motorRedExterno, error) {
	if p, ok := r.procesos[nombre]; ok {
		if !p.roto {
			return p, nil
		}
		p.cerrar()
		delete(r.procesos, nombre)
	}
	exe, args := r.ExeOpenMP, []string{}
	if nombre == nombreCUDA {
		exe = r.ExeCUDA
	} else if r.Hilos > 0 {
		args = append(args, "--hilos", fmt.Sprint(r.Hilos))
	}
	if _, err := os.Stat(exe); exe == "" || err != nil {
		return nil, fmt.Errorf("este nodo no tiene el motor %s compilado (%s)", nombre, exe)
	}
	p, err := iniciarMotorRed(nombre, exe, args...)
	if err != nil {
		return nil, err
	}
	inicio := time.Now()
	if err = p.datos(r.X, r.y, r.datos.PesosClase); err == nil {
		log.Printf("motor %s listo (%s): datos cargados en %s", nombre, p.dispositivo, time.Since(inicio).Round(time.Millisecond))
		err = r.validar(p)
	}
	if err != nil {
		p.roto = true
		p.cerrar()
		return nil, err
	}
	if r.procesos == nil {
		r.procesos = map[string]*motorRedExterno{}
	}
	r.procesos[nombre] = p
	return p, nil
}

// validar: mismo gradiente y mismas probabilidades que Go (salvo el
// redondeo). Un motor rápido pero con otra respuesta no se usa.
func (r *MotoresRed) validar(p motorRed) error {
	n := min(2000, len(r.X))
	idx := make([]uint32, n)
	for k := range idx {
		idx[k] = uint32(k * (len(r.X) / n))
	}
	g := r.motorGo()
	capas := []int{len(r.X[0]), 8, 4, 1}
	params := red.Inicializar(capas, 7)
	for _, m := range []motorRed{g, p} {
		if err := m.particion(idx); err != nil {
			return err
		}
	}
	ga, la, _ := g.gradiente(capas, params, 0, n)
	gb, lb, err := p.gradiente(capas, params, 0, n)
	if err != nil {
		return err
	}
	pa, _ := g.predecir(capas, params, idx)
	pb, err := p.predecir(capas, params, idx)
	if err != nil {
		return err
	}
	a, b := append(append(ga, la), pa...), append(append(gb, lb), pb...)
	for j := range a {
		// Negado, para que un NaN del motor tampoco pase.
		if !(math.Abs(a[j]-b[j]) <= 1e-8*math.Max(1, math.Abs(a[j]))) {
			return fmt.Errorf("el motor %s no da lo mismo que Go (valor %d: %g y %g)", p.nombre(), j, b[j], a[j])
		}
	}
	return nil
}

// armar el motor pedido. Si falta el de CPU es un error (la comparación no
// sería la que se pidió); si CUDA no está o falla, se sigue solo con la CPU.
func (r *MotoresRed) armar(d *entrenador.Datos, cpu string, cuda bool) (*hibrido, error) {
	r.preparar(d)
	h := &hibrido{cpu: r.motorGo()}
	if cpu == MotorCPUOpenMP {
		p, err := r.externo(MotorCPUOpenMP)
		if err != nil {
			return nil, err
		}
		h.cpu = p
	}
	if cuda {
		if p, err := r.externo(nombreCUDA); err != nil {
			log.Printf("sin CUDA: %v", err)
		} else {
			h.gpu = p
		}
	}
	return h, nil
}

// filasDe: índices (en el entrenamiento) de las filas de una partición, en
// el mismo orden que Filtrar y FiltrarRestos.
func filasDe(estaciones []string, mod, resto int, restos []int) []uint32 {
	cubetas := make(map[int]bool, len(restos))
	for _, r := range restos {
		cubetas[r] = true
	}
	var idx []uint32
	ultima, dentro := "", false
	for i, e := range estaciones {
		if i == 0 || e != ultima {
			ultima = e
			if len(restos) > 0 {
				dentro = cubetas[particion.Asignar(e, mod)]
			} else {
				dentro = particion.Pertenece(e, mod, resto)
			}
		}
		if dentro {
			idx = append(idx, uint32(i))
		}
	}
	return idx
}
