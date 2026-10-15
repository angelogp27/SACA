package distribuido

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/entrenador"
	"proyecto9/internal/logistica"
	"proyecto9/internal/protocolo"
)

// Si el binario de test se lanza con esta variable, hace de motor externo
// (ServirMotor) en vez de correr los tests: así se prueba MotorExterno con
// un proceso de verdad sin necesitar una GPU.
const varMotorFalso = "SACA_MOTOR_FALSO"

func TestMain(m *testing.M) {
	if os.Getenv(varMotorFalso) != "" {
		if err := ServirMotor(os.Stdin, os.Stdout, "motor falso en Go"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// muestraSintetica arma una muestra con 40 estaciones y 12 meses cuya
// etiqueta depende de las dos primeras variables.
func muestraSintetica(n, d int) *dataset.Muestra {
	azar := rand.New(rand.NewPCG(7, 11))
	m := &dataset.Muestra{}
	for i := 0; i < n; i++ {
		x := make([]float64, d)
		for j := range x {
			x[j] = azar.NormFloat64()*3 + float64(j)
		}
		y := 0.0
		if x[0]+0.5*x[1]+azar.NormFloat64() > 2 {
			y = 1
		}
		m.X = append(m.X, x)
		m.Y = append(m.Y, y)
		m.EstacionID = append(m.EstacionID, fmt.Sprintf("06-037-%04d", i%40))
		m.Mes = append(m.Mes, 1+i%12)
	}
	return m
}

func datosSinteticos(t *testing.T) *entrenador.Datos {
	t.Helper()
	datos, err := entrenador.PrepararDesde(muestraSintetica(3000, 5), 10)
	if err != nil {
		t.Fatal(err)
	}
	return datos
}

func TestFiltrarRepartePorEstacionSinPerderEjemplos(t *testing.T) {
	m := muestraSintetica(1000, 3)
	total := 0
	for resto := 0; resto < 4; resto++ {
		X, y := Filtrar(m, 4, resto)
		if len(X) != len(y) {
			t.Fatalf("partición %d: %d filas y %d etiquetas", resto, len(X), len(y))
		}
		total += len(X)
	}
	if total != m.Len() {
		t.Fatalf("las particiones suman %d ejemplos de %d", total, m.Len())
	}
}

func TestActualizarIgualQueEntrenarSecuencial(t *testing.T) {
	datos := datosSinteticos(t)
	X, y, pc := datos.Entrenamiento.X, datos.Entrenamiento.Y, datos.PesosClase
	const tasa, epocas = 0.1, 30

	secuencial := logistica.NuevoModelo(len(X[0]))
	perdidasSec := secuencial.Entrenar(X, y, tasa, epocas, pc)

	const nodos = 3
	type part struct {
		X [][]float64
		y []float64
	}
	partes := make([]part, nodos)
	for r := range partes {
		partes[r].X, partes[r].y = Filtrar(datos.Entrenamiento, nodos, r)
	}
	distribuido := logistica.NuevoModelo(len(X[0]))
	for e := 0; e < epocas; e++ {
		parciales := make([]Parcial, nodos)
		for r, p := range partes {
			g, gs, pe := logistica.GradienteParcial(distribuido.Pesos, distribuido.Sesgo, p.X, p.y, pc)
			parciales[r] = Parcial{Grad: g, GradSesgo: gs, Perdida: pe, N: len(p.X)}
		}
		perdida, err := Actualizar(distribuido, tasa, parciales, len(X))
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(perdida-perdidasSec[e]) > 1e-12 {
			t.Fatalf("época %d: pérdida distribuida %.15f, secuencial %.15f", e, perdida, perdidasSec[e])
		}
	}
	for j := range secuencial.Pesos {
		if math.Abs(secuencial.Pesos[j]-distribuido.Pesos[j]) > 1e-12 {
			t.Fatalf("peso %d: %g frente a %g", j, distribuido.Pesos[j], secuencial.Pesos[j])
		}
	}
}

func TestActualizarDetectaParticionesIncompletas(t *testing.T) {
	m := logistica.NuevoModelo(2)
	_, err := Actualizar(m, 0.1, []Parcial{{Grad: []float64{1, 1}, N: 10}}, 11)
	if err == nil {
		t.Fatal("se esperaba error: las particiones suman 10 ejemplos y el entrenamiento tiene 11")
	}
}

func iniciarMotorFalso(t *testing.T) *MotorExterno {
	t.Helper()
	t.Setenv(varMotorFalso, "1")
	m, err := IniciarExterno("falso", os.Args[0], "-test.run=^$")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMotorExternoIgualQueMotorGo(t *testing.T) {
	datos := datosSinteticos(t)
	X, y, pc := datos.Entrenamiento.X, datos.Entrenamiento.Y, datos.PesosClase
	d := len(X[0])

	externo := iniciarMotorFalso(t)
	if externo.Dispositivo() != "motor falso en Go" {
		t.Fatalf("dispositivo = %q", externo.Dispositivo())
	}
	if err := externo.Cargar(d, X, y, pc); err != nil {
		t.Fatal(err)
	}
	local := NuevoMotorGo(4)
	local.Cargar(d, X, y, pc)
	if err := Comparar(externo, local, []float64{0.3, -0.2, 0.1, 0, 0.5}, -1, 1e-12); err != nil {
		t.Fatal(err)
	}
	if err := externo.Cerrar(); err != nil {
		t.Fatalf("cerrar: %v", err)
	}
}

func TestMotorExternoConParticionVacia(t *testing.T) {
	externo := iniciarMotorFalso(t)
	defer externo.Cerrar()
	if err := externo.Cargar(3, nil, nil, logistica.PesosClase{Cero: 1, Uno: 1}); err != nil {
		t.Fatal(err)
	}
	g, gs, p, err := externo.Gradiente([]float64{1, 2, 3}, 0)
	if err != nil || len(g) != 3 || gs != 0 || p != 0 {
		t.Fatalf("gradiente = %v, %v, %v (%v)", g, gs, p, err)
	}
}

func TestMotorExternoInexistente(t *testing.T) {
	if _, err := IniciarExterno("cuda", "no-existe-este-motor.exe"); err == nil {
		t.Fatal("se esperaba error al lanzar un ejecutable que no existe")
	}
}

// motorRoto devuelve siempre un gradiente distinto al correcto, para
// probar que el trabajador lo detecta y vuelve al motor de Go.
type motorRoto struct{ MotorGo }

func (m *motorRoto) Nombre() string { return "roto" }
func (m *motorRoto) Gradiente(p []float64, s float64) ([]float64, float64, float64, error) {
	g, gs, pe, err := m.MotorGo.Gradiente(p, s)
	g[0] += 1
	return g, gs, pe, err
}

// levantarMaestro escucha en un puerto libre de localhost y atiende las
// conexiones con el maestro, como entrenamiento/cmd/maestro.
func levantarMaestro(t *testing.T, cfg ConfigMaestro) (*Maestro, string) {
	t.Helper()
	oyente, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { oyente.Close() })
	m := NuevoMaestro(cfg)
	go func() {
		for {
			conn, err := oyente.Accept()
			if err != nil {
				return
			}
			go m.Atender(conn)
		}
	}()
	return m, oyente.Addr().String()
}

// lanzarTrabajador conecta un trabajador en una goroutine. Su error (si
// no es nil ni ErrSesionTerminada) va a errs; al terminar cierra su motor.
func lanzarTrabajador(t *testing.T, wg *sync.WaitGroup, errs chan<- error, dir string, cfg *ConfigTrabajador) {
	t.Helper()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cfg.Motor.Cerrar()
		conn, err := net.Dial("tcp", dir)
		if err != nil {
			errs <- err
			return
		}
		defer conn.Close()
		if err := Trabajar(protocolo.NuevaConexion(conn), cfg); err != nil && !errors.Is(err, ErrSesionTerminada) {
			errs <- fmt.Errorf("%s: %w", cfg.ID, err)
		}
	}()
}

func comprobarErrores(t *testing.T, wg *sync.WaitGroup, errs chan error) {
	t.Helper()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestEntrenamientoDistribuidoIgualQueSecuencial(t *testing.T) {
	datos := datosSinteticos(t)
	const tasa, epocas = 0.1, 40

	secuencial := logistica.NuevoModelo(len(datos.Entrenamiento.X[0]))
	perdidasSec := secuencial.Entrenar(datos.Entrenamiento.X, datos.Entrenamiento.Y, tasa, epocas, datos.PesosClase)

	maestro, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 3, Datos: datos, Huella: "h", EsperaGradiente: 10 * time.Second})

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	// Un nodo con goroutines, uno con el motor externo falso y uno con un
	// motor que da mal: este último debe volver solo al motor de Go.
	lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: "cpu1", Datos: datos, Huella: "h", Motor: NuevoMotorGo(2)})
	lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: "gpu1", Datos: datos, Huella: "h", Motor: iniciarMotorFalso(t), Respaldo: NuevoMotorGo(1)})
	lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: "roto", Datos: datos, Huella: "h", Motor: &motorRoto{MotorGo{Hilos: 1}}, Respaldo: NuevoMotorGo(1)})

	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	res, err := maestro.Entrenar(ctx, epocas, tasa)
	if err != nil {
		t.Fatal(err)
	}
	comprobarErrores(t, &wg, errs)

	if len(res.Perdidas) != epocas {
		t.Fatalf("%d pérdidas, se esperaban %d", len(res.Perdidas), epocas)
	}
	if dif := math.Abs(res.Perdidas[epocas-1] - perdidasSec[epocas-1]); dif > 1e-12 {
		t.Fatalf("pérdida final distribuida %.15f, secuencial %.15f (dif %g)", res.Perdidas[epocas-1], perdidasSec[epocas-1], dif)
	}
	n := 0
	motores := map[string]string{}
	for _, nodo := range res.Nodos {
		n += nodo.N
		motores[nodo.ID] = nodo.Motor
	}
	if n != datos.Entrenamiento.Len() {
		t.Fatalf("los nodos suman %d ejemplos de %d", n, datos.Entrenamiento.Len())
	}
	if motores["gpu1"] != "falso" || motores["roto"] != "go" {
		t.Fatalf("motores = %v; gpu1 debía usar el externo y roto volver a go", motores)
	}
}

// TestSalaVariasCorridas es el modo servidor: tres nodos se conectan una
// vez y el maestro lanza corridas con distintos subconjuntos. Cada corrida
// debe dar la pérdida del secuencial, y los nodos vuelven a la sala.
func TestSalaVariasCorridas(t *testing.T) {
	datos := datosSinteticos(t)
	const tasa, epocas = 0.1, 25
	secuencial := logistica.NuevoModelo(len(datos.Entrenamiento.X[0]))
	perdidasSec := secuencial.Entrenar(datos.Entrenamiento.X, datos.Entrenamiento.Y, tasa, epocas, datos.PesosClase)

	var mu sync.Mutex
	var estados []string
	notificar := func(i Instantanea) {
		mu.Lock()
		defer mu.Unlock()
		if i.Corrida != nil {
			estados = append(estados, fmt.Sprintf("%d:%s:%d", i.Corrida.ID, i.Corrida.Estado, i.Corrida.Epoca))
		}
	}
	maestro, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 5, Datos: datos, Huella: "h", EsperaGradiente: 10 * time.Second, Notificar: notificar})

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: "a", Datos: datos, Huella: "h", Motor: NuevoMotorGo(1)})
	lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: "b", Datos: datos, Huella: "h", Motor: iniciarMotorFalso(t), Respaldo: NuevoMotorGo(1)})
	lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: "c", Datos: datos, Huella: "h", Motor: NuevoMotorGo(2)})

	ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelar()
	if err := maestro.EsperarNodos(ctx, 3); err != nil {
		t.Fatal(err)
	}
	maestro.MedirT1(2)

	for _, eleccion := range [][]string{{"b"}, {"a", "b"}, {"c", "a", "b"}, {"b", "c"}} {
		res, err := maestro.Correr(ctx, OpcionesCorrida{Nodos: eleccion, Epocas: epocas, Tasa: tasa, Continuar: true})
		if err != nil {
			t.Fatalf("corrida con %v: %v", eleccion, err)
		}
		if dif := math.Abs(res.Perdidas[epocas-1] - perdidasSec[epocas-1]); dif > 1e-12 {
			t.Fatalf("corrida con %v: pérdida %.15f, secuencial %.15f", eleccion, res.Perdidas[epocas-1], perdidasSec[epocas-1])
		}
		if len(res.Nodos) != len(eleccion) {
			t.Fatalf("corrida con %v usó %d nodos", eleccion, len(res.Nodos))
		}
	}

	inst := maestro.Instantanea()
	if len(inst.Sala) != 3 || len(inst.Historial) != 4 || inst.Ocupado {
		t.Fatalf("sala=%d historial=%d ocupado=%v; se esperaban 3 nodos libres y 4 corridas", len(inst.Sala), len(inst.Historial), inst.Ocupado)
	}
	for _, n := range inst.Sala {
		if n.Estado != "libre" {
			t.Fatalf("nodo %s quedó %q", n.ID, n.Estado)
		}
	}
	h := inst.Historial[1]
	if h.Estado != EstadoTerminado || len(h.Nodos) != 2 || h.Speedup <= 0 || h.TEpocaMs <= 0 {
		t.Fatalf("historial[1] = %+v", h)
	}
	if c := inst.Corrida; c == nil || c.Epoca != epocas || len(c.Perdidas) != epocas || len(c.Nodos[0].CalculoMs) != epocas {
		t.Fatalf("la corrida de la instantánea no tiene las series completas: %+v", c)
	}

	// No se puede empezar con un nodo que no existe; tampoco dos a la vez.
	if _, err := maestro.Iniciar(OpcionesCorrida{Nodos: []string{"zz"}, Epocas: 1}); !errors.Is(err, ErrNodoNoDisponible) {
		t.Fatalf("err = %v; se esperaba ErrNodoNoDisponible", err)
	}
	reserva, err := maestro.Iniciar(OpcionesCorrida{Nodos: []string{"a"}, Epocas: 3, Tasa: tasa, Continuar: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maestro.Iniciar(OpcionesCorrida{Nodos: []string{"b"}, Epocas: 1}); !errors.Is(err, ErrOcupado) {
		t.Fatalf("err = %v; se esperaba ErrOcupado", err)
	}
	if _, err := reserva.Ejecutar(ctx); err != nil {
		t.Fatal(err)
	}

	maestro.Despedir(MotivoSesionTerminada)
	comprobarErrores(t, &wg, errs)

	mu.Lock()
	defer mu.Unlock()
	visto := strings.Join(estados, " ")
	for _, esperado := range []string{"1:entrenando:0", "1:entrenando:25", "1:terminado:25", "4:terminado:25"} {
		if !strings.Contains(visto, esperado) {
			t.Fatalf("las notificaciones no incluyen %q", esperado)
		}
	}
}

func TestCorridaCanceladaDevuelveLosNodosALaSala(t *testing.T) {
	datos := datosSinteticos(t)
	maestro, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 2, Datos: datos, Huella: "h", EsperaGradiente: 10 * time.Second})
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: "a", Datos: datos, Huella: "h", Motor: NuevoMotorGo(1)})
	lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: "b", Datos: datos, Huella: "h", Motor: NuevoMotorGo(1)})
	ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelar()
	if err := maestro.EsperarNodos(ctx, 2); err != nil {
		t.Fatal(err)
	}

	ctxCorrida, cancelarCorrida := context.WithCancel(ctx)
	time.AfterFunc(200*time.Millisecond, cancelarCorrida)
	_, err := maestro.Correr(ctxCorrida, OpcionesCorrida{Epocas: 1000000, Tasa: 0.1, Continuar: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; se esperaba la cancelación", err)
	}
	// Los mismos nodos sirven para la siguiente corrida.
	if _, err := maestro.Correr(ctx, OpcionesCorrida{Epocas: 5, Tasa: 0.1, Continuar: true}); err != nil {
		t.Fatalf("la corrida siguiente a la cancelada falló: %v", err)
	}
	if h := maestro.Instantanea().Historial; len(h) != 2 || h[0].Estado != EstadoCancelado || h[1].Estado != EstadoTerminado {
		t.Fatalf("historial = %+v", h)
	}
	if err := maestro.Quitar("a"); err != nil {
		t.Fatal(err)
	}
	maestro.Despedir(MotivoSesionTerminada)
	comprobarErrores(t, &wg, errs)
}

func TestMaestroRechazaMuestraDistinta(t *testing.T) {
	datos := datosSinteticos(t)
	_, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 1, Datos: datos, Huella: "buena"})

	conn, err := net.Dial("tcp", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	err = Trabajar(protocolo.NuevaConexion(conn), &ConfigTrabajador{ID: "w1", Datos: datos, Huella: "otra", Motor: NuevoMotorGo(1)})
	if !errors.Is(err, ErrRechazado) || !strings.Contains(err.Error(), "muestra_distinta") {
		t.Fatalf("err = %v; se esperaba un rechazo por muestra distinta", err)
	}
}

func TestMaestroAbortaSiUnNodoSeCae(t *testing.T) {
	datos := datosSinteticos(t)
	maestro, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 2, Datos: datos, Huella: "h", EsperaGradiente: 10 * time.Second})

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: "sano", Datos: datos, Huella: "h", Motor: NuevoMotorGo(1)})

	// Un nodo que se registra, responde dos épocas y corta la conexión.
	conn, err := net.Dial("tcp", dir)
	if err != nil {
		t.Fatal(err)
	}
	c := protocolo.NuevaConexion(conn)
	c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoRegistro, WorkerID: "fragil", Huella: "h", Motor: "go"})
	go func() {
		defer conn.Close()
		asignada, err := c.Recibir()
		if err != nil || asignada.FiltroMongo == nil {
			return
		}
		X, y := Filtrar(datos.Entrenamiento, asignada.FiltroMongo.Mod, asignada.FiltroMongo.Resto)
		for e := 0; e < 2; e++ {
			p, err := c.Recibir()
			if err != nil {
				return
			}
			g, gs, pe := logistica.GradienteParcial(p.Pesos, p.Sesgo, X, y, datos.PesosClase)
			c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoGradienteParcial, WorkerID: "fragil", Corrida: p.Corrida, Epoca: p.Epoca, Gradiente: g, GradienteSesgo: gs, Perdida: pe, NMuestras: len(X)})
		}
	}()

	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	_, err = maestro.Entrenar(ctx, 1000, 0.1)
	if err == nil || !strings.Contains(err.Error(), "fragil se cayó") {
		t.Fatalf("err = %v; se esperaba que el maestro abortara por la caída de fragil", err)
	}
	maestro.Despedir(MotivoMaestroCerrando)
	wg.Wait()
}
