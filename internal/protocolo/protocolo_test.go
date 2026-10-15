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

func TestCamposDelEntrenamientoDistribuido(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	ca, cb := NuevaConexion(a), NuevaConexion(b)

	go func() {
		ca.Enviar(Mensaje{Tipo: TipoRegistro, WorkerID: "gpu1", Huella: "abc", Motor: "cuda", Dispositivo: "RTX 3060"})
		ca.Enviar(Mensaje{Tipo: TipoGradienteParcial, WorkerID: "gpu1", Epoca: 3, Gradiente: []float64{1, 2}, Perdida: 0.5, TCalculoMs: 1.25, Motor: "cuda"})
	}()

	r, err := cb.Recibir()
	if err != nil || r.Huella != "abc" || r.Motor != "cuda" || r.Dispositivo != "RTX 3060" {
		t.Fatalf("registro = %+v (%v)", r, err)
	}
	g, err := cb.Recibir()
	if err != nil || g.Perdida != 0.5 || g.TCalculoMs != 1.25 || g.Motor != "cuda" {
		t.Fatalf("gradiente_parcial = %+v (%v)", g, err)
	}
}
