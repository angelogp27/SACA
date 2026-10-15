package distribuido

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"proyecto9/internal/cluster"
	"proyecto9/internal/entrenador"
	"proyecto9/internal/logistica"
	"proyecto9/internal/protocolo"
)

// ConfigMaestro son los parámetros de un entrenamiento distribuido.
type ConfigMaestro struct {
	Nodos  int
	Datos  *entrenador.Datos
	Huella string
	Tasa   float64
	Epocas int
	// EsperaGradiente es cuánto se espera a un nodo en cada época antes
	// de abortar: un nodo colgado no debe colgar la demo.
	EsperaGradiente time.Duration
}

// InfoNodo describe a un nodo para el reporte final.
type InfoNodo struct {
	ID, Motor, Dispositivo, Remoto string
	Particion, N                   int
	TCalculo                       time.Duration // suma de todas las épocas
}

// Resultado de un entrenamiento distribuido. TCalculo es la suma, época
// por época, del cálculo del nodo más lento (el que marca el ritmo);
// TComunicacion es el resto del tiempo de cada época: red, serialización
// y espera.
type Resultado struct {
	Modelo        *logistica.Modelo
	Perdidas      []float64
	Duracion      time.Duration
	TCalculo      time.Duration
	TComunicacion time.Duration
	Nodos         []InfoNodo
}

type tipoEvento int

const (
	alta tipoEvento = iota
	gradiente
	caida
)

type evento struct {
	tipo   tipoEvento
	id     string
	msg    protocolo.Mensaje
	motivo string
}

// Maestro coordina el entrenamiento síncrono (pasos 3 a 6 del contrato):
// difunde los parámetros, espera el gradiente parcial de todos los nodos,
// los suma y actualiza. Cada conexión la atiende su propia goroutine
// (Atender), que le pasa los mensajes al bucle de entrenamiento por un
// channel; así el modelo solo lo toca una goroutine y no necesita locks.
type Maestro struct {
	cfg      ConfigMaestro
	registro *cluster.Registro
	eventos  chan evento
	fin      chan struct{}
	cerrar   sync.Once

	mu    sync.Mutex
	infos map[string]*InfoNodo
}

func NuevoMaestro(cfg ConfigMaestro) *Maestro {
	if cfg.EsperaGradiente <= 0 {
		cfg.EsperaGradiente = 2 * time.Minute
	}
	return &Maestro{
		cfg:      cfg,
		registro: cluster.NuevoRegistro(cfg.Nodos),
		eventos:  make(chan evento, 64),
		fin:      make(chan struct{}),
		infos:    map[string]*InfoNodo{},
	}
}

// avisar entrega un evento al bucle de entrenamiento; si este ya terminó,
// lo descarta para no dejar goroutines bloqueadas.
func (m *Maestro) avisar(ev evento) {
	select {
	case m.eventos <- ev:
	case <-m.fin:
	}
}

// Atender es la goroutine dedicada a una conexión: valida el registro,
// asigna la partición y reenvía los mensajes del nodo al bucle de
// entrenamiento hasta que el nodo se va.
func (m *Maestro) Atender(conn net.Conn) {
	defer conn.Close()
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

	if msg.Huella != m.cfg.Huella {
		log.Printf("%s: %s rechazado, su muestra tiene otra huella (%s; la del maestro es %s)", remoto, msg.WorkerID, Corta(msg.Huella), Corta(m.cfg.Huella))
		c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, WorkerID: msg.WorkerID, Motivo: "muestra_distinta: copia la misma data/muestra.gob que el maestro"})
		return
	}
	nodo, err := m.registro.Registrar(msg.WorkerID, c)
	if err != nil {
		log.Printf("%s: registro de %s rechazado: %v", remoto, msg.WorkerID, err)
		c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, WorkerID: msg.WorkerID, Motivo: "registro_rechazado: " + err.Error()})
		return
	}
	m.mu.Lock()
	m.infos[nodo.ID] = &InfoNodo{ID: nodo.ID, Motor: msg.Motor, Dispositivo: msg.Dispositivo, Remoto: remoto, Particion: nodo.Particion}
	m.mu.Unlock()

	err = c.Enviar(protocolo.Mensaje{
		Tipo:        protocolo.TipoParticionAsignada,
		WorkerID:    nodo.ID,
		FiltroMongo: &protocolo.FiltroMongo{Mod: m.registro.Particiones(), Resto: nodo.Particion},
	})
	if err != nil {
		m.registro.DarDeBaja(nodo.ID)
		log.Printf("%s: no se pudo enviar la partición: %v", nodo.ID, err)
		return
	}
	log.Printf("nodo %s registrado desde %s → partición %d/%d, motor %s (%s)", nodo.ID, remoto, nodo.Particion, m.registro.Particiones(), msg.Motor, msg.Dispositivo)
	m.avisar(evento{tipo: alta, id: nodo.ID})

	motivo := m.escuchar(c, nodo.ID)
	if _, estaba := m.registro.DarDeBaja(nodo.ID); estaba {
		m.avisar(evento{tipo: caida, id: nodo.ID, motivo: motivo})
	}
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
		case msg.Tipo == protocolo.TipoGradienteParcial:
			m.avisar(evento{tipo: gradiente, id: id, msg: msg})
		default:
			log.Printf("nodo %s envió %q, se ignora", id, msg.Tipo)
		}
	}
}

// EstadoNodos resume los nodos activos para el log.
func (m *Maestro) EstadoNodos() string {
	activos := m.registro.Activos()
	ids := make([]string, len(activos))
	for i, n := range activos {
		ids[i] = fmt.Sprintf("%s(p%d)", n.ID, n.Particion)
	}
	return fmt.Sprintf("nodos activos %d/%d [%s]", len(activos), m.registro.Particiones(), strings.Join(ids, " "))
}

// Entrenar espera a que se registren todos los nodos y corre las épocas.
// Si un nodo se cae o no responde a tiempo, aborta con error: el modo
// síncrono no puede seguir sin su partición (la redistribución queda para
// más adelante).
func (m *Maestro) Entrenar(ctx context.Context) (*Resultado, error) {
	defer m.cerrar.Do(func() { close(m.fin) })

	for !m.registro.Completo() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case ev := <-m.eventos:
			switch ev.tipo {
			case alta:
				log.Print(m.EstadoNodos())
			case caida:
				log.Printf("nodo %s dado de baja antes de empezar (%s); %s", ev.id, ev.motivo, m.EstadoNodos())
			}
		}
	}

	nodos := m.registro.Activos()
	indice := map[string]int{}
	for i, n := range nodos {
		indice[n.ID] = i
	}
	log.Printf("clúster completo con %d nodos: empieza el entrenamiento (%d épocas, tasa %g)", len(nodos), m.cfg.Epocas, m.cfg.Tasa)

	X := m.cfg.Datos.Entrenamiento.X
	modelo := logistica.NuevoModelo(len(X[0]))
	res := &Resultado{Modelo: modelo}
	tCalcNodo := make([]time.Duration, len(nodos))
	nNodo := make([]int, len(nodos))
	motorNodo := make([]string, len(nodos))
	paso := max(m.cfg.Epocas/10, 1)
	inicio := time.Now()

	for e := 0; e < m.cfg.Epocas; e++ {
		inicioEpoca := time.Now()
		for _, n := range nodos {
			err := n.Conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoParametros, Epoca: e, Pesos: modelo.Pesos, Sesgo: modelo.Sesgo})
			if err != nil {
				// Si se cayó entre dos épocas, su conexión ya está cerrada
				// aunque el aviso de caída aún no se haya leído.
				return nil, errCaida(e, n.ID, "no se le pudieron enviar los parámetros: "+err.Error())
			}
		}

		parciales := make([]Parcial, len(nodos))
		recibido := make([]bool, len(nodos))
		var calcMax time.Duration
		limite := time.NewTimer(m.cfg.EsperaGradiente)
		for faltan := len(nodos); faltan > 0; {
			var ev evento
			select {
			case <-ctx.Done():
				limite.Stop()
				return nil, ctx.Err()
			case <-limite.C:
				return nil, fmt.Errorf("época %d: %d nodo(s) no enviaron su gradiente en %s", e, faltan, m.cfg.EsperaGradiente)
			case ev = <-m.eventos:
			}
			i, delEntrenamiento := indice[ev.id]
			if !delEntrenamiento {
				continue
			}
			if ev.tipo == caida {
				limite.Stop()
				return nil, errCaida(e, ev.id, ev.motivo)
			}
			if ev.tipo != gradiente || recibido[i] {
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
			nNodo[i] = ev.msg.NMuestras
			motorNodo[i] = ev.msg.Motor
			recibido[i] = true
			faltan--
		}
		limite.Stop()

		perdida, err := Actualizar(modelo, m.cfg.Tasa, parciales, len(X))
		if err != nil {
			return nil, fmt.Errorf("época %d: %w", e, err)
		}
		res.Perdidas = append(res.Perdidas, perdida)
		tEpoca := time.Since(inicioEpoca)
		red := max(tEpoca-calcMax, 0)
		res.TCalculo += calcMax
		res.TComunicacion += red
		if e%paso == 0 || e == m.cfg.Epocas-1 {
			log.Printf("época %3d: pérdida=%.6f | %7.2f ms (cálculo del más lento %6.2f ms, red y espera %7.2f ms)",
				e, perdida, ms(tEpoca), ms(calcMax), ms(red))
		}
	}
	res.Duracion = time.Since(inicio)

	final := res.Perdidas[len(res.Perdidas)-1]
	for _, n := range nodos {
		n.Conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoEntrenamientoFinalizado, EpocaFinal: m.cfg.Epocas, PerdidaFinal: final})
	}

	m.mu.Lock()
	for i, n := range nodos {
		info := *m.infos[n.ID]
		info.N, info.TCalculo = nNodo[i], tCalcNodo[i]
		if motorNodo[i] != "" {
			info.Motor = motorNodo[i]
		}
		res.Nodos = append(res.Nodos, info)
	}
	m.mu.Unlock()
	sort.Slice(res.Nodos, func(i, j int) bool { return res.Nodos[i].Particion < res.Nodos[j].Particion })
	return res, nil
}

func errCaida(epoca int, id, motivo string) error {
	return fmt.Errorf("época %d: el nodo %s se cayó (%s); el modo síncrono no puede seguir sin su partición", epoca, id, motivo)
}

// Despedir avisa a los nodos activos que el maestro se va.
func (m *Maestro) Despedir(motivo string) {
	for _, n := range m.registro.Activos() {
		n.Conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, Motivo: motivo})
	}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
