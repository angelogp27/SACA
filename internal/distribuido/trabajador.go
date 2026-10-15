package distribuido

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"strings"
	"time"

	"proyecto9/internal/entrenador"
	"proyecto9/internal/protocolo"
)

// ConfigTrabajador son los datos de un nodo trabajador. Motor es con qué
// calcula (MotorGo o un MotorExterno). Si no es Respaldo, se valida contra
// él antes de usarlo y, si no coincide, el nodo pasa a calcular con
// Respaldo: Trabajar actualiza Motor en ese caso, y quien llama lo cierra
// al final.
type ConfigTrabajador struct {
	ID       string
	Datos    *entrenador.Datos
	Huella   string
	Motor    Motor
	Respaldo *MotorGo

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
	err := c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoRegistro, WorkerID: cfg.ID, Huella: cfg.Huella, Motor: cfg.Motor.Nombre(), Dispositivo: cfg.Motor.Dispositivo()})
	if err != nil {
		return fmt.Errorf("%w: no se pudo enviar el registro: %v", ErrConexionPerdida, err)
	}
	log.Printf("conectado al maestro como %s; esperando que inicie una corrida", cfg.ID)

	entrenamiento := cfg.Datos.Entrenamiento
	d := len(entrenamiento.X[0])
	var X [][]float64
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

		case m.Tipo == protocolo.TipoParticionAsignada:
			if m.FiltroMongo == nil {
				return fmt.Errorf("particion_asignada sin filtro")
			}
			mod, resto := m.FiltroMongo.Mod, m.FiltroMongo.Resto
			var y []float64
			X, y = Filtrar(entrenamiento, mod, resto)
			log.Printf("registrado como %s: partición %d de %d → %d de %d ejemplos de entrenamiento (%.1f%%)",
				cfg.ID, resto, mod, len(X), entrenamiento.Len(), 100*float64(len(X))/float64(entrenamiento.Len()))
			if err := cfg.cargar(d, X, y); err != nil {
				return err
			}
			cargado, tCalculo, epocas = true, 0, 0
			log.Printf("listo: calcula con el motor %s (%s); esperando los parámetros del maestro", cfg.Motor.Nombre(), cfg.Motor.Dispositivo())

		case m.Tipo == protocolo.TipoEntrenamientoFinalizado:
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
