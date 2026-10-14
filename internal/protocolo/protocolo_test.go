package protocolo

import (
	"io"
	"net"
	"testing"
)

func TestIdaYVuelta(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	ca, cb := NuevaConexion(a), NuevaConexion(b)

	enviado := Mensaje{Tipo: TipoGradienteParcial, WorkerID: "w1", Epoca: 7, Gradiente: []float64{0.001, -0.0004}, GradienteSesgo: 0.2, NMuestras: 214300}
	go ca.Enviar(enviado)

	recibido, err := cb.Recibir()
	if err != nil {
		t.Fatal(err)
	}
	if recibido.Tipo != enviado.Tipo || recibido.WorkerID != "w1" || recibido.Epoca != 7 ||
		len(recibido.Gradiente) != 2 || recibido.Gradiente[1] != -0.0004 || recibido.NMuestras != 214300 {
		t.Fatalf("recibido = %+v", recibido)
	}

	a.Close()
	if _, err := cb.Recibir(); err != io.EOF {
		t.Fatalf("tras cerrar se esperaba io.EOF, llegó %v", err)
	}
}

func TestParticionAsignadaSigueElContrato(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	go NuevaConexion(a).Enviar(Mensaje{Tipo: TipoParticionAsignada, WorkerID: "w1", FiltroMongo: &FiltroMongo{Mod: 4, Resto: 0}})

	buf := make([]byte, 256)
	n, _ := b.Read(buf)
	esperado := `{"tipo":"particion_asignada","worker_id":"w1","filtro_mongo":{"estacion_id_hash_mod":4,"estacion_id_hash_resto":0}}` + "\n"
	if string(buf[:n]) != esperado {
		t.Fatalf("JSON = %s", buf[:n])
	}
}
