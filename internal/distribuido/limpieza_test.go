package distribuido

import (
	"errors"
	"net"
	"testing"
	"time"

	"proyecto9/internal/protocolo"
)

// nodoFalso se registra y deja hablar el protocolo a mano desde la prueba.
func nodoFalso(t *testing.T, dir, id, huella string) *protocolo.Conexion {
	t.Helper()
	conn, err := net.Dial("tcp", dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	c := protocolo.NuevaConexion(conn)
	if err := c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoRegistro, WorkerID: id, Huella: huella, Motor: "go"}); err != nil {
		t.Fatal(err)
	}
	return c
}

func esperar(t *testing.T, que string, cond func() bool) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(limite) {
			t.Fatalf("no se cumplió: %s", que)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func recibir(t *testing.T, c *protocolo.Conexion, tipo string) protocolo.Mensaje {
	t.Helper()
	m, err := c.Recibir()
	if err != nil || m.Tipo != tipo {
		t.Fatalf("se esperaba %s, llegó %+v (%v)", tipo, m, err)
	}
	return m
}

func TestLimpiezaGeneraLaMuestraYHabilitaElEntrenamiento(t *testing.T) {
	datos := datosSinteticos(t)
	type cierre struct {
		e       EstadoLimpieza
		guardar bool
	}
	cerradas := make(chan cierre, 4)
	m, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 4, Datos: datos, Huella: "h", AceptarOtraMuestra: true,
		LimpiezaTerminada: func(e EstadoLimpieza, g bool) { cerradas <- cierre{e, g} }})

	// Un nodo sin muestra entra igual a la sala (modo servidor)...
	c := nodoFalso(t, dir, "nuevo", "")
	esperar(t, "nodo en la sala", func() bool { return len(m.Instantanea().Sala) == 1 })
	// ...pero no puede entrenar.
	if _, err := m.Iniciar(OpcionesCorrida{Nodos: []string{"nuevo"}, Epocas: 1}); !errors.Is(err, ErrMuestraDistinta) {
		t.Fatalf("err = %v; se esperaba ErrMuestraDistinta", err)
	}

	if err := m.IniciarLimpieza([]string{"nuevo"}, nil, 0); err != nil {
		t.Fatal(err)
	}
	orden := recibir(t, c, protocolo.TipoIniciarLimpieza)
	if len(orden.Tecnologias) != 2 || orden.Tecnologias[0] != "pthreads" || orden.Tecnologias[1] != "go" {
		t.Fatalf("tecnologías = %v", orden.Tecnologias)
	}
	if _, err := m.Iniciar(OpcionesCorrida{Nodos: []string{"nuevo"}, Epocas: 1}); !errors.Is(err, ErrLimpiando) {
		t.Fatalf("durante la limpieza err = %v; se esperaba ErrLimpiando", err)
	}
	if e := m.Instantanea(); e.Sala[0].Estado != "limpiando" {
		t.Fatalf("estado del nodo = %s", e.Sala[0].Estado)
	}

	for _, tec := range orden.Tecnologias {
		for _, av := range []float64{0, 0.5, 1} {
			c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoProgresoLimpieza, Corrida: orden.Corrida, Tecnologia: tec, Hilos: 4, Avance: av, CPU: 80})
		}
		c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoLimpiezaTerminada, Corrida: orden.Corrida, Tecnologia: tec, Hilos: 4, TTotalS: 1.5, Huella: "h"})
	}
	var fin cierre
	select {
	case fin = <-cerradas:
	case <-time.After(5 * time.Second):
		t.Fatal("la limpieza no terminó")
	}
	if !fin.guardar || fin.e.Estado != LimpiezaTerminada {
		t.Fatalf("cierre = %+v", fin)
	}
	tec := fin.e.Nodos[0].Tecnologias[1]
	if tec.Estado != LimpiezaTerminada || tec.TTotalS != 1.5 || len(tec.Serie) != 3 || tec.Serie[1] != [2]float64{0.5, 80} {
		t.Fatalf("tecnología = %+v", tec)
	}
	if e := m.Instantanea(); e.Sala[0].Huella != "h" || e.Sala[0].Estado != "libre" {
		t.Fatalf("sala = %+v", e.Sala[0])
	}
	// Con la misma huella que el maestro, ya puede entrenar.
	corrida, err := m.Iniciar(OpcionesCorrida{Nodos: []string{"nuevo"}, Epocas: 1})
	if err != nil {
		t.Fatal(err)
	}
	if p := recibirParticion(t, c, corrida); p.Huella != "h" {
		t.Fatalf("la partición debe llevar la huella de la corrida, lleva %q", p.Huella)
	}
}

// recibirParticion lanza la corrida y devuelve la particion_asignada que
// le llega al nodo; luego corta (la corrida aborta, no importa aquí).
func recibirParticion(t *testing.T, c *protocolo.Conexion, corrida *Corrida) protocolo.Mensaje {
	t.Helper()
	go corrida.Ejecutar(t.Context())
	return recibir(t, c, protocolo.TipoParticionAsignada)
}

func TestPararLimpiezaYNodoQueLlegaTarde(t *testing.T) {
	datos := datosSinteticos(t)
	cerradas := make(chan bool, 4)
	m, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 4, Datos: datos, Huella: "h", AceptarOtraMuestra: true,
		LimpiezaTerminada: func(e EstadoLimpieza, g bool) { cerradas <- g }})
	a := nodoFalso(t, dir, "a", "")
	b := nodoFalso(t, dir, "b", "")
	esperar(t, "dos nodos", func() bool { return len(m.Instantanea().Sala) == 2 })

	if err := m.IniciarLimpieza([]string{"a"}, []string{"go"}, 2); err != nil {
		t.Fatal(err)
	}
	ordenA := recibir(t, a, protocolo.TipoIniciarLimpieza)
	// b llega tarde: se suma a la misma limpieza, con sus tecnologías.
	if err := m.IniciarLimpieza([]string{"b"}, nil, 0); err != nil {
		t.Fatal(err)
	}
	ordenB := recibir(t, b, protocolo.TipoIniciarLimpieza)
	if ordenB.Corrida != ordenA.Corrida || len(ordenB.Tecnologias) != 1 || ordenB.Hilos != 2 {
		t.Fatalf("orden de b = %+v", ordenB)
	}
	if err := m.IniciarLimpieza([]string{"a"}, nil, 0); err == nil {
		t.Fatal("a ya estaba en la limpieza")
	}

	// a termina; b no: se para la limpieza sin guardar.
	a.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoLimpiezaTerminada, Corrida: ordenA.Corrida, Tecnologia: "go", TTotalS: 2, Huella: "h"})
	esperar(t, "a terminó", func() bool { return m.Instantanea().Limpieza.Nodos[0].Tecnologias[0].Estado == LimpiezaTerminada })
	if err := m.CancelarLimpieza(false); err != nil {
		t.Fatal(err)
	}
	recibir(t, b, protocolo.TipoCancelarLimpieza)
	if g := <-cerradas; g {
		t.Fatal("se pidió no guardar")
	}
	e := m.Instantanea().Limpieza
	if e.Estado != LimpiezaCancelada || e.Nodos[0].Tecnologias[0].Estado != LimpiezaTerminada || e.Nodos[1].Tecnologias[0].Estado != LimpiezaCancelada {
		t.Fatalf("limpieza = %+v", e)
	}
	if err := m.CancelarLimpieza(true); err == nil {
		t.Fatal("no había limpieza en curso")
	}
}

func TestVerificarHuellas(t *testing.T) {
	datos := datosSinteticos(t)
	m, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 4, Datos: datos, Huella: "h", AceptarOtraMuestra: true})
	c := nodoFalso(t, dir, "n", "vieja")
	esperar(t, "nodo", func() bool { return len(m.Instantanea().Sala) == 1 })
	listo := make(chan error, 1)
	go func() { listo <- m.VerificarHuellas(t.Context(), 3*time.Second) }()
	recibir(t, c, protocolo.TipoConsultarHuella)
	c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoHuellaMuestra, Huella: "h"})
	if err := <-listo; err != nil {
		t.Fatal(err)
	}
	if h := m.Instantanea().Sala[0].Huella; h != "h" {
		t.Fatalf("huella = %q", h)
	}
}
