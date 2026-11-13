package distribuido

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"proyecto9/internal/cluster"
	"proyecto9/internal/dataset"
	"proyecto9/internal/entrenador"
	"proyecto9/internal/hardware"
	"proyecto9/internal/logistica"
	"proyecto9/internal/metricas"
	"proyecto9/internal/pares"
	"proyecto9/internal/particion"
	"proyecto9/internal/protocolo"
	"proyecto9/internal/red"
)

// Motivos de "baja" que el trabajador entiende.
const (
	MotivoSesionTerminada = "sesion_terminada"
	MotivoMaestroCerrando = "maestro_cerrando"
)

var (
	ErrOcupado          = errors.New("ya hay una corrida en curso")
	ErrLimpiando        = errors.New("hay una limpieza en curso")
	ErrMuestraDistinta  = errors.New("muestra_distinta")
	ErrSinMuestra       = errors.New("el maestro no tiene muestra (data/muestra.bin): realiza la limpieza en el nodo del maestro")
	ErrNodoNoDisponible = errors.New("nodo no disponible")
	ErrSinNodos         = errors.New("no se eligió ningún nodo")
)

// ConfigMaestro son los parámetros del maestro.
type ConfigMaestro struct {
	// Capacidad es cuántos nodos caben en la sala de espera. En el modo de
	// una corrida es exactamente la cantidad de nodos que se esperan.
	Capacidad int
	// Datos y Huella son la muestra con la que arranca el maestro (puede
	// no tener: Datos nil y Huella ""). Si RutaMuestra no es vacía, antes
	// de cada corrida se vuelve a leer ese archivo: una limpieza puede
	// haberlo regenerado.
	Datos       *entrenador.Datos
	Huella      string
	RutaMuestra string
	MesCorte    int
	// AceptarOtraMuestra: en modo servidor un nodo entra a la sala aunque
	// su muestra sea otra (o no tenga): puede limpiar y generarla desde la
	// página. La huella se exige al empezar cada entrenamiento.
	AceptarOtraMuestra bool
	// NodoMaestro es el id del nodo trabajador que corre en la misma
	// laptop que el maestro (la página lo dibuja como el maestro).
	NodoMaestro string
	// LimpiezaTerminada, si no es nil, recibe la limpieza al terminar o
	// pararse; guardar dice si sus tiempos deben guardarse.
	LimpiezaTerminada func(e EstadoLimpieza, guardar bool)
	// PrediccionTerminada, si no es nil, recibe las predicciones de todos
	// los nodos (Fase 5) para regenerar los datos del mapa.
	PrediccionTerminada func(e EstadoPrediccion, filas []FilaPrediccion, modelo ModeloEntrenado)
	// EsperaGradiente es cuánto se espera a un nodo en cada época antes
	// de abortar: un nodo colgado no debe colgar la demo.
	EsperaGradiente time.Duration
	// Notificar, si no es nil, recibe el estado cada vez que cambia (un
	// nodo entra o sale, termina una época, termina una corrida). Debe
	// volver rápido: la página lo recibe a través de él.
	Notificar func(Instantanea)
}

// OpcionesCorrida describen un entrenamiento.
type OpcionesCorrida struct {
	Nodos     []string // en este orden reciben las particiones; vacío = todos los de la sala
	Epocas    int
	Tasa      float64
	Verificar bool // entrenar también en secuencial para comparar
	// Continuar: los nodos vuelven a la sala al terminar (modo servidor).
	// Sin él, el nodo termina (modo de una corrida).
	Continuar bool
	// CerrarSala: al terminar el entrenamiento no se aceptan más nodos (modo
	// de una corrida: uno que llegue tarde no se registra en vano).
	CerrarSala bool
	// Comunicacion de la red neuronal (Fase 4): "central" (todo por el
	// maestro), "arbol" o "anillo" (los nodos suman entre ellos).
	Comunicacion string
	// Modelo: "" o "logistica" (regresión logística, por lotes completos) o
	// "red" (red neuronal por mini-lotes, Fase 2). Capas son las capas
	// ocultas de la red; PasosPorEpoca, los mini-lotes por época.
	Modelo        string
	Capas         []int
	PasosPorEpoca int
	// Pesos reparte la muestra en proporción al peso de cada nodo (por id)
	// en vez de en partes iguales (Experimento 9). Debe tener un peso
	// positivo para cada nodo de la corrida; nil = reparto igual.
	Pesos map[string]float64
}

// InfoNodo describe a un nodo de una corrida para el reporte final.
type InfoNodo struct {
	ID, Motor, Dispositivo, Remoto string
	Hardware                       *hardware.Info
	Particion, N                   int
	TCalculo                       time.Duration // suma de todas las épocas
}

// Verificacion es el mismo entrenamiento hecho en secuencial en el maestro.
type Verificacion struct {
	Perdidas   []float64
	Duracion   time.Duration
	Diferencia float64
	Evaluacion metricas.Clasificacion
}

// Resultado de un entrenamiento distribuido. TCalculo es la suma, época
// por época, del cálculo del nodo más lento (el que marca el ritmo);
// TComunicacion es el resto del tiempo de cada época: red, serialización
// y espera.
type Resultado struct {
	ID            int
	Modelo        *logistica.Modelo
	Perdidas      []float64
	Duracion      time.Duration
	TCalculo      time.Duration
	TComunicacion time.Duration
	Nodos         []InfoNodo
	Evaluacion    metricas.Clasificacion
	Verificacion  *Verificacion
	Normalizador  dataset.Normalizador // de la muestra de la corrida, para guardar el modelo
	// Red: el modelo cuando la corrida es de la red neuronal (Modelo es nil).
	Red *ResultadoRed
	// MBRed: megabytes que pasaron por la red en toda la corrida (red
	// neuronal: parámetros, gradientes y lo que se mandan los nodos entre
	// ellos en el árbol o el anillo).
	MBRed float64
	// T1EpocaMs: una época en secuencial medida en esta corrida (la red); 0
	// = usar la de la regresión logística.
	T1EpocaMs float64
}

type tipoEvento int

const (
	gradiente tipoEvento = iota
	caida
)

type evento struct {
	tipo   tipoEvento
	id     string
	msg    protocolo.Mensaje
	motivo string
}

type nodoSala struct {
	vista    NodoVista
	conexion *protocolo.Conexion
	orden    int
	// puertoPares: donde escucha a los otros nodos (0 = no puede);
	// ipLocal: la IP de ESTE maestro por la que llegó el nodo (sirve para
	// dar a los demás la dirección del nodo que corre junto al maestro).
	puertoPares int
	ipLocal     string
}

// corridaActiva recibe los mensajes de los nodos que participan en ella.
type corridaActiva struct {
	id      int
	indice  map[string]int
	eventos chan evento
	fin     chan struct{}
	huella  string // de la muestra de la corrida
}

func (c *corridaActiva) avisar(ev evento) {
	select {
	case c.eventos <- ev:
	case <-c.fin:
	}
}

// Maestro coordina el entrenamiento síncrono (pasos 3 a 6 del contrato)
// sobre una sala de nodos conectados. Cada conexión la atiende su propia
// goroutine (Atender), que le pasa los gradientes a la corrida en curso
// por un channel; así el modelo solo lo toca la goroutine de la corrida y
// no necesita locks. La sala (quién está conectado) es estado compartido
// y va protegida por un mutex.
type Maestro struct {
	cfg      ConfigMaestro
	registro *cluster.Registro // cupos de la sala, con su propio mutex

	mu        sync.Mutex
	nodos     map[string]*nodoSala
	llegadas  int
	cerrada   bool
	activa    *corridaActiva
	ocupados  map[string]bool
	estado    *EstadoCorrida // la corrida en curso o la última
	historial []ResumenCorrida
	siguiente int
	t1EpocaMs float64
	cambios   chan struct{}

	datos   *entrenador.Datos // muestra actual del maestro (nil = no tiene)
	huella  string
	cargaMu sync.Mutex // una sola recarga de la muestra a la vez

	limpieza          *EstadoLimpieza // la limpieza en curso o la última
	siguienteLimpieza int
	consultas         map[string]bool // nodos a los que se pidió la huella y no respondieron

	// Rendimiento medido de cada nodo (por id), para proponer el reparto
	// proporcional (Fase 3): ejemplos por ms de cálculo en el último
	// entrenamiento, y 1/segundos de su última limpieza con Go.
	rendEntreno  map[string]float64
	rendLimpieza map[string]float64

	// Predicción distribuida (Fase 5).
	ultimoModelo        *ModeloEntrenado
	prediccion          *EstadoPrediccion
	siguientePrediccion int
	predFilas           map[string][]FilaPrediccion
	predLista           []string
}

func NuevoMaestro(cfg ConfigMaestro) *Maestro {
	if cfg.EsperaGradiente <= 0 {
		cfg.EsperaGradiente = 2 * time.Minute
	}
	if cfg.Capacidad <= 0 {
		cfg.Capacidad = 16
	}
	return &Maestro{
		cfg:          cfg,
		registro:     cluster.NuevoRegistro(cfg.Capacidad),
		nodos:        map[string]*nodoSala{},
		ocupados:     map[string]bool{},
		cambios:      make(chan struct{}, 1),
		datos:        cfg.Datos,
		huella:       cfg.Huella,
		consultas:    map[string]bool{},
		rendEntreno:  map[string]float64{},
		rendLimpieza: map[string]float64{},
	}
}

// recargar vuelve a leer la muestra del maestro si el archivo cambió (una
// limpieza lo regeneró, o se copió otro). Sin archivo, el maestro queda
// sin muestra y no puede empezar un entrenamiento.
func (m *Maestro) recargar() error {
	if m.cfg.RutaMuestra == "" {
		return nil
	}
	m.cargaMu.Lock()
	defer m.cargaMu.Unlock()
	h, err := Huella(m.cfg.RutaMuestra)
	if err != nil {
		m.mu.Lock()
		cambio := m.datos != nil
		m.datos, m.huella = nil, ""
		m.mu.Unlock()
		if cambio {
			m.publicar()
		}
		return nil
	}
	m.mu.Lock()
	igual := h == m.huella && m.datos != nil
	m.mu.Unlock()
	if igual {
		return nil
	}
	datos, h, err := PrepararLocal(m.cfg.RutaMuestra, m.cfg.MesCorte)
	if err != nil {
		return fmt.Errorf("no se pudo cargar la muestra del maestro %s: %w", m.cfg.RutaMuestra, err)
	}
	m.mu.Lock()
	m.datos, m.huella = datos, h
	sinT1 := m.t1EpocaMs == 0
	m.mu.Unlock()
	log.Printf("muestra del maestro: %s, huella %s, %d ejemplos de entrenamiento", m.cfg.RutaMuestra, Corta(h), datos.Entrenamiento.Len())
	if sinT1 {
		m.MedirT1(5)
	}
	m.publicar()
	return nil
}

// Recargar es recargar para quien usa el maestro (p. ej. al arrancar).
func (m *Maestro) Recargar() error { return m.recargar() }

// senalar avisa (sin bloquear) que la sala cambió.
func (m *Maestro) senalar() {
	select {
	case m.cambios <- struct{}{}:
	default:
	}
}

// Atender es la goroutine dedicada a una conexión: valida el registro,
// deja al nodo en la sala y reenvía sus gradientes a la corrida en curso
// hasta que el nodo se va.
func (m *Maestro) Atender(conn net.Conn) {
	defer conn.Close()
	// Sin esto, una conexión que se corta en silencio (la PC del nodo se
	// durmió, cambió de red) queda "viva" en el maestro y bloquea el
	// registro del mismo nodo cuando vuelve. Con sondas cada pocos segundos,
	// el sistema operativo la declara muerta en unos 15 s.
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetKeepAliveConfig(net.KeepAliveConfig{Enable: true, Idle: 5 * time.Second, Interval: 3 * time.Second, Count: 3})
	}
	c := protocolo.NuevaConexion(conn)
	remoto := conn.RemoteAddr().String()

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	msg, err := c.Recibir()
	if errors.Is(err, io.EOF) {
		return // se conectó y se fue sin hablar: la prueba de conexión de scripts/nodo.*
	}
	if err != nil || msg.Tipo != protocolo.TipoRegistro || msg.WorkerID == "" {
		log.Printf("%s: se esperaba un mensaje de registro con worker_id (%v)", remoto, err)
		return
	}
	conn.SetReadDeadline(time.Time{})

	rechazar := func(motivo string) {
		c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, WorkerID: msg.WorkerID, Motivo: motivo})
	}
	m.mu.Lock()
	huellaMaestro := m.huella
	m.mu.Unlock()
	if msg.Huella != huellaMaestro && !m.cfg.AceptarOtraMuestra {
		log.Printf("%s: %s rechazado, su muestra tiene otra huella (%s; la del maestro es %s)", remoto, msg.WorkerID, Corta(msg.Huella), Corta(huellaMaestro))
		rechazar("muestra_distinta: genera (limpieza) o copia la misma data/muestra.bin que el maestro")
		return
	}
	m.mu.Lock()
	cerrada := m.cerrada
	m.mu.Unlock()
	if cerrada {
		log.Printf("%s: registro de %s rechazado: la sesión ya terminó", remoto, msg.WorkerID)
		rechazar("registro_rechazado: la sesión ya terminó")
		return
	}
	// Un nodo que se registra con el nombre de otro que está libre en la sala
	// (no en una corrida) es casi siempre el mismo nodo, que reinició o
	// perdió la red: su conexión anterior está muerta aunque el maestro no se
	// haya enterado. Se reemplaza en vez de rechazarlo para siempre. Con una
	// corrida en curso no se toca: se rechaza como antes.
	m.mu.Lock()
	viejo, hay := m.nodos[msg.WorkerID]
	reemplazar := hay && !m.ocupados[msg.WorkerID]
	if reemplazar {
		delete(m.nodos, msg.WorkerID)
	}
	m.mu.Unlock()
	if reemplazar {
		log.Printf("%s: %s vuelve a registrarse y su conexión anterior (%s) estaba libre: se reemplaza", remoto, msg.WorkerID, viejo.vista.Remoto)
		m.registro.DarDeBaja(msg.WorkerID)
		viejo.conexion.Cerrar()
	}
	if _, err := m.registro.Registrar(msg.WorkerID, c); err != nil {
		log.Printf("%s: registro de %s rechazado: %v", remoto, msg.WorkerID, err)
		rechazar("registro_rechazado: " + err.Error())
		return
	}

	m.mu.Lock()
	m.llegadas++
	m.nodos[msg.WorkerID] = &nodoSala{
		vista:       NodoVista{ID: msg.WorkerID, Motor: msg.Motor, Dispositivo: msg.Dispositivo, Hardware: msg.Hardware, Remoto: remoto, Estado: "libre", ConectadoDesde: time.Now(), Huella: msg.Huella},
		conexion:    c,
		orden:       m.llegadas,
		puertoPares: msg.PuertoPares,
		ipLocal:     soloIP(conn.LocalAddr().String()),
	}
	enSala := len(m.nodos)
	m.mu.Unlock()
	log.Printf("nodo %s conectado desde %s, motor %s (%s) → en la sala (%d de %d)", msg.WorkerID, remoto, msg.Motor, msg.Dispositivo, enSala, m.cfg.Capacidad)
	if msg.Hardware != nil {
		log.Printf("nodo %s, hardware: %s", msg.WorkerID, msg.Hardware.Resumen())
	}
	m.senalar()
	m.publicar()

	motivo := m.escuchar(c, msg.WorkerID)

	// Solo se limpia si el nodo de la sala sigue siendo esta conexión: si la
	// reemplazó otra del mismo nombre, no hay nada que quitar.
	m.mu.Lock()
	actual, esta := m.nodos[msg.WorkerID]
	esta = esta && actual.conexion == c
	if esta {
		delete(m.nodos, msg.WorkerID)
	}
	activa := m.activa
	m.mu.Unlock()
	if !esta {
		log.Printf("conexión anterior de %s cerrada (%s)", msg.WorkerID, motivo)
		return
	}
	m.registro.DarDeBaja(msg.WorkerID)
	m.nodoSalioDeLimpieza(msg.WorkerID, motivo)
	m.nodoSalioDePrediccion(msg.WorkerID, motivo)
	if activa != nil {
		if _, participa := activa.indice[msg.WorkerID]; participa {
			activa.avisar(evento{tipo: caida, id: msg.WorkerID, motivo: motivo})
		}
	}
	log.Printf("nodo %s salió de la sala (%s)", msg.WorkerID, motivo)
	m.senalar()
	m.publicar()
}

func (m *Maestro) escuchar(c *protocolo.Conexion, id string) string {
	for {
		msg, err := c.Recibir()
		switch {
		case errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed):
			return "desconexión"
		case err != nil:
			return "caída de la conexión: " + err.Error()
		case msg.Tipo == protocolo.TipoBaja:
			return "baja: " + msg.Motivo
		case msg.Tipo == protocolo.TipoProgresoLimpieza, msg.Tipo == protocolo.TipoLimpiezaTerminada:
			m.mensajeLimpieza(id, msg)
		case msg.Tipo == protocolo.TipoPredicciones:
			m.mensajePrediccion(id, msg)
		case msg.Tipo == protocolo.TipoHuellaMuestra:
			m.huellaRecibida(id, msg.Huella)
		case msg.Tipo == protocolo.TipoGradienteParcial:
			m.mu.Lock()
			activa := m.activa
			m.mu.Unlock()
			if activa == nil || msg.Corrida != activa.id {
				continue // de una corrida que ya terminó
			}
			if _, participa := activa.indice[id]; participa {
				activa.avisar(evento{tipo: gradiente, id: id, msg: msg})
			}
		default:
			log.Printf("nodo %s envió %q, se ignora", id, msg.Tipo)
		}
	}
}

// EstadoNodos resume la sala para el log.
func (m *Maestro) EstadoNodos() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.nodos))
	for _, n := range m.ordenSala() {
		ids = append(ids, n.vista.ID)
	}
	return fmt.Sprintf("nodos en la sala %d/%d [%s]", len(ids), m.cfg.Capacidad, strings.Join(ids, " "))
}

// ordenSala devuelve los nodos por orden de llegada. Requiere m.mu.
func (m *Maestro) ordenSala() []*nodoSala {
	lista := make([]*nodoSala, 0, len(m.nodos))
	for _, n := range m.nodos {
		lista = append(lista, n)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].orden < lista[j].orden })
	return lista
}

// EsperarNodos bloquea hasta que haya al menos n nodos en la sala.
func (m *Maestro) EsperarNodos(ctx context.Context, n int) error {
	for {
		m.mu.Lock()
		hay := len(m.nodos)
		m.mu.Unlock()
		if hay >= n {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.cambios:
			log.Print(m.EstadoNodos())
		}
	}
}

// Quitar despide a un nodo libre de la sala (botón "Quitar" de la página).
func (m *Maestro) Quitar(id string) error {
	m.mu.Lock()
	n, ok := m.nodos[id]
	ocupado := m.ocupados[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %s no está en la sala", ErrNodoNoDisponible, id)
	}
	if ocupado {
		return fmt.Errorf("%w: %s está en una corrida", ErrNodoNoDisponible, id)
	}
	err := n.conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, WorkerID: id, Motivo: MotivoSesionTerminada})
	// Se cierra también del lado del maestro: si la conexión estaba muerta, el
	// aviso no llega a nadie y la entrada no se liberaría sola.
	n.conexion.Cerrar()
	return err
}

// Despedir avisa a todos los nodos con el motivo dado (p. ej.
// MotivoSesionTerminada para que se cierren, o MotivoMaestroCerrando).
func (m *Maestro) Despedir(motivo string) {
	m.mu.Lock()
	lista := m.ordenSala()
	m.mu.Unlock()
	for _, n := range lista {
		n.conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, Motivo: motivo})
	}
}

// Entrenar es el modo de una corrida: espera a que la sala esté llena y
// entrena con todos los nodos (los que se registraron, en ese orden).
func (m *Maestro) Entrenar(ctx context.Context, epocas int, tasa float64) (*Resultado, error) {
	if err := m.EsperarNodos(ctx, m.cfg.Capacidad); err != nil {
		return nil, err
	}
	return m.Correr(ctx, OpcionesCorrida{Epocas: epocas, Tasa: tasa, CerrarSala: true})
}

// Correr entrena con los nodos elegidos (Iniciar + Ejecutar).
func (m *Maestro) Correr(ctx context.Context, op OpcionesCorrida) (*Resultado, error) {
	c, err := m.Iniciar(op)
	if err != nil {
		return nil, err
	}
	return c.Ejecutar(ctx)
}

// Corrida es una corrida reservada: sus nodos ya están marcados como
// ocupados y nadie más puede empezar otra hasta que termine.
type Corrida struct {
	m      *Maestro
	op     OpcionesCorrida
	activa *corridaActiva
	nodos  []*nodoSala
	estado *EstadoCorrida
	datos  *entrenador.Datos // la muestra con la que empezó
	huella string
}

// Iniciar valida y reserva una corrida. Falla con ErrOcupado si hay otra
// en curso, o con ErrNodoNoDisponible si algún nodo no está libre.
func (m *Maestro) Iniciar(op OpcionesCorrida) (*Corrida, error) {
	if op.Epocas < 1 {
		return nil, errors.New("la corrida necesita al menos 1 época")
	}
	switch op.Modelo {
	case "", "logistica":
		op.Modelo = "logistica"
	case "red":
		if len(op.Capas) == 0 {
			op.Capas = red.CapasPorDefecto
		}
		if op.PasosPorEpoca < 1 {
			op.PasosPorEpoca = PasosRedPorDefecto
		}
		if op.Tasa <= 0 || op.Tasa == 0.1 {
			op.Tasa = TasaRedPorDefecto // la tasa de la logística es demasiado alta para Adam
		}
		if op.Comunicacion == "" {
			op.Comunicacion = pares.Central
		}
		if !pares.Valido(op.Comunicacion) {
			return nil, fmt.Errorf("comunicación desconocida %q (usa central, arbol o anillo)", op.Comunicacion)
		}
	default:
		return nil, fmt.Errorf("modelo desconocido %q (usa logistica o red)", op.Modelo)
	}
	if err := m.recargar(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activa != nil {
		return nil, ErrOcupado
	}
	if m.limpiezaEnCurso() {
		return nil, ErrLimpiando
	}
	if m.prediccionEnCurso() {
		return nil, fmt.Errorf("%w: hay una predicción en curso", ErrOcupado)
	}
	if m.datos == nil {
		return nil, ErrSinMuestra
	}
	ids := op.Nodos
	if len(ids) == 0 {
		for _, n := range m.ordenSala() {
			ids = append(ids, n.vista.ID)
		}
	}
	if len(ids) == 0 {
		return nil, ErrSinNodos
	}
	vistos := map[string]bool{}
	var nodos []*nodoSala
	var distintos []string
	for _, id := range ids {
		n, ok := m.nodos[id]
		if !ok || vistos[id] {
			return nil, fmt.Errorf("%w: %s", ErrNodoNoDisponible, id)
		}
		vistos[id] = true
		nodos = append(nodos, n)
		if n.vista.Huella != m.huella {
			distintos = append(distintos, fmt.Sprintf("%s (%s)", id, cortaOVacia(n.vista.Huella)))
		}
	}
	if op.Modelo == "red" && op.Comunicacion != pares.Central {
		for _, n := range nodos {
			if n.puertoPares == 0 {
				return nil, fmt.Errorf("%w: %s no tiene puerto de pares (actualiza su programa) y no puede comunicarse en %s", ErrNodoNoDisponible, n.vista.ID, op.Comunicacion)
			}
		}
	}
	if len(distintos) > 0 {
		return nil, fmt.Errorf("%w: estos nodos no tienen el mismo muestra.bin que el maestro (%s): %s", ErrMuestraDistinta, Corta(m.huella), strings.Join(distintos, ", "))
	}

	m.siguiente++
	activa := &corridaActiva{id: m.siguiente, indice: map[string]int{}, eventos: make(chan evento, 64), fin: make(chan struct{}), huella: m.huella}
	estado := &EstadoCorrida{ID: m.siguiente, Estado: EstadoEntrenando, Epocas: op.Epocas, Inicio: time.Now(), Modelo: describirModelo(op), PasosPorEpoca: op.PasosPorEpoca, Comunicacion: op.Comunicacion}
	if op.Modelo == "red" {
		estado.Parametros = red.NumParametros(red.Arquitectura(len(m.datos.Entrenamiento.X[0]), op.Capas))
	} else {
		estado.Parametros = len(m.datos.Entrenamiento.X[0]) + 1
	}
	for i, n := range nodos {
		activa.indice[n.vista.ID] = i
		m.ocupados[n.vista.ID] = true
		estado.Nodos = append(estado.Nodos, NodoCorrida{ID: n.vista.ID, Motor: n.vista.Motor, Dispositivo: n.vista.Dispositivo, Hardware: n.vista.Hardware, Remoto: n.vista.Remoto, Particion: i})
	}
	m.activa = activa
	m.estado = estado
	return &Corrida{m: m, op: op, activa: activa, nodos: nodos, estado: estado, datos: m.datos, huella: m.huella}, nil
}

// Ejecutar corre el entrenamiento reservado por Iniciar. Si un nodo se
// cae o no responde a tiempo, aborta con error: el modo síncrono no puede
// seguir sin su partición.
func (c *Corrida) Ejecutar(ctx context.Context) (*Resultado, error) {
	m, op := c.m, c.op
	defer func() {
		close(c.activa.fin)
		m.mu.Lock()
		m.activa = nil
		for _, n := range c.nodos {
			delete(m.ocupados, n.vista.ID)
		}
		m.mu.Unlock()
		m.publicar()
	}()
	m.publicar()

	res, err := c.entrenar(ctx)
	if err != nil {
		estado := EstadoAbortado
		if errors.Is(err, context.Canceled) {
			estado = EstadoCancelado
		}
		c.terminar(estado, err.Error(), nil)
		if op.Continuar {
			// Los nodos que siguen conectados vuelven a la sala.
			for _, n := range c.nodos {
				n.conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoEntrenamientoFinalizado, Continuar: true, Motivo: "corrida_" + estado})
			}
		}
		return nil, err
	}
	return res, nil
}

func (c *Corrida) entrenar(ctx context.Context) (*Resultado, error) {
	m, op := c.m, c.op
	n := len(c.nodos)
	log.Printf("corrida %d: empieza el entrenamiento con %d nodos [%s] (%d épocas, tasa %g)", c.activa.id, n, strings.Join(c.ids(), " "), op.Epocas, op.Tasa)

	// Scatter: a cada nodo su parte de las estaciones, igual para todos o
	// proporcional a su peso.
	filtros, err := c.filtros()
	if err != nil {
		return nil, err
	}
	for i, nodo := range c.nodos {
		err := nodo.conexion.Enviar(protocolo.Mensaje{
			Tipo: protocolo.TipoParticionAsignada, WorkerID: nodo.vista.ID,
			FiltroMongo: filtros[i], Huella: c.huella, Modelo: modeloMsg(op), Capas: capasMsg(op, c.datos), PasosPorEpoca: op.PasosPorEpoca,
			Comunicacion: comunicacionMsg(op), Rango: i, Pares: c.direccionesPares(), Epocas: op.Epocas, Tasa: op.Tasa,
		})
		if err != nil {
			return nil, errCaida(0, nodo.vista.ID, "no se le pudo enviar la partición: "+err.Error())
		}
	}
	if op.CerrarSala {
		defer func() {
			m.mu.Lock()
			m.cerrada = true
			m.mu.Unlock()
		}()
	}

	if op.Modelo == "red" {
		return c.entrenarRed(ctx)
	}

	X := c.datos.Entrenamiento.X
	modelo := logistica.NuevoModelo(len(X[0]))
	res := &Resultado{ID: c.activa.id, Modelo: modelo, Normalizador: c.datos.Normalizador}
	tCalcNodo := make([]time.Duration, n)
	nNodo := make([]int, n)
	motorNodo := make([]string, n)
	paso := max(op.Epocas/10, 1)
	inicio := time.Now()

	for e := 0; e < op.Epocas; e++ {
		inicioEpoca := time.Now()
		for _, nodo := range c.nodos {
			err := nodo.conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoParametros, Corrida: c.activa.id, Epoca: e, Pesos: modelo.Pesos, Sesgo: modelo.Sesgo})
			if err != nil {
				// Si se cayó entre dos épocas, su conexión ya está cerrada
				// aunque el aviso de caída aún no se haya leído.
				return nil, errCaida(e, nodo.vista.ID, "no se le pudieron enviar los parámetros: "+err.Error())
			}
		}

		parciales := make([]Parcial, n)
		recibido := make([]bool, n)
		calcEpoca := make([]float64, n)
		var calcMax time.Duration
		limite := time.NewTimer(m.cfg.EsperaGradiente)
		for faltan := n; faltan > 0; {
			var ev evento
			select {
			case <-ctx.Done():
				limite.Stop()
				return nil, ctx.Err()
			case <-limite.C:
				return nil, fmt.Errorf("época %d: %d nodo(s) no enviaron su gradiente en %s", e, faltan, m.cfg.EsperaGradiente)
			case ev = <-c.activa.eventos:
			}
			i := c.activa.indice[ev.id]
			if ev.tipo == caida {
				limite.Stop()
				return nil, errCaida(e, ev.id, ev.motivo)
			}
			if recibido[i] {
				continue
			}
			if ev.msg.Epoca != e {
				log.Printf("nodo %s envió el gradiente de la época %d durante la %d, se ignora", ev.id, ev.msg.Epoca, e)
				continue
			}
			parciales[i] = Parcial{Grad: ev.msg.Gradiente, GradSesgo: ev.msg.GradienteSesgo, Perdida: ev.msg.Perdida, N: ev.msg.NMuestras}
			calc := time.Duration(ev.msg.TCalculoMs * float64(time.Millisecond))
			calcMax = max(calcMax, calc)
			tCalcNodo[i] += calc
			calcEpoca[i] = ev.msg.TCalculoMs
			nNodo[i] = ev.msg.NMuestras
			motorNodo[i] = ev.msg.Motor
			recibido[i] = true
			faltan--
		}
		limite.Stop()

		perdida, err := Actualizar(modelo, op.Tasa, parciales, len(X))
		if err != nil {
			return nil, fmt.Errorf("época %d: %w", e, err)
		}
		res.Perdidas = append(res.Perdidas, perdida)
		tEpoca := time.Since(inicioEpoca)
		red := max(tEpoca-calcMax, 0)
		res.TCalculo += calcMax
		res.TComunicacion += red
		if e%paso == 0 || e == op.Epocas-1 {
			log.Printf("época %3d: pérdida=%.6f | %7.2f ms (cálculo del más lento %6.2f ms, red y espera %7.2f ms)",
				e, perdida, ms(tEpoca), ms(calcMax), ms(red))
		}

		m.mu.Lock()
		est := c.estado
		est.Epoca = e + 1
		est.Perdidas = append(est.Perdidas, perdida)
		est.TEpocaMs = append(est.TEpocaMs, ms(tEpoca))
		est.TCalculoMs = append(est.TCalculoMs, ms(calcMax))
		est.TRedMs = append(est.TRedMs, ms(red))
		for i := range est.Nodos {
			est.Nodos[i].CalculoMs = append(est.Nodos[i].CalculoMs, calcEpoca[i])
			est.Nodos[i].Ejemplos = nNodo[i]
			if motorNodo[i] != "" {
				est.Nodos[i].Motor = motorNodo[i]
			}
		}
		m.mu.Unlock()
		m.publicar()
	}
	res.Duracion = time.Since(inicio)

	final := res.Perdidas[len(res.Perdidas)-1]
	for _, nodo := range c.nodos {
		nodo.conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoEntrenamientoFinalizado, EpocaFinal: op.Epocas, PerdidaFinal: final, Continuar: op.Continuar})
	}
	// Los nodos quedan libres ya: la verificación es solo del maestro.
	m.mu.Lock()
	for _, nodo := range c.nodos {
		delete(m.ocupados, nodo.vista.ID)
	}
	m.mu.Unlock()

	for i, nodo := range c.nodos {
		motor := nodo.vista.Motor
		if motorNodo[i] != "" {
			motor = motorNodo[i]
		}
		res.Nodos = append(res.Nodos, InfoNodo{ID: nodo.vista.ID, Motor: motor, Dispositivo: nodo.vista.Dispositivo, Hardware: nodo.vista.Hardware, Remoto: nodo.vista.Remoto, Particion: i, N: nNodo[i], TCalculo: tCalcNodo[i]})
	}
	res.Evaluacion = entrenador.Evaluar(modelo, c.datos.Prueba)

	if op.Verificar {
		m.mu.Lock()
		c.estado.Estado = EstadoVerificando
		m.mu.Unlock()
		m.publicar()
		log.Printf("verificando: entreno lo mismo en secuencial en esta PC para comparar (%d ejemplos; puede tardar ~1 min, no cierres la ventana)...", len(X))
		secuencial := logistica.NuevoModelo(len(X[0]))
		inicioSec := time.Now()
		perdidas := secuencial.EntrenarConAvance(X, c.datos.Entrenamiento.Y, op.Tasa, op.Epocas, c.datos.PesosClase, func(e int) {
			m.mu.Lock()
			c.estado.VerificacionEpoca = e
			m.mu.Unlock()
			m.publicar()
		})
		v := &Verificacion{Perdidas: perdidas, Duracion: time.Since(inicioSec)}
		v.Diferencia = math.Abs(final - perdidas[len(perdidas)-1])
		v.Evaluacion = entrenador.Evaluar(secuencial, c.datos.Prueba)
		res.Verificacion = v
		m.mu.Lock()
		if t1 := float64(v.Duracion.Microseconds()) / 1000 / float64(op.Epocas); t1 > 0 {
			m.t1EpocaMs = t1 // la medición completa reemplaza a la estimación inicial
		}
		m.mu.Unlock()
	}
	c.terminar(EstadoTerminado, "", res)
	return res, nil
}

// filtros arma la partición de cada nodo: una cubeta del hash cada uno
// (reparto igual) o varias, proporcionales a los pesos de la corrida.
func (c *Corrida) filtros() ([]*protocolo.FiltroMongo, error) {
	n := len(c.nodos)
	filtros := make([]*protocolo.FiltroMongo, n)
	if len(c.op.Pesos) == 0 {
		for i := range filtros {
			filtros[i] = &protocolo.FiltroMongo{Mod: n, Resto: i}
		}
		return filtros, nil
	}
	pesos := make([]float64, n)
	for i, nodo := range c.nodos {
		p, ok := c.op.Pesos[nodo.vista.ID]
		if !ok {
			return nil, fmt.Errorf("falta el peso del nodo %s para el reparto proporcional", nodo.vista.ID)
		}
		pesos[i] = p
	}
	restos, err := particion.RepartirProporcional(pesos, particion.BucketsReparto)
	if err != nil {
		return nil, err
	}
	for i := range filtros {
		filtros[i] = &protocolo.FiltroMongo{Mod: particion.BucketsReparto, Restos: restos[i]}
		log.Printf("reparto proporcional: %s (peso %g) recibe %d de %d cubetas", c.nodos[i].vista.ID, pesos[i], len(restos[i]), particion.BucketsReparto)
	}
	return filtros, nil
}

func (c *Corrida) ids() []string {
	ids := make([]string, len(c.nodos))
	for i, n := range c.nodos {
		ids[i] = n.vista.ID
	}
	return ids
}

// terminar cierra el estado de la corrida y la agrega al historial.
func (c *Corrida) terminar(estado, mensaje string, res *Resultado) {
	m := c.m
	m.mu.Lock()
	defer m.mu.Unlock()
	est := c.estado
	est.Estado, est.Mensaje = estado, mensaje
	r := ResumenCorrida{ID: est.ID, Epocas: est.Epocas, Estado: estado, Modelo: est.Modelo}
	for _, n := range est.Nodos {
		r.Nodos = append(r.Nodos, n.ID)
		r.Motores = append(r.Motores, n.Motor)
	}
	if res != nil {
		epocas := float64(len(res.Perdidas))
		r.TTotalS = res.Duracion.Seconds()
		r.TEpocaMs = ms(res.Duracion) / epocas
		r.TCalculoMs = ms(res.TCalculo) / epocas
		r.TComunicacionMs = ms(res.TComunicacion) / epocas
		r.PerdidaFinal = res.Perdidas[len(res.Perdidas)-1]
		r.AUC, r.F1 = res.Evaluacion.AUC, res.Evaluacion.F1
		if v := res.Verificacion; v != nil {
			r.Verificado, r.Diferencia, r.TSecuencialS = true, v.Diferencia, v.Duracion.Seconds()
		}
		t1 := m.t1EpocaMs
		if res.Red != nil {
			t1 = res.T1EpocaMs // la red solo tiene T(1) si se verificó; la de la logística no sirve
		}
		if t1 > 0 && r.TEpocaMs > 0 {
			r.Speedup = t1 / r.TEpocaMs
			r.Eficiencia = r.Speedup / float64(len(r.Nodos))
		}
		est.Resultado = &r
		if estado == EstadoTerminado {
			mod := &ModeloEntrenado{Nombre: est.Modelo, Evaluacion: res.Evaluacion, Huella: c.huella}
			if res.Red != nil {
				mod.Capas, mod.Params = res.Red.Capas, append([]float64(nil), res.Red.Params...)
			} else if res.Modelo != nil {
				mod.Params = append(append([]float64(nil), res.Modelo.Pesos...), res.Modelo.Sesgo)
			}
			if len(mod.Params) > 0 {
				m.ultimoModelo = mod
			}
		}
		for _, n := range res.Nodos {
			if n.TCalculo > 0 && n.N > 0 {
				m.rendEntreno[n.ID] = float64(n.N) / (ms(n.TCalculo) / epocas)
			}
		}
	}
	m.historial = append(m.historial, r)
}

// MedirT1 estima cuánto dura una época en secuencial (un hilo) en esta PC
// con unas pocas épocas de prueba (la mediana), para mostrar el speedup en
// vivo. Antes se descarta una época de calentamiento: la primera pasada por
// 2,5 millones de ejemplos recién cargados es más lenta (memoria fría) y
// inflaba el T(1), y con él el speedup que ve la página.
func (m *Maestro) MedirT1(epocas int) float64 {
	m.mu.Lock()
	datos := m.datos
	m.mu.Unlock()
	if datos == nil {
		return 0
	}
	X, y := datos.Entrenamiento.X, datos.Entrenamiento.Y
	pesos := make([]float64, len(X[0]))
	logistica.GradienteParcial(pesos, 0, X, y, datos.PesosClase)
	tiempos := make([]float64, 0, epocas)
	for e := 0; e < max(epocas, 1); e++ {
		inicio := time.Now()
		logistica.GradienteParcial(pesos, 0, X, y, datos.PesosClase)
		tiempos = append(tiempos, ms(time.Since(inicio)))
	}
	sort.Float64s(tiempos)
	t1 := tiempos[len(tiempos)/2]
	m.mu.Lock()
	m.t1EpocaMs = t1
	m.mu.Unlock()
	return t1
}

// Instantanea arma el estado actual para la página.
func (m *Maestro) Instantanea() Instantanea {
	m.mu.Lock()
	defer m.mu.Unlock()
	ejemplos := 0
	if m.datos != nil {
		ejemplos = m.datos.Entrenamiento.Len()
	}
	inst := Instantanea{
		Huella:                m.huella,
		EjemplosEntrenamiento: ejemplos,
		Limpieza:              m.limpieza.copia(),
		PesosSugeridos:        map[string]float64{},
		NodoMaestro:           m.cfg.NodoMaestro,
		Prediccion:            m.prediccion.copia(),
		Capacidad:             m.cfg.Capacidad,
		T1EpocaMs:             m.t1EpocaMs,
		Ocupado:               m.activa != nil,
		Sala:                  []NodoVista{},
		Corrida:               m.estado.copia(),
		Historial:             append([]ResumenCorrida{}, m.historial...),
	}
	inst.PesosSugeridos, inst.OrigenPesos = m.pesosSugeridos()
	if m.ultimoModelo != nil {
		inst.ModeloListo = m.ultimoModelo.Nombre
	}
	for _, n := range m.ordenSala() {
		v := n.vista
		if m.ocupados[v.ID] {
			v.Estado = "en_corrida"
		} else if m.limpiandoNodo(v.ID) {
			v.Estado = "limpiando"
		}
		inst.Sala = append(inst.Sala, v)
	}
	return inst
}

func (m *Maestro) publicar() {
	if m.cfg.Notificar != nil {
		m.cfg.Notificar(m.Instantanea())
	}
}

func errCaida(epoca int, id, motivo string) error {
	return fmt.Errorf("época %d: el nodo %s se cayó (%s); el modo síncrono no puede seguir sin su partición", epoca, id, motivo)
}

func cortaOVacia(h string) string {
	if h == "" {
		return "sin muestra"
	}
	return Corta(h)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func modeloMsg(op OpcionesCorrida) string {
	if op.Modelo == "red" {
		return "red"
	}
	return ""
}

func capasMsg(op OpcionesCorrida, d *entrenador.Datos) []int {
	if op.Modelo != "red" {
		return nil
	}
	return red.Arquitectura(len(d.Entrenamiento.X[0]), op.Capas)
}

// pesosSugeridos propone el reparto proporcional para los nodos de la sala
// (Fase 3): su rendimiento en el último entrenamiento, relativo al más
// lento; si alguno no entrenó todavía, su velocidad en la limpieza con Go.
// Sin mediciones de todos, no propone nada (reparto igual). Requiere m.mu.
func (m *Maestro) pesosSugeridos() (map[string]float64, string) {
	for _, fuente := range []struct {
		rend   map[string]float64
		origen string
	}{{m.rendEntreno, "entrenamiento"}, {m.rendLimpieza, "limpieza"}} {
		pesos := map[string]float64{}
		minimo := 0.0
		completo := len(m.nodos) > 0
		for id := range m.nodos {
			r, ok := fuente.rend[id]
			if !ok || r <= 0 {
				completo = false
				break
			}
			pesos[id] = r
			if minimo == 0 || r < minimo {
				minimo = r
			}
		}
		if !completo {
			continue
		}
		for id, r := range pesos {
			pesos[id] = math.Round(100*r/minimo) / 100
		}
		return pesos, fuente.origen
	}
	return map[string]float64{}, ""
}

func comunicacionMsg(op OpcionesCorrida) string {
	if op.Modelo == "red" && op.Comunicacion != pares.Central {
		return op.Comunicacion
	}
	return ""
}

// direccionesPares: la dirección de pares de cada nodo de la corrida, en
// orden de rango. El nodo que corre en la laptop del maestro se conectó por
// 127.0.0.1: a los demás se les da la IP del maestro por la que ellos
// llegaron (la misma laptop).
func (c *Corrida) direccionesPares() []string {
	if comunicacionMsg(c.op) == "" {
		return nil
	}
	ipMaestro := ""
	for _, n := range c.nodos {
		if ip := soloIP(n.vista.Remoto); !esLoopback(ip) {
			ipMaestro = n.ipLocal
			break
		}
	}
	dirs := make([]string, len(c.nodos))
	for i, n := range c.nodos {
		ip := soloIP(n.vista.Remoto)
		if esLoopback(ip) && ipMaestro != "" {
			ip = ipMaestro
		}
		dirs[i] = net.JoinHostPort(ip, strconv.Itoa(n.puertoPares))
	}
	return dirs
}

func soloIP(dir string) string {
	if h, _, err := net.SplitHostPort(dir); err == nil {
		return h
	}
	return dir
}

func esLoopback(ip string) bool {
	p := net.ParseIP(ip)
	return p != nil && p.IsLoopback()
}
