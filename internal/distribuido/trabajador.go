package distribuido

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"proyecto9/internal/entrenador"
	"proyecto9/internal/hardware"
	"proyecto9/internal/limpieza"
	"proyecto9/internal/pares"
	"proyecto9/internal/protocolo"
	"proyecto9/internal/red"
)

// ConfigTrabajador son los datos de un nodo trabajador. Motor es con qué
// calcula (MotorGo o un MotorExterno). Si no es Respaldo, se valida contra
// él antes de usarlo y, si no coincide, el nodo pasa a calcular con
// Respaldo: Trabajar actualiza Motor en ese caso, y quien llama lo cierra
// al final.
type ConfigTrabajador struct {
	ID string
	// Datos y Huella son la muestra cargada (Datos nil si el nodo todavía
	// no tiene). Con RutaMuestra, el nodo relee el archivo si cambió (al
	// conectarse, al recibir su partición y cuando el maestro pregunta su
	// huella): una limpieza o una copia pueden haberlo regenerado.
	Datos       *entrenador.Datos
	Huella      string
	RutaMuestra string
	MesCorte    int
	// Limpieza, si no es nil, permite que el maestro le pida limpiar sus
	// CSV y generar su muestra.bin (iniciar_limpieza).
	Limpieza *limpieza.Config
	// Pares, si no es nil, es el puerto donde este nodo escucha a los otros
	// nodos para sumar los gradientes entre ellos (árbol o anillo, Fase 4).
	Pares    *pares.Pares
	Motor    Motor
	Respaldo *MotorGo
	// Hardware es la huella de esta máquina que se manda al registrarse;
	// si es nil, Trabajar la detecta.
	Hardware *hardware.Info

	validado bool
}

var (
	// ErrRechazado: el maestro no aceptó al nodo (otra muestra, sala llena
	// o nombre repetido).
	ErrRechazado = errors.New("el maestro rechazó el registro")
	// ErrSesionTerminada: el maestro despidió al nodo a propósito (botón
	// "Quitar" o "Terminar sesión" de la página).
	ErrSesionTerminada = errors.New("el maestro terminó la sesión")
	// ErrConexionPerdida: se cortó la conexión o el maestro se cerró; con
	// -reconectar, el nodo vuelve a intentar.
	ErrConexionPerdida = errors.New("se perdió la conexión con el maestro")
)

// Trabajar corre el lado del trabajador del protocolo sobre una conexión
// ya abierta: se registra y queda en la sala del maestro. Por cada
// "particion_asignada" carga su parte de los datos, y por cada
// "parametros" responde un gradiente parcial. Con un maestro de una sola
// corrida termina al recibir "entrenamiento_finalizado"; con un maestro en
// modo servidor (continuar) vuelve a la sala y espera la siguiente.
func Trabajar(c *protocolo.Conexion, cfg *ConfigTrabajador) error {
	if cfg.Hardware == nil {
		h := hardware.Detectar()
		cfg.Hardware = &h
	}
	if cfg.RutaMuestra != "" {
		cfg.Huella = huellaArchivo(cfg.RutaMuestra)
	}
	// Una limpieza en curso se cancela si la sesión termina.
	var lim limpiezaNodo
	defer lim.cancelar(0)
	puertoPares := 0
	if cfg.Pares != nil {
		puertoPares = cfg.Pares.Puerto()
	}
	err := c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoRegistro, WorkerID: cfg.ID, Huella: cfg.Huella, Motor: cfg.Motor.Nombre(), Dispositivo: cfg.Motor.Dispositivo(), Hardware: cfg.Hardware, PuertoPares: puertoPares})
	if err != nil {
		return fmt.Errorf("%w: no se pudo enviar el registro: %v", ErrConexionPerdida, err)
	}
	log.Printf("conectado al maestro como %s; esperando que inicie una corrida", cfg.ID)

	var d int
	var X [][]float64
	var Y []float64
	// Modelo de la corrida: regresión logística (por defecto) o red
	// neuronal por mini-lotes (capas != nil).
	var capas []int
	pasosRed := 1
	// Con comunicación entre nodos: el grupo de pares, los parámetros y el
	// optimizador propios (todos los nodos los mantienen idénticos).
	var grupo *pares.Grupo
	var paramsLocal []float64
	var adamLocal *red.Adam
	var rango, epocasRed int
	defer func() {
		if grupo != nil {
			grupo.Cerrar()
		}
	}()
	cargado := false
	var tCalculo time.Duration
	epocas := 0
	for {
		m, err := c.Recibir()
		switch {
		case errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed):
			return fmt.Errorf("%w: el maestro cerró la conexión", ErrConexionPerdida)
		case err != nil:
			return fmt.Errorf("%w: %v", ErrConexionPerdida, err)

		case m.Tipo == protocolo.TipoBaja:
			switch {
			case strings.HasPrefix(m.Motivo, MotivoSesionTerminada):
				return ErrSesionTerminada
			case strings.HasPrefix(m.Motivo, "registro_rechazado"), strings.HasPrefix(m.Motivo, "muestra_distinta"):
				return fmt.Errorf("%w: %s", ErrRechazado, m.Motivo)
			default:
				return fmt.Errorf("%w: el maestro pidió la baja (%s)", ErrConexionPerdida, m.Motivo)
			}

		case m.Tipo == protocolo.TipoIniciarLimpieza:
			lim.iniciar(c, cfg, m)

		case m.Tipo == protocolo.TipoCancelarLimpieza:
			lim.cancelar(m.Corrida)

		case m.Tipo == protocolo.TipoPredecir:
			if err := cfg.asegurarMuestra(); err != nil {
				log.Printf("AVISO: %v", err)
			}
			log.Printf("predicción %d: predigo las horas de mis estaciones", m.Corrida)
			if err := predecirNodo(c, cfg, m); err != nil {
				if errors.Is(err, ErrConexionPerdida) {
					return err
				}
				log.Printf("predicción %d: %v", m.Corrida, err)
				c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoPredicciones, WorkerID: cfg.ID, Corrida: m.Corrida, Motivo: err.Error(), Ultimo: true})
			}

		case m.Tipo == protocolo.TipoConsultarHuella:
			if cfg.RutaMuestra != "" {
				cfg.Huella = huellaArchivo(cfg.RutaMuestra)
			}
			c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoHuellaMuestra, WorkerID: cfg.ID, Huella: cfg.Huella})

		case m.Tipo == protocolo.TipoParticionAsignada:
			if m.FiltroMongo == nil {
				return fmt.Errorf("particion_asignada sin filtro")
			}
			cargado = false
			if err := cfg.asegurarMuestra(); err != nil {
				log.Printf("AVISO: %v", err)
			}
			if cfg.Datos == nil || (m.Huella != "" && m.Huella != cfg.Huella) {
				// Otra muestra (o ninguna): avisarle al maestro, que aborta la corrida.
				log.Printf("mi muestra (%s) no es la de la corrida (%s): no puedo participar", cortaOVacia(cfg.Huella), cortaOVacia(m.Huella))
				c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoHuellaMuestra, WorkerID: cfg.ID, Huella: cfg.Huella})
				continue
			}
			entrenamiento := cfg.Datos.Entrenamiento
			d = len(entrenamiento.X[0])
			mod, resto := m.FiltroMongo.Mod, m.FiltroMongo.Resto
			var y []float64
			if len(m.FiltroMongo.Restos) > 0 {
				X, y = FiltrarRestos(entrenamiento, mod, m.FiltroMongo.Restos)
				log.Printf("registrado como %s: %d de %d cubetas del hash → %d de %d ejemplos de entrenamiento (%.1f%%)",
					cfg.ID, len(m.FiltroMongo.Restos), mod, len(X), entrenamiento.Len(), 100*float64(len(X))/float64(entrenamiento.Len()))
			} else {
				X, y = Filtrar(entrenamiento, mod, resto)
				log.Printf("registrado como %s: partición %d de %d → %d de %d ejemplos de entrenamiento (%.1f%%)",
					cfg.ID, resto, mod, len(X), entrenamiento.Len(), 100*float64(len(X))/float64(entrenamiento.Len()))
			}
			if err := cfg.cargar(d, X, y); err != nil {
				return err
			}
			cargado, tCalculo, epocas = true, 0, 0
			Y, capas = y, nil
			if m.Modelo == "red" {
				capas, pasosRed = m.Capas, max(m.PasosPorEpoca, 1)
				if grupo != nil {
					grupo.Cerrar()
					grupo = nil
				}
				if m.Comunicacion == pares.Arbol || m.Comunicacion == pares.Anillo {
					if cfg.Pares == nil {
						return fmt.Errorf("el maestro pidió comunicación en %s y este nodo no tiene puerto de pares", m.Comunicacion)
					}
					g, err := cfg.Pares.Unirse(context.Background(), m.Corrida, m.Rango, m.Pares, m.Comunicacion)
					if err != nil {
						return fmt.Errorf("no se pudo conectar con los otros nodos (%s): %w", m.Comunicacion, err)
					}
					grupo, rango, epocasRed = g, m.Rango, m.Epocas
					paramsLocal = red.Inicializar(capas, 1)
					adamLocal = red.NuevoAdam(len(paramsLocal), m.Tasa)
					log.Printf("comunicación en %s: rango %d de %d, conectado con sus pares", m.Comunicacion, m.Rango, len(m.Pares))
				}
				log.Printf("modelo: red neuronal %v (%d parámetros), %d pasos por época", capas, red.NumParametros(capas), m.PasosPorEpoca)
			}
			log.Printf("listo: calcula con el motor %s (%s); esperando los parámetros del maestro", cfg.Motor.Nombre(), cfg.Motor.Dispositivo())

		case m.Tipo == protocolo.TipoReparto:
			// Reparto dinámico: desde la próxima época, estas cubetas. La
			// muestra ya está completa en este nodo; solo cambia qué filas
			// recorre.
			if !cargado || capas == nil || m.FiltroMongo == nil || cfg.Datos == nil {
				continue
			}
			antes := len(X)
			X, Y = FiltrarRestos(cfg.Datos.Entrenamiento, m.FiltroMongo.Mod, m.FiltroMongo.Restos)
			log.Printf("reparto dinámico (época %d): %d → %d ejemplos", m.Epoca, antes, len(X))

		case m.Tipo == protocolo.TipoEntrenamientoFinalizado:
			if grupo != nil {
				grupo.Cerrar()
				grupo = nil
			}
			promedio := 0.0
			if epocas > 0 {
				promedio = ms(tCalculo) / float64(epocas)
			}
			if m.Motivo != "" {
				log.Printf("la corrida terminó sin completarse (%s)", m.Motivo)
			} else {
				log.Printf("entrenamiento finalizado: %d épocas, pérdida final %.6f; este nodo calculó %.2f ms por época en promedio", m.EpocaFinal, m.PerdidaFinal, promedio)
			}
			if !m.Continuar {
				return nil
			}
			cargado = false
			log.Print("de vuelta en la sala: esperando la siguiente corrida (Ctrl+C para salir)")

		case m.Tipo == protocolo.TipoParametros && m.Modelo == "red":
			if !cargado || capas == nil {
				log.Printf("parámetros de la red sin partición asignada, se ignoran")
				continue
			}
			p := paramsLocal
			if grupo == nil {
				var err error
				p, err = red.Decodificar(m.ParamsBin)
				if err != nil || len(p) != red.NumParametros(capas) {
					return fmt.Errorf("época %d paso %d: parámetros de la red inválidos (%v)", m.Epoca, m.Paso, err)
				}
			}
			// Mini-lote: el tramo m.Paso de la partición de este nodo.
			if m.Paso < 0 || m.Paso >= pasosRed {
				return fmt.Errorf("época %d: paso %d fuera de rango (la época tiene %d)", m.Epoca, m.Paso, pasosRed)
			}
			ini, fin := red.Lote(len(X), m.Paso, pasosRed)
			inicio := time.Now()
			hilos := runtime.NumCPU()
			if cfg.Respaldo != nil {
				hilos = cfg.Respaldo.Hilos
			}
			grad, perdida := red.Gradiente(capas, p, X[ini:fin], Y[ini:fin], cfg.Datos.PesosClase, hilos)
			calc := time.Since(inicio)
			tCalculo += calc
			if m.Paso == 0 {
				epocas++
			}
			resp := protocolo.Mensaje{
				Tipo: protocolo.TipoGradienteParcial, WorkerID: cfg.ID, Corrida: m.Corrida, Epoca: m.Epoca, Paso: m.Paso,
				Modelo: "red", Perdida: perdida, NMuestras: fin - ini, TCalculoMs: ms(calc), Motor: "go",
			}
			if grupo == nil {
				resp.GradBin = red.Codificar(grad)
			} else {
				// Suma entre nodos: el gradiente y, al final, el número de
				// ejemplos del paso (para promediar). Todos reciben la misma
				// suma y aplican Adam igual: los parámetros siguen idénticos.
				v := append(grad, float64(fin-ini))
				antes, inicioRed := grupo.Bytes, time.Now()
				if err := grupo.AllReduce(v); err != nil {
					return fmt.Errorf("época %d paso %d: suma entre nodos: %w", m.Epoca, m.Paso, err)
				}
				resp.TRedMs, resp.BytesPares = ms(time.Since(inicioRed)), grupo.Bytes-antes
				if nPaso := int(v[len(v)-1]); nPaso > 0 {
					adamLocal.Paso(paramsLocal, v[:len(v)-1], nPaso)
				}
				// El rango 0 entrega el modelo final al maestro.
				if rango == 0 && m.Epoca == epocasRed-1 && m.Paso == pasosRed-1 {
					resp.ParamsBin = red.Codificar(paramsLocal)
				}
			}
			err := c.Enviar(resp)
			if err != nil {
				return fmt.Errorf("%w: época %d: no se pudo enviar el gradiente: %v", ErrConexionPerdida, m.Epoca, err)
			}

		case m.Tipo == protocolo.TipoParametros:
			if !cargado {
				log.Printf("parámetros de la época %d sin partición asignada, se ignoran", m.Epoca)
				continue
			}
			if len(m.Pesos) != d {
				return fmt.Errorf("época %d: llegaron %d pesos, se esperaban %d", m.Epoca, len(m.Pesos), d)
			}
			inicio := time.Now()
			grad, gradSesgo, perdida, err := cfg.Motor.Gradiente(m.Pesos, m.Sesgo)
			if err != nil {
				return fmt.Errorf("época %d: %w", m.Epoca, err)
			}
			calc := time.Since(inicio)
			tCalculo += calc
			epocas++
			err = c.Enviar(protocolo.Mensaje{
				Tipo: protocolo.TipoGradienteParcial, WorkerID: cfg.ID, Corrida: m.Corrida, Epoca: m.Epoca,
				Gradiente: grad, GradienteSesgo: gradSesgo, Perdida: perdida, NMuestras: len(X),
				TCalculoMs: ms(calc), Motor: cfg.Motor.Nombre(),
			})
			if err != nil {
				return fmt.Errorf("%w: época %d: no se pudo enviar el gradiente: %v", ErrConexionPerdida, m.Epoca, err)
			}
			if m.Epoca%50 == 0 {
				log.Printf("época %d: gradiente enviado (%.2f ms de cálculo)", m.Epoca, ms(calc))
			}

		default:
			log.Printf("mensaje %q inesperado, se ignora", m.Tipo)
		}
	}
}

// cargar entrega la partición al motor. La primera vez que un motor que no
// es el de respaldo recibe datos, se valida contra el de Go en dos puntos
// (pesos en cero y pesos al azar). Si la carga o la validación fallan, el
// nodo sigue con el de respaldo: la demo no se cae por un motor externo
// con problemas, y queda el aviso en el log.
func (cfg *ConfigTrabajador) cargar(d int, X [][]float64, y []float64) error {
	pc := cfg.Datos.PesosClase
	esRespaldo := cfg.Respaldo == nil || cfg.Motor == Motor(cfg.Respaldo)
	if err := cfg.Motor.Cargar(d, X, y, pc); err != nil {
		if esRespaldo {
			return err
		}
		log.Printf("AVISO: %v; se calcula con el motor go", err)
		cfg.Motor.Cerrar()
		cfg.Motor = cfg.Respaldo
		return cfg.Respaldo.Cargar(d, X, y, pc)
	}
	if esRespaldo || cfg.validado {
		return nil
	}

	cfg.Respaldo.Cargar(d, X, y, pc)
	azar := rand.New(rand.NewPCG(1, 2))
	pesos := make([]float64, d)
	for prueba := 0; prueba < 2; prueba++ {
		if prueba == 1 {
			for j := range pesos {
				pesos[j] = azar.NormFloat64()
			}
		}
		if err := Comparar(cfg.Motor, cfg.Respaldo, pesos, 0.1, 1e-8); err != nil {
			log.Printf("AVISO: el motor %s no da el mismo gradiente que el de Go (%v); se calcula con el motor go", cfg.Motor.Nombre(), err)
			cfg.Motor.Cerrar()
			cfg.Motor = cfg.Respaldo
			return nil
		}
	}
	cfg.validado = true
	log.Printf("motor %s validado contra el de Go: mismo gradiente (tolerancia relativa 1e-8)", cfg.Motor.Nombre())
	return nil
}

// huellaArchivo es la huella de la muestra en disco, o "" si no hay.
func huellaArchivo(ruta string) string {
	h, err := Huella(ruta)
	if err != nil {
		return ""
	}
	return h
}

// asegurarMuestra relee la muestra si el archivo cambió desde que se
// cargó (o si todavía no se había cargado).
func (cfg *ConfigTrabajador) asegurarMuestra() error {
	if cfg.RutaMuestra == "" {
		return nil
	}
	h := huellaArchivo(cfg.RutaMuestra)
	if h == cfg.Huella && cfg.Datos != nil {
		return nil
	}
	cfg.Datos, cfg.Huella = nil, h
	if h == "" {
		return fmt.Errorf("no hay muestra en %s: realiza la limpieza", cfg.RutaMuestra)
	}
	datos, h, err := PrepararLocal(cfg.RutaMuestra, cfg.MesCorte)
	if err != nil {
		cfg.Huella = ""
		return fmt.Errorf("no se pudo cargar %s: %w", cfg.RutaMuestra, err)
	}
	cfg.Datos, cfg.Huella = datos, h
	log.Printf("muestra %s cargada, huella %s", cfg.RutaMuestra, Corta(h))
	return nil
}

// limpiezaNodo es la limpieza que corre este nodo en segundo plano: la
// goroutine principal sigue leyendo mensajes (para poder cancelarla).
type limpiezaNodo struct {
	mu     sync.Mutex
	id     int
	cancel context.CancelFunc
}

// cancelar detiene la limpieza id (0 = cualquiera).
func (l *limpiezaNodo) cancelar(id int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cancel != nil && (id == 0 || id == l.id) {
		l.cancel()
	}
}

func (l *limpiezaNodo) iniciar(c *protocolo.Conexion, cfg *ConfigTrabajador, m protocolo.Mensaje) {
	terminada := func(tec, motivo string) {
		c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoLimpiezaTerminada, WorkerID: cfg.ID, Corrida: m.Corrida, Tecnologia: tec, Motivo: motivo})
	}
	if cfg.Limpieza == nil {
		for _, t := range m.Tecnologias {
			terminada(t, "este nodo no tiene configurada la limpieza")
		}
		return
	}
	l.mu.Lock()
	if l.cancel != nil {
		l.mu.Unlock()
		for _, t := range m.Tecnologias {
			terminada(t, "este nodo ya está limpiando")
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.id, l.cancel = m.Corrida, cancel
	l.mu.Unlock()

	go func() {
		defer func() {
			cancel()
			l.mu.Lock()
			l.cancel = nil
			l.mu.Unlock()
		}()
		hilos := m.Hilos
		if hilos <= 0 {
			hilos = runtime.NumCPU()
		}
		for _, tec := range m.Tecnologias {
			if ctx.Err() != nil {
				terminada(tec, LimpiezaCancelada)
				continue
			}
			log.Printf("limpieza %d: empiezo con %s (%d hilos)", m.Corrida, tec, hilos)
			progreso := func(a limpieza.Avance) {
				c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoProgresoLimpieza, WorkerID: cfg.ID, Corrida: m.Corrida, Tecnologia: tec, Hilos: hilos, Fase: a.Fase, Avance: a.Avance, CPU: a.CPU, TLimpiezaS: a.TLimpieza.Seconds()})
			}
			progreso(limpieza.Avance{Fase: limpieza.FaseLimpieza})
			res, err := cfg.Limpieza.Ejecutar(ctx, tec, hilos, progreso)
			switch {
			case ctx.Err() != nil:
				log.Printf("limpieza %d: %s cancelada", m.Corrida, tec)
				terminada(tec, LimpiezaCancelada)
			case err != nil:
				log.Printf("limpieza %d: %s falló: %v", m.Corrida, tec, err)
				terminada(tec, err.Error())
			default:
				log.Printf("limpieza %d: %s terminó en %s, huella %s", m.Corrida, tec, res.Duracion.Round(time.Millisecond), Corta(res.Huella))
				c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoLimpiezaTerminada, WorkerID: cfg.ID, Corrida: m.Corrida, Tecnologia: tec,
					Hilos: res.Hilos, TTotalS: res.Duracion.Seconds(), TLimpiezaS: res.Limpieza.Seconds(), TVariablesS: res.Variables.Seconds(), Huella: res.Huella})
			}
		}
	}()
}
