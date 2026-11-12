package distribuido

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/pares"
	"proyecto9/internal/red"
)

func TestRedDistribuidaIgualQueSecuencial(t *testing.T) {
	datos := datosSinteticos(t)
	maestro, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 2, Datos: datos, Huella: "h"})
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for _, id := range []string{"a", "b"} {
		lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: id, Datos: datos, Huella: "h", Motor: NuevoMotorGo(2), Respaldo: NuevoMotorGo(3)})
	}
	if err := maestro.EsperarNodos(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	res, err := maestro.Correr(context.Background(), OpcionesCorrida{Epocas: 3, Modelo: "red", Capas: []int{6, 3}, PasosPorEpoca: 4, Verificar: true, Continuar: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Red == nil || len(res.Perdidas) != 3 {
		t.Fatalf("resultado %+v", res)
	}
	if res.Verificacion == nil || res.Verificacion.Diferencia > 1e-9 {
		t.Fatalf("la primera época distribuida no coincide con la secuencial: %+v", res.Verificacion)
	}
	if !(res.Perdidas[2] < res.Perdidas[0]) {
		t.Fatalf("la pérdida no bajó: %v", res.Perdidas)
	}
	if e := maestro.Instantanea().Corrida; len(e.MBEpoca) != 3 || e.MBEpoca[0] <= 0 || e.Parametros != 5*6+6+6*3+3+3+1 {
		t.Fatalf("estado: mb=%v parámetros=%d", e.MBEpoca, e.Parametros)
	}
	// Fase 3: después de entrenar, el maestro propone el reparto según lo
	// medido, y una corrida con ese reparto también coincide con el secuencial.
	inst := maestro.Instantanea()
	if inst.OrigenPesos != "entrenamiento" || len(inst.PesosSugeridos) != 2 {
		t.Fatalf("pesos sugeridos: %v (%s)", inst.PesosSugeridos, inst.OrigenPesos)
	}
	minimo := 1e9
	for _, p := range inst.PesosSugeridos {
		minimo = min(minimo, p)
	}
	if minimo != 1 {
		t.Fatalf("el más lento debe pesar 1: %v", inst.PesosSugeridos)
	}
	res, err = maestro.Correr(context.Background(), OpcionesCorrida{Epocas: 1, Modelo: "red", Capas: []int{6, 3}, PasosPorEpoca: 4, Verificar: true,
		Pesos: map[string]float64{"a": 1, "b": 3}, Continuar: true})
	if err != nil || res.Verificacion.Diferencia > 1e-9 {
		t.Fatalf("reparto proporcional: %v %+v", err, res.Verificacion)
	}
	maestro.Despedir(MotivoSesionTerminada)
	comprobarErrores(t, &wg, errs)
}

func TestRedEnArbolYAnillo(t *testing.T) {
	datos := datosSinteticos(t)
	maestro, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 3, Datos: datos, Huella: "h"})
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for _, id := range []string{"a", "b", "c"} {
		p, err := pares.Escuchar("127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer p.Cerrar()
		lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: id, Datos: datos, Huella: "h", Motor: NuevoMotorGo(2), Respaldo: NuevoMotorGo(2), Pares: p})
	}
	if err := maestro.EsperarNodos(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	var central float64
	for _, com := range []string{pares.Central, pares.Arbol, pares.Anillo} {
		res, err := maestro.Correr(context.Background(), OpcionesCorrida{Epocas: 2, Modelo: "red", Capas: []int{6, 3}, PasosPorEpoca: 4,
			Comunicacion: com, Verificar: true, Continuar: true})
		if err != nil {
			t.Fatalf("%s: %v", com, err)
		}
		if res.Verificacion.Diferencia > 1e-9 {
			t.Fatalf("%s: la primera época no coincide con la secuencial (%g)", com, res.Verificacion.Diferencia)
		}
		final := res.Perdidas[len(res.Perdidas)-1]
		if com == pares.Central {
			central = final
		} else if math.Abs(final-central) > 1e-9 {
			t.Fatalf("%s: pérdida final %v y por el maestro %v", com, final, central)
		}
		if e := maestro.Instantanea().Corrida; e.Comunicacion != com || e.MBEpoca[0] <= 0 {
			t.Fatalf("%s: estado %s, %v MB", com, e.Comunicacion, e.MBEpoca)
		}
		// El modelo final viene del rango 0 y evalúa igual que el del maestro.
		if res.Evaluacion.AUC <= 0.5 {
			t.Fatalf("%s: AUC %v", com, res.Evaluacion.AUC)
		}
	}
	maestro.Despedir(MotivoSesionTerminada)
	comprobarErrores(t, &wg, errs)
}

func TestPrediccionDistribuida(t *testing.T) {
	datos := datosSinteticos(t)
	for k, mu := range []*dataset.Muestra{datos.Entrenamiento, datos.Prueba} {
		mu.Hora = make([]int64, mu.Len())
		for i := range mu.Hora {
			mu.Hora[i] = int64(480000 + 10000*k + i) // horas distintas en cada fila
		}
	}
	type entrega struct {
		e     EstadoPrediccion
		filas []FilaPrediccion
		m     ModeloEntrenado
	}
	llegada := make(chan entrega, 1)
	maestro, dir := levantarMaestro(t, ConfigMaestro{Capacidad: 2, Datos: datos, Huella: "h",
		PrediccionTerminada: func(e EstadoPrediccion, f []FilaPrediccion, m ModeloEntrenado) { llegada <- entrega{e, f, m} }})
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for _, id := range []string{"a", "b"} {
		lanzarTrabajador(t, &wg, errs, dir, &ConfigTrabajador{ID: id, Datos: datos, Huella: "h", Motor: NuevoMotorGo(2), Respaldo: NuevoMotorGo(2)})
	}
	if err := maestro.EsperarNodos(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if err := maestro.IniciarPrediccion(nil); err == nil {
		t.Fatal("sin modelo entrenado no se puede predecir")
	}
	if _, err := maestro.Correr(context.Background(), OpcionesCorrida{Epocas: 2, Modelo: "red", Capas: []int{6, 3}, PasosPorEpoca: 3, Continuar: true}); err != nil {
		t.Fatal(err)
	}
	if err := maestro.IniciarPrediccion(nil); err != nil {
		t.Fatal(err)
	}
	var r entrega
	select {
	case r = <-llegada:
	case <-time.After(10 * time.Second):
		t.Fatal("la predicción no terminó")
	}
	total := datos.Entrenamiento.Len() + datos.Prueba.Len()
	if r.e.Estado != PrediccionTerminada || len(r.filas) != total || r.e.MB <= 0 {
		t.Fatalf("estado %s, %d filas de %d, %.2f MB", r.e.Estado, len(r.filas), total, r.e.MB)
	}
	// Mismas probabilidades que calculando directamente (en float32).
	directa := map[int64]float64{}
	for _, mu := range []*dataset.Muestra{datos.Entrenamiento, datos.Prueba} {
		for i, x := range mu.X {
			directa[mu.Hora[i]] = red.Predecir(r.m.Capas, r.m.Params, x)
		}
	}
	for _, f := range r.filas {
		if math.Abs(f.Prob-directa[f.Hora]) > 1e-6 {
			t.Fatalf("hora %d: %v, directa %v", f.Hora, f.Prob, directa[f.Hora])
		}
	}
	maestro.Despedir(MotivoSesionTerminada)
	comprobarErrores(t, &wg, errs)
}
