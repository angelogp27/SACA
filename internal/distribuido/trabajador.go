package distribuido

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"time"

	"proyecto9/internal/entrenador"
	"proyecto9/internal/protocolo"
)

// ConfigTrabajador son los datos de un nodo trabajador. Motor es con qué
// calcula (MotorGo o un MotorExterno). Si no es Respaldo, se valida contra
// él antes de la primera época y, si no coincide, se calcula con Respaldo.
type ConfigTrabajador struct {
	ID       string
	Datos    *entrenador.Datos
	Huella   string
	Motor    Motor
	Respaldo *MotorGo
}

// ErrRechazado indica que el maestro no aceptó al nodo.
var ErrRechazado = errors.New("el maestro rechazó el registro")

// Trabajar corre el lado del trabajador del protocolo sobre una conexión
// ya abierta: registro, carga de la partición y un gradiente parcial por
// cada mensaje "parametros", hasta "entrenamiento_finalizado" o "baja".
func Trabajar(c *protocolo.Conexion, cfg ConfigTrabajador) error {
	motor := cfg.Motor
	err := c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoRegistro, WorkerID: cfg.ID, Huella: cfg.Huella, Motor: motor.Nombre(), Dispositivo: motor.Dispositivo()})
	if err != nil {
		return fmt.Errorf("no se pudo enviar el registro: %w", err)
	}
	m, err := c.Recibir()
	if err != nil {
		return fmt.Errorf("el maestro cerró la conexión antes de asignar partición: %w", err)
	}
	if m.Tipo == protocolo.TipoBaja {
		return fmt.Errorf("%w: %s", ErrRechazado, m.Motivo)
	}
	if m.Tipo != protocolo.TipoParticionAsignada || m.FiltroMongo == nil {
		return fmt.Errorf("se esperaba particion_asignada, llegó %q", m.Tipo)
	}
	mod, resto := m.FiltroMongo.Mod, m.FiltroMongo.Resto
	entrenamiento := cfg.Datos.Entrenamiento
	X, y := Filtrar(entrenamiento, mod, resto)
	d := len(entrenamiento.X[0])
	log.Printf("registrado como %s: partición %d de %d → %d de %d ejemplos de entrenamiento (%.1f%%)",
		cfg.ID, resto, mod, len(X), entrenamiento.Len(), 100*float64(len(X))/float64(entrenamiento.Len()))

	motor, err = prepararMotor(motor, cfg.Respaldo, d, X, y, cfg)
	if err != nil {
		return err
	}
	defer motor.Cerrar()
	log.Printf("listo: calcula con el motor %s (%s); esperando los parámetros del maestro", motor.Nombre(), motor.Dispositivo())

	var tCalculo time.Duration
	epocas := 0
	for {
		m, err := c.Recibir()
		switch {
		case errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed):
			return fmt.Errorf("el maestro cerró la conexión")
		case err != nil:
			return fmt.Errorf("error leyendo del maestro: %w", err)
		case m.Tipo == protocolo.TipoBaja:
			return fmt.Errorf("el maestro pidió la baja: %s", m.Motivo)
		case m.Tipo == protocolo.TipoEntrenamientoFinalizado:
			promedio := 0.0
			if epocas > 0 {
				promedio = ms(tCalculo) / float64(epocas)
			}
			log.Printf("entrenamiento finalizado: %d épocas, pérdida final %.6f; este nodo calculó %.2f ms por época en promedio", m.EpocaFinal, m.PerdidaFinal, promedio)
			return nil
		case m.Tipo != protocolo.TipoParametros:
			log.Printf("mensaje %q inesperado, se ignora", m.Tipo)
			continue
		}
		if len(m.Pesos) != d {
			return fmt.Errorf("época %d: llegaron %d pesos, se esperaban %d", m.Epoca, len(m.Pesos), d)
		}

		inicio := time.Now()
		grad, gradSesgo, perdida, err := motor.Gradiente(m.Pesos, m.Sesgo)
		if err != nil {
			return fmt.Errorf("época %d: %w", m.Epoca, err)
		}
		calc := time.Since(inicio)
		tCalculo += calc
		epocas++
		err = c.Enviar(protocolo.Mensaje{
			Tipo: protocolo.TipoGradienteParcial, WorkerID: cfg.ID, Epoca: m.Epoca,
			Gradiente: grad, GradienteSesgo: gradSesgo, Perdida: perdida, NMuestras: len(X),
			TCalculoMs: ms(calc), Motor: motor.Nombre(),
		})
		if err != nil {
			return fmt.Errorf("época %d: no se pudo enviar el gradiente: %w", m.Epoca, err)
		}
		if m.Epoca%50 == 0 {
			log.Printf("época %d: gradiente enviado (%.2f ms de cálculo)", m.Epoca, ms(calc))
		}
	}
}

// prepararMotor carga la partición en el motor y, si no es el de
// respaldo, lo valida contra él en dos puntos (pesos en cero y pesos al
// azar). Si la carga o la validación fallan, sigue con el de respaldo: la
// demo no se cae por un motor externo con problemas, y queda el aviso en
// el log.
func prepararMotor(motor Motor, respaldo *MotorGo, d int, X [][]float64, y []float64, cfg ConfigTrabajador) (Motor, error) {
	pc := cfg.Datos.PesosClase
	esRespaldo := respaldo == nil || motor == Motor(respaldo)
	if err := motor.Cargar(d, X, y, pc); err != nil {
		if esRespaldo {
			return nil, err
		}
		log.Printf("AVISO: %v; se calcula con el motor go", err)
		motor.Cerrar()
		return respaldo, respaldo.Cargar(d, X, y, pc)
	}
	if esRespaldo {
		return motor, nil
	}

	respaldo.Cargar(d, X, y, pc)
	azar := rand.New(rand.NewPCG(1, 2))
	pesos := make([]float64, d)
	for prueba := 0; prueba < 2; prueba++ {
		if prueba == 1 {
			for j := range pesos {
				pesos[j] = azar.NormFloat64()
			}
		}
		if err := Comparar(motor, respaldo, pesos, 0.1, 1e-8); err != nil {
			log.Printf("AVISO: el motor %s no da el mismo gradiente que el de Go (%v); se calcula con el motor go", motor.Nombre(), err)
			motor.Cerrar()
			return respaldo, nil
		}
	}
	log.Printf("motor %s validado contra el de Go: mismo gradiente (tolerancia relativa 1e-8)", motor.Nombre())
	return motor, nil
}
