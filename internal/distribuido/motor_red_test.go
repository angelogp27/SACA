package distribuido

import (
	"context"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/pares"
	"proyecto9/internal/red"
)

// Los motores en C se prueban si están compilados en bin/ (scripts/saca.sh);
// el de CUDA, con la emulación (nativo/cuda/emulacion/compilar.sh red).
func exeDePrueba(t *testing.T, nombre string) string {
	t.Helper()
	ruta := "../../bin/" + nombre
	if _, err := os.Stat(ruta); err != nil {
		t.Skipf("falta %s", ruta)
	}
	return ruta
}

// Una corrida con cada motor da las mismas pérdidas que la de siempre.
func TestMotoresDanLoMismo(t *testing.T) {
	omp, cuda := exeDePrueba(t, "red_openmp"), exeDePrueba(t, "red_cuda_emulado")
	datos := datosSinteticos(t)
	maestro, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 2, Datos: datos, Huella: "h"})
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for _, id := range []string{"a", "b"} {
		p, err := pares.Escuchar("127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer p.Cerrar()
		lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: id, Datos: datos, Huella: "h", Motor: NuevoMotorGo(2), Respaldo: NuevoMotorGo(2),
			Pares: p, Motores: &MotoresRed{ExeOpenMP: omp, ExeCUDA: cuda}})
	}
	if err := maestro.EsperarNodos(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	for _, modelo := range []string{"red", "logistica"} {
		var base []float64
		for _, c := range []struct {
			cpu  string
			cuda bool
			mot  string
		}{{"", false, "go"}, {"go", false, "go"}, {"openmp", false, "openmp"}, {"openmp", true, "openmp+cuda"}, {"go", true, "go+cuda"}} {
			res, err := maestro.Correr(context.Background(), OpcionesCorrida{Epocas: 2, Tasa: 0.1, Modelo: modelo, Capas: []int{6, 3}, PasosPorEpoca: 2,
				MotorCPU: c.cpu, CUDA: c.cuda, Verificar: true, Continuar: true, Dinamico: modelo == "red"})
			if err != nil {
				t.Fatalf("%s %+v: %v", modelo, c, err)
			}
			if res.Verificacion.Diferencia > 1e-9 {
				t.Fatalf("%s %+v: difiere del secuencial en %g", modelo, c, res.Verificacion.Diferencia)
			}
			if base == nil {
				base = res.Perdidas
			}
			// Con el reparto dinámico los lotes de la segunda época cambian
			// entre corridas: se compara la primera.
			if math.Abs(res.Perdidas[0]-base[0]) > 1e-9 {
				t.Fatalf("%s %+v: pérdidas %v y %v", modelo, c, res.Perdidas, base)
			}
			e := maestro.Instantanea().Corrida
			if e.Nodos[0].Motor != c.mot {
				t.Fatalf("%s %+v: motor %q", modelo, c, e.Nodos[0].Motor)
			}
			if c.cpu != "" && len(e.Nodos[0].CPU) != 2 {
				t.Fatalf("%+v: CPU %v", c, e.Nodos[0].CPU)
			}
		}
	}
	maestro.Despedir(MotivoSesionTerminada)
	comprobarErrores(t, &wg, errs)
}

// Si la GPU muere a mitad de la corrida, la CPU hace su parte.
func TestGPUQueFalla(t *testing.T) {
	cuda := exeDePrueba(t, "red_cuda_emulado")
	datos := datosSinteticos(t)
	r := &MotoresRed{ExeCUDA: cuda, Hilos: 2}
	defer r.Cerrar()
	h, err := r.armar(datos, MotorCPUGo, true)
	if err != nil || h.gpu == nil {
		t.Fatalf("%v, gpu %v", err, h)
	}
	n := datos.Entrenamiento.Len()
	if n < minimoParaGPU {
		t.Fatalf("solo %d filas", n)
	}
	idx := make([]uint32, n)
	for i := range idx {
		idx[i] = uint32(i)
	}
	if err := h.particion(idx); err != nil {
		t.Fatal(err)
	}
	capas := []int{len(datos.Entrenamiento.X[0]), 4, 1}
	p := red.Inicializar(capas, 1)
	r.procesos[nombreCUDA].cmd.Process.Kill()
	g, l, err := h.gradiente(capas, p, 0, n)
	if err != nil || h.gpu != nil {
		t.Fatalf("%v, gpu %v", err, h.gpu)
	}
	gr, lr := red.Gradiente(capas, p, datos.Entrenamiento.X, datos.Entrenamiento.Y, datos.PesosClase, 1)
	if math.Abs(l-lr) > 1e-9*lr {
		t.Fatalf("pérdida %v y %v", l, lr)
	}
	for j := range g {
		if math.Abs(g[j]-gr[j]) > 1e-9*math.Max(1, math.Abs(gr[j])) {
			t.Fatalf("componente %d: %v y %v", j, g[j], gr[j])
		}
	}
	// La siguiente vez se relanza el proceso.
	if h, err = r.armar(datos, MotorCPUGo, true); err != nil || h.gpu == nil {
		t.Fatalf("no se relanzó: %v", err)
	}
	r.procesos[nombreCUDA].cmd.Process.Kill()
	prob, err := h.predecir(capas, p, idx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range prob {
		if math.Abs(prob[i]-red.Predecir(capas, p, datos.Entrenamiento.X[i])) > 1e-12 {
			t.Fatalf("fila %d", i)
		}
	}
}

// MPI de verdad (Open MPI o MS-MPI, 3 procesos en esta PC): Allreduce en el
// entrenamiento y Gatherv en la predicción.
func TestMPI(t *testing.T) {
	exeDePrueba(t, "puente_mpi")
	prediccionMPISoloWindows = false
	t.Cleanup(func() { prediccionMPISoloWindows = true })
	puente := pares.Puente("../../bin")
	if err := pares.ComprobarMPI([]pares.NodoMPI{{ID: "x", Puente: puente, Puerto: 1}}); err != nil {
		t.Skip(err)
	}
	datos := datosSinteticos(t)
	for k, mu := range []*dataset.Muestra{datos.Entrenamiento, datos.Prueba} {
		mu.Hora = make([]int64, mu.Len())
		for i := range mu.Hora {
			mu.Hora[i] = int64(480000 + 10000*k + i)
		}
	}
	type entrega struct {
		e     EstadoPrediccion
		filas []FilaPrediccion
	}
	llegada := make(chan entrega, 1)
	maestro, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 3, Datos: datos, Huella: "h",
		PrediccionTerminada: func(e EstadoPrediccion, f []FilaPrediccion, m ModeloEntrenado) { llegada <- entrega{e, f} }})
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for _, id := range []string{"a", "b", "c"} {
		p, err := pares.Escuchar("127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer p.Cerrar()
		lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: id, Datos: datos, Huella: "h", Motor: NuevoMotorGo(2), Respaldo: NuevoMotorGo(2), Pares: p, PuenteMPI: puente})
	}
	if err := maestro.EsperarNodos(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	var central float64
	for _, com := range []string{pares.Central, pares.MPI} {
		res, err := maestro.Correr(context.Background(), OpcionesCorrida{Epocas: 2, Modelo: "red", Capas: []int{6, 3}, PasosPorEpoca: 4, Comunicacion: com, Verificar: true, Continuar: true})
		if err != nil {
			t.Fatalf("%s: %v", com, err)
		}
		if res.Verificacion.Diferencia > 1e-9 {
			t.Fatalf("%s: diferencia %g", com, res.Verificacion.Diferencia)
		}
		if com == pares.Central {
			central = res.Perdidas[1]
		} else if math.Abs(res.Perdidas[1]-central) > 1e-9 {
			t.Fatalf("MPI %v, central %v", res.Perdidas[1], central)
		}
	}
	if err := maestro.IniciarPrediccion(nil, OpcionesPrediccion{Comunicacion: pares.MPI}); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-llegada:
		if len(r.filas) != datos.Entrenamiento.Len()+datos.Prueba.Len() {
			t.Fatalf("%d filas", len(r.filas))
		}
		for _, n := range r.e.Nodos {
			if n.Filas == 0 || n.Motor != "go" {
				t.Fatalf("nodo %+v", n)
			}
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("la predicción no terminó: %+v", maestro.Instantanea().Prediccion)
	}
	maestro.Despedir(MotivoSesionTerminada)
	comprobarErrores(t, &wg, errs)
}

// Predicción comparando motores: OpenMP y luego Go sobre los mismos nodos;
// el maestro avisa una sola vez, con la primera en Anteriores.
func TestPrediccionComparaMotores(t *testing.T) {
	omp := exeDePrueba(t, "red_openmp")
	datos := datosSinteticos(t)
	for k, mu := range []*dataset.Muestra{datos.Entrenamiento, datos.Prueba} {
		mu.Hora = make([]int64, mu.Len())
		for i := range mu.Hora {
			mu.Hora[i] = int64(480000 + 10000*k + i)
		}
	}
	llegada := make(chan EstadoPrediccion, 2)
	maestro, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 2, Datos: datos, Huella: "h",
		PrediccionTerminada: func(e EstadoPrediccion, f []FilaPrediccion, m ModeloEntrenado) { llegada <- e }})
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for _, id := range []string{"a", "b"} {
		lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: id, Datos: datos, Huella: "h", Motor: NuevoMotorGo(2), Respaldo: NuevoMotorGo(2),
			Motores: &MotoresRed{ExeOpenMP: omp}})
	}
	if err := maestro.EsperarNodos(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if _, err := maestro.Correr(context.Background(), OpcionesCorrida{Epocas: 1, Modelo: "red", Capas: []int{6, 3}, PasosPorEpoca: 2, Continuar: true}); err != nil {
		t.Fatal(err)
	}
	if err := maestro.IniciarPrediccion(nil, OpcionesPrediccion{Comparacion: []string{"openmp", "go"}}); err != nil {
		t.Fatal(err)
	}
	var e EstadoPrediccion
	select {
	case e = <-llegada:
	case <-time.After(15 * time.Second):
		t.Fatal("la predicción no terminó")
	}
	if e.MotorCPU != "go" || len(e.Anteriores) != 1 || e.Anteriores[0].MotorCPU != "openmp" || len(e.Motores) != 2 {
		t.Fatalf("motor %s, motores %v, %d anteriores", e.MotorCPU, e.Motores, len(e.Anteriores))
	}
	if a := e.Anteriores[0]; a.Estado != PrediccionTerminada || a.Filas != e.Filas || a.Nodos[0].Motor != "openmp" {
		t.Fatalf("openmp: %s, %d filas (go %d), motor %s", a.Estado, a.Filas, e.Filas, a.Nodos[0].Motor)
	}
	select {
	case <-llegada:
		t.Fatal("avisó más de una vez")
	case <-time.After(200 * time.Millisecond):
	}
	maestro.Despedir(MotivoSesionTerminada)
	comprobarErrores(t, &wg, errs)
}
