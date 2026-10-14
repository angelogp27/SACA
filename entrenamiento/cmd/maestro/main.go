// Maestro del clúster de entrenamiento (Parte B, semana 7). Escucha en un
// puerto TCP, atiende cada conexión en su propia goroutine, registra a los
// nodos trabajadores y le asigna a cada uno una partición disjunta de las
// estaciones (docs/contrato_protocolo_tcp.md, pasos 1, 2 y 7). Si un nodo
// envía "baja" o se desconecta, lo quita del registro y libera su
// partición para el siguiente que llegue.
//
// La difusión de parámetros y la agregación de gradientes (pasos 3 a 6)
// se agregan en la semana 9, sobre este mismo registro.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"

	"proyecto9/internal/cluster"
	"proyecto9/internal/protocolo"
)

func main() {
	direccion := flag.String("escuchar", ":9000", "dirección TCP donde escucha el maestro")
	nodos := flag.Int("nodos", 2, "número de nodos trabajadores esperados (una partición por nodo)")
	flag.Parse()

	if *nodos < 1 {
		log.Fatal("-nodos debe ser >= 1")
	}

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancelar()

	oyente, err := net.Listen("tcp", *direccion)
	if err != nil {
		log.Fatalf("no se pudo escuchar en %s: %v", *direccion, err)
	}
	log.Printf("maestro escuchando en %s, esperando %d nodos", oyente.Addr(), *nodos)

	registro := cluster.NuevoRegistro(*nodos)

	// Al recibir Ctrl+C se cierra el oyente (eso desbloquea Accept) y se
	// avisa a todos los nodos con un mensaje "baja".
	go func() {
		<-ctx.Done()
		log.Print("cerrando el maestro")
		oyente.Close()
		for _, n := range registro.Activos() {
			n.Conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, Motivo: "maestro_cerrando"})
		}
	}()

	for {
		conn, err := oyente.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("error aceptando conexión: %v", err)
			continue
		}
		go atender(conn, registro)
	}
}

// atender es la goroutine dedicada a un nodo: espera su registro, le
// asigna partición y luego escucha sus mensajes hasta que se va.
func atender(conn net.Conn, registro *cluster.Registro) {
	defer conn.Close()
	c := protocolo.NuevaConexion(conn)
	remoto := conn.RemoteAddr().String()

	// El primer mensaje tiene que ser "registro", y tiene que llegar pronto.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	m, err := c.Recibir()
	if err != nil || m.Tipo != protocolo.TipoRegistro || m.WorkerID == "" {
		log.Printf("%s: se esperaba un mensaje de registro con worker_id (%v)", remoto, err)
		return
	}
	conn.SetReadDeadline(time.Time{})

	nodo, err := registro.Registrar(m.WorkerID, c)
	if err != nil {
		log.Printf("%s: registro de %s rechazado: %v", remoto, m.WorkerID, err)
		c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, WorkerID: m.WorkerID, Motivo: "registro_rechazado: " + err.Error()})
		return
	}

	err = c.Enviar(protocolo.Mensaje{
		Tipo:        protocolo.TipoParticionAsignada,
		WorkerID:    nodo.ID,
		FiltroMongo: &protocolo.FiltroMongo{Mod: registro.Particiones(), Resto: nodo.Particion},
	})
	if err != nil {
		log.Printf("%s: no se pudo enviar la partición: %v", nodo.ID, err)
		registro.DarDeBaja(nodo.ID)
		return
	}
	log.Printf("nodo %s registrado desde %s → partición %d/%d", nodo.ID, remoto, nodo.Particion, registro.Particiones())
	mostrarEstado(registro)

	motivo := escuchar(c, nodo.ID)
	if _, estaba := registro.DarDeBaja(nodo.ID); estaba {
		log.Printf("nodo %s dado de baja (%s); partición %d libre", nodo.ID, motivo, nodo.Particion)
		mostrarEstado(registro)
	}
}

// escuchar lee mensajes del nodo hasta que se despide o se corta la
// conexión, y devuelve el motivo.
func escuchar(c *protocolo.Conexion, id string) string {
	for {
		m, err := c.Recibir()
		switch {
		case errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed):
			return "desconexion"
		case err != nil:
			return "caída de la conexión: " + err.Error()
		case m.Tipo == protocolo.TipoBaja:
			return "baja: " + m.Motivo
		default:
			log.Printf("nodo %s envió %q (la agregación de gradientes se implementa en la semana 9)", id, m.Tipo)
		}
	}
}

func mostrarEstado(registro *cluster.Registro) {
	activos := registro.Activos()
	ids := make([]string, len(activos))
	for i, n := range activos {
		ids[i] = fmt.Sprintf("%s(p%d)", n.ID, n.Particion)
	}
	estado := "esperando nodos"
	if registro.Completo() {
		estado = "clúster completo, listo para entrenar"
	}
	log.Printf("nodos activos %d/%d [%s] — %s", len(activos), registro.Particiones(), strings.Join(ids, " "), estado)
}
