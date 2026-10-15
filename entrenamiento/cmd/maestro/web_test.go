package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/distribuido"
	"proyecto9/internal/entrenador"
	"proyecto9/internal/protocolo"
)

func datosDePrueba(t *testing.T) *entrenador.Datos {
	t.Helper()
	azar := rand.New(rand.NewPCG(3, 5))
	m := &dataset.Muestra{}
	for i := 0; i < 2000; i++ {
		x := []float64{azar.NormFloat64(), azar.NormFloat64(), azar.NormFloat64()}
		y := 0.0
		if x[0]+x[1] > 1 {
			y = 1
		}
		m.X = append(m.X, x)
		m.Y = append(m.Y, y)
		m.EstacionID = append(m.EstacionID, fmt.Sprintf("01-001-%04d", i%30))
		m.Mes = append(m.Mes, 1+i%12)
	}
	datos, err := entrenador.PrepararDesde(m, 10)
	if err != nil {
		t.Fatal(err)
	}
	return datos
}

func TestAPIControlaCorridas(t *testing.T) {
	datos := datosDePrueba(t)
	d := nuevoDifusor()
	m := distribuido.NuevoMaestro(distribuido.ConfigMaestro{Capacidad: 4, Datos: datos, Huella: "h", EsperaGradiente: 10 * time.Second, Notificar: d.Publicar})
	oyente, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer oyente.Close()
	go func() {
		for {
			conn, err := oyente.Accept()
			if err != nil {
				return
			}
			go m.Atender(conn)
		}
	}()
	terminados := make(chan error, 2)
	for _, id := range []string{"a", "b"} {
		go func(id string) {
			conn, err := net.Dial("tcp", oyente.Addr().String())
			if err != nil {
				terminados <- err
				return
			}
			defer conn.Close()
			terminados <- distribuido.Trabajar(protocolo.NuevaConexion(conn), &distribuido.ConfigTrabajador{ID: id, Datos: datos, Huella: "h", Motor: distribuido.NuevoMotorGo(1)})
		}(id)
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	if err := m.EsperarNodos(ctx, 2); err != nil {
		t.Fatal(err)
	}
	m.MedirT1(1)

	a := &api{m: m, difusor: d, cfg: configuracion{tasa: 0.1}, base: ctx}
	srv := httptest.NewServer(a.rutas())
	defer srv.Close()

	// Eventos en vivo: el primero llega apenas se conecta.
	resp, err := http.Get(srv.URL + "/api/eventos")
	if err != nil {
		t.Fatal(err)
	}
	linea, _ := bufio.NewReader(resp.Body).ReadString('\n')
	resp.Body.Close()
	if !strings.HasPrefix(linea, "data: {") {
		t.Fatalf("primer evento = %q", linea)
	}

	pedir := func(cuerpo string) int {
		r, err := http.Post(srv.URL+"/api/corridas", "application/json", bytes.NewBufferString(cuerpo))
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r.StatusCode
	}
	if c := pedir(`{"nodos":["a","b"],"epocas":3000,"verificar":false}`); c != http.StatusAccepted {
		t.Fatalf("POST /api/corridas = %d", c)
	}
	if c := pedir(`{"nodos":["a"],"epocas":5}`); c != http.StatusConflict {
		t.Fatalf("una segunda corrida en paralelo debía dar 409, dio %d", c)
	}
	if c := pedir(`{"nodos":[],"epocas":5}`); c != http.StatusBadRequest {
		t.Fatalf("sin nodos debía dar 400, dio %d", c)
	}
	http.Post(srv.URL+"/api/corridas/cancelar", "application/json", nil)

	estado := func() distribuido.Instantanea {
		r, err := http.Get(srv.URL + "/api/estado")
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var i distribuido.Instantanea
		json.NewDecoder(r.Body).Decode(&i)
		return i
	}
	esperar := func(cond func(distribuido.Instantanea) bool) distribuido.Instantanea {
		for ctx.Err() == nil {
			if i := estado(); cond(i) {
				return i
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("tiempo agotado esperando el estado")
		return distribuido.Instantanea{}
	}
	esperar(func(i distribuido.Instantanea) bool { return len(i.Historial) == 1 && !i.Ocupado })

	// Después de cancelar, los mismos nodos entrenan otra corrida.
	if c := pedir(`{"nodos":["b"],"epocas":20,"verificar":true}`); c != http.StatusAccepted {
		t.Fatalf("POST tras cancelar = %d", c)
	}
	i := esperar(func(i distribuido.Instantanea) bool { return len(i.Historial) == 2 && !i.Ocupado })
	h := i.Historial
	if h[0].Estado != distribuido.EstadoCancelado || h[1].Estado != distribuido.EstadoTerminado || !h[1].Verificado || h[1].Diferencia > 1e-9 {
		t.Fatalf("historial = %+v", h)
	}
	if len(i.Sala) != 2 {
		t.Fatalf("la sala debía seguir con 2 nodos: %+v", i.Sala)
	}

	http.Post(srv.URL+"/api/sesion/terminar", "application/json", nil)
	for range 2 {
		if err := <-terminados; err != distribuido.ErrSesionTerminada {
			t.Fatalf("el trabajador terminó con %v; se esperaba ErrSesionTerminada", err)
		}
	}
}
