// Nodo trabajador del clúster de entrenamiento (Parte B). Se conecta al
// maestro (reintentando si todavía no está arriba), se registra con la
// huella de su muestra y su motor de cálculo, recibe su partición y
// calcula un gradiente parcial por cada época que le pide el maestro
// (docs/contrato_protocolo_tcp.md, pasos 1 a 7).
//
// Los datos salen de data/muestra.gob, copiada en cada PC: el trabajador
// no necesita MongoDB. El motor de cálculo puede ser:
//   - go:   goroutines en la CPU (cualquier PC);
//   - cuda: nativo/cuda/gradiente_servidor.cu en una GPU NVIDIA, validado
//     contra el de Go antes de la primera época;
//   - auto: cuda si existe el ejecutable compilado y funciona; si no, go.
//
// Con un maestro en modo servidor, el nodo queda en la sala entre corridas
// hasta que la página lo despide o se cierra con Ctrl+C (que manda una
// "baja"). Con -reconectar, si se pierde la conexión (internet, el maestro
// se reinició) vuelve a intentar y reaparece solo en la sala.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"proyecto9/internal/distribuido"
	"proyecto9/internal/protocolo"
)

func main() {
	maestro := flag.String("maestro", "localhost:9000", "dirección TCP del maestro (IP:puerto; con Tailscale, la IP 100.x.y.z del maestro)")
	nombre, _ := os.Hostname()
	id := flag.String("id", fmt.Sprintf("%s-%d", nombre, os.Getpid()), "worker_id con el que se registra este nodo")
	rutaMuestra := flag.String("muestra", "data/muestra.gob", "muestra de entrenamiento (la misma que la del maestro)")
	mesCorte := flag.Int("mes-corte", 10, "los meses >= mes-corte son de prueba (igual que en el maestro)")
	nombreMotor := flag.String("motor", "auto", "motor de cálculo: auto, go o cuda")
	cudaExe := flag.String("cuda-exe", ejecutableCUDA(), "ejecutable compilado de nativo/cuda/gradiente_servidor.cu")
	hilos := flag.Int("hilos", 0, "goroutines del motor go (0 = todos los núcleos lógicos)")
	reconectar := flag.Bool("reconectar", false, "si se pierde la conexión con el maestro, volver a intentar en vez de terminar")
	flag.Parse()

	datos, huella, err := distribuido.PrepararLocal(*rutaMuestra, *mesCorte)
	if err != nil {
		log.Fatalf("no se pudo preparar la muestra %s: %v (descárgala del Drive del grupo: paso 'Copiar la muestra' de tu guía en docs/cluster/)", *rutaMuestra, err)
	}
	log.Printf("muestra %s cargada, huella %s", *rutaMuestra, distribuido.Corta(huella))

	respaldo := distribuido.NuevoMotorGo(*hilos)
	motor, err := elegirMotor(*nombreMotor, *cudaExe, respaldo)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancelar()

	cfg := &distribuido.ConfigTrabajador{ID: *id, Datos: datos, Huella: huella, Motor: motor, Respaldo: respaldo}
	err = sesion(ctx, *maestro, cfg, *reconectar)
	cfg.Motor.Cerrar()
	switch {
	case ctx.Err() != nil:
		log.Print("nodo cerrado")
	case errors.Is(err, distribuido.ErrSesionTerminada):
		log.Print("el maestro terminó la sesión: nodo cerrado")
	case err != nil:
		log.Fatal(err)
	}
}

// sesion conecta con el maestro y trabaja. Con reconectar, una conexión
// perdida (o un nombre todavía registrado de la conexión anterior) no es
// el final: espera un momento y vuelve a la sala.
func sesion(ctx context.Context, maestro string, cfg *distribuido.ConfigTrabajador, reconectar bool) error {
	for {
		conn, err := conectar(ctx, maestro)
		if err != nil {
			return err
		}
		c := protocolo.NuevaConexion(conn)

		// Ctrl+C: despedirse con "baja" y cerrar, lo que también desbloquea
		// la lectura de Trabajar.
		listo := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, WorkerID: cfg.ID, Motivo: "cierre_normal"})
				conn.Close()
			case <-listo:
			}
		}()
		err = distribuido.Trabajar(c, cfg)
		close(listo)
		conn.Close()

		switch {
		case ctx.Err() != nil:
			return nil
		case err == nil, !reconectar, errors.Is(err, distribuido.ErrSesionTerminada):
			return err
		case errors.Is(err, distribuido.ErrRechazado) && !strings.Contains(err.Error(), "worker_id"):
			return err // otra muestra o sala llena: reintentar no lo arregla
		}
		log.Printf("%v; vuelvo a intentar en 3 s (Ctrl+C para salir)", err)
		select {
		case <-time.After(3 * time.Second):
		case <-ctx.Done():
			return nil
		}
	}
}

// elegirMotor arranca el motor pedido. Con "auto", un motor CUDA que no
// existe o no arranca no es un error: el nodo calcula con goroutines.
func elegirMotor(nombre, cudaExe string, respaldo *distribuido.MotorGo) (distribuido.Motor, error) {
	switch nombre {
	case "go":
		return respaldo, nil
	case "cuda", "auto":
		if _, err := os.Stat(cudaExe); err != nil {
			if nombre == "cuda" {
				return nil, fmt.Errorf("no se encontró %s: compílalo con nvcc (ver nativo/cuda/README.md)", cudaExe)
			}
			log.Printf("motor auto: no hay %s, se calcula con goroutines", cudaExe)
			return respaldo, nil
		}
		motor, err := distribuido.IniciarExterno("cuda", cudaExe)
		if err != nil {
			if nombre == "cuda" {
				return nil, err
			}
			log.Printf("motor auto: %v; se calcula con goroutines", err)
			return respaldo, nil
		}
		log.Printf("motor cuda iniciado: %s", motor.Dispositivo())
		return motor, nil
	}
	return nil, fmt.Errorf("motor desconocido %q (usa auto, go o cuda)", nombre)
}

func ejecutableCUDA() string {
	if runtime.GOOS == "windows" {
		return "bin/gradiente_cuda.exe"
	}
	return "bin/gradiente_cuda"
}

// conectar reintenta hasta que el maestro acepte la conexión: los nodos
// pueden arrancar antes que el maestro.
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
