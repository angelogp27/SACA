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
// Al cerrarlo con Ctrl+C se despide con un mensaje "baja".
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
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

	conn, err := conectar(ctx, *maestro)
	if err != nil {
		motor.Cerrar()
		log.Fatal(err)
	}
	defer conn.Close()
	c := protocolo.NuevaConexion(conn)

	// Ctrl+C: despedirse con "baja" y cerrar, lo que también desbloquea la
	// lectura de Trabajar.
	go func() {
		<-ctx.Done()
		c.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoBaja, WorkerID: *id, Motivo: "cierre_normal"})
		conn.Close()
	}()

	err = distribuido.Trabajar(c, distribuido.ConfigTrabajador{ID: *id, Datos: datos, Huella: huella, Motor: motor, Respaldo: respaldo})
	switch {
	case ctx.Err() != nil:
		log.Print("nodo cerrado")
	case err != nil:
		log.Fatal(err)
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
