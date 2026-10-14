// Nodo trabajador del clúster de entrenamiento (Parte B, semana 7). Se
// conecta al maestro (reintentando si todavía no está arriba), se
// registra, recibe su partición de datos y queda escuchando. Al cerrarlo
// con Ctrl+C se despide con un mensaje "baja".
//
// El cálculo del gradiente parcial sobre su partición
// (logistica.GradienteParcial) se conecta en la semana 9.
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
	"time"

	"proyecto9/internal/protocolo"
)

func main() {
	maestro := flag.String("maestro", "localhost:9000", "dirección TCP del maestro")
	nombre, _ := os.Hostname()
	id := flag.String("id", fmt.Sprintf("%s-%d", nombre, os.Getpid()), "worker_id con el que se registra este nodo")
	flag.Parse()

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancelar()

	conn, err := conectar(ctx, *maestro)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	c := protocolo.NuevaConexion(conn)

	if err := c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoRegistro, WorkerID: *id}); err != nil {
		log.Fatalf("no se pudo enviar el registro: %v", err)
	}

	m, err := c.Recibir()
	if err != nil {
		log.Fatalf("el maestro cerró la conexión antes de asignar partición: %v", err)
	}
	if m.Tipo == protocolo.TipoBaja {
		log.Fatalf("el maestro rechazó el registro: %s", m.Motivo)
	}
	if m.Tipo != protocolo.TipoParticionAsignada || m.FiltroMongo == nil {
		log.Fatalf("se esperaba particion_asignada, llegó %q", m.Tipo)
	}
	log.Printf("registrado como %s: partición %d de %d (estaciones con hash %% %d == %d)",
		*id, m.FiltroMongo.Resto, m.FiltroMongo.Mod, m.FiltroMongo.Mod, m.FiltroMongo.Resto)

	// Ctrl+C: despedirse con "baja" y cerrar, lo que también desbloquea el
	// Recibir de abajo.
	go func() {
		<-ctx.Done()
		c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, WorkerID: *id, Motivo: "cierre_normal"})
		conn.Close()
	}()

	for {
		m, err := c.Recibir()
		switch {
		case ctx.Err() != nil:
			log.Print("nodo cerrado")
			return
		case errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed):
			log.Print("el maestro cerró la conexión")
			return
		case err != nil:
			log.Fatalf("error leyendo del maestro: %v", err)
		case m.Tipo == protocolo.TipoBaja:
			log.Printf("el maestro pidió la baja: %s", m.Motivo)
			return
		case m.Tipo == protocolo.TipoEntrenamientoFinalizado:
			log.Printf("entrenamiento finalizado en la época %d (pérdida %.4f)", m.EpocaFinal, m.PerdidaFinal)
			return
		default:
			log.Printf("mensaje %q recibido (el cálculo del gradiente se conecta en la semana 9)", m.Tipo)
		}
	}
}

// conectar reintenta hasta que el maestro acepte la conexión: en Docker
// Compose los trabajadores pueden arrancar antes que el maestro.
func conectar(ctx context.Context, direccion string) (net.Conn, error) {
	var d net.Dialer
	for intento := 1; ; intento++ {
		conn, err := d.DialContext(ctx, "tcp", direccion)
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if intento%5 == 1 {
			log.Printf("maestro %s no disponible todavía (%v), reintentando...", direccion, err)
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
