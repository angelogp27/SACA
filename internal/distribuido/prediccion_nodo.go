package distribuido

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"slices"
	"sync"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/entrenador"
	"proyecto9/internal/logistica"
	"proyecto9/internal/particion"
	"proyecto9/internal/protocolo"
	"proyecto9/internal/red"
)

// BytesFilaPrediccion: estación (uint32), hora (int64), probabilidad
// (float32) y NO2 observado (float32).
const BytesFilaPrediccion = 20

// filasPorTrozo: ~1 MB por mensaje (1,3 MB en base64), muy por debajo del
// límite de una línea del protocolo.
const filasPorTrozo = 50_000

// Prediccion es lo que el maestro junta de todos los nodos.
type FilaPrediccion struct {
	Estacion string
	Hora     int64 // horas desde 1970-01-01 UTC
	Prob     float64
	NO2      float64 // ppb observado en esa hora
}

// listaEstaciones: las estaciones de la muestra, ordenadas. Maestro y nodos
// tienen el mismo muestra.bin, así que el índice de una estación en esta
// lista es el mismo en todos y basta con mandar el índice.
func listaEstaciones(d *entrenador.Datos) []string {
	vistas := map[string]bool{}
	for _, m := range []*dataset.Muestra{d.Entrenamiento, d.Prueba} {
		for _, e := range m.EstacionID {
			vistas[e] = true
		}
	}
	lista := make([]string, 0, len(vistas))
	for e := range vistas {
		lista = append(lista, e)
	}
	slices.Sort(lista)
	return lista
}

// modeloPrediccion arma la función de probabilidad: red neuronal (capas) o
// regresión logística (params = pesos y, al final, el sesgo).
func modeloPrediccion(capas []int, params []float64) (func(x []float64) float64, error) {
	if len(capas) > 0 {
		if len(params) != red.NumParametros(capas) {
			return nil, fmt.Errorf("la red %v necesita %d parámetros y llegaron %d", capas, red.NumParametros(capas), len(params))
		}
		return func(x []float64) float64 { return red.Predecir(capas, params, x) }, nil
	}
	if len(params) < 2 {
		return nil, fmt.Errorf("modelo vacío")
	}
	m := &logistica.Modelo{Pesos: params[:len(params)-1], Sesgo: params[len(params)-1]}
	return m.Predecir, nil
}

// predecirNodo: el nodo predice todas las horas (entrenamiento y prueba) de
// las estaciones de su partición, con goroutines, y manda los resultados en
// trozos.
func predecirNodo(c *protocolo.Conexion, cfg *ConfigTrabajador, m protocolo.Mensaje) error {
	if cfg.Datos == nil {
		return fmt.Errorf("este nodo no tiene muestra cargada")
	}
	params, err := red.Decodificar(m.ParamsBin)
	if err != nil {
		return err
	}
	predecir, err := modeloPrediccion(m.Capas, params)
	if err != nil {
		return err
	}
	f := m.FiltroMongo
	if f == nil {
		return fmt.Errorf("predecir sin partición")
	}
	cubetas := map[int]bool{}
	for _, r := range f.Restos {
		cubetas[r] = true
	}
	pertenece := func(est string) bool {
		if len(cubetas) > 0 {
			return cubetas[particion.Asignar(est, f.Mod)]
		}
		return particion.Pertenece(est, f.Mod, f.Resto)
	}
	lista := listaEstaciones(cfg.Datos)
	indice := make(map[string]uint32, len(lista))
	for i, e := range lista {
		indice[e] = uint32(i)
	}
	norm := cfg.Datos.Normalizador

	type fila struct {
		x   []float64
		est uint32
		h   int64
	}
	var filas []fila
	for _, mu := range []*dataset.Muestra{cfg.Datos.Entrenamiento, cfg.Datos.Prueba} {
		if mu.Hora == nil {
			return fmt.Errorf("muestra.bin sin las horas de cada fila: regenérala con la limpieza")
		}
		for i, est := range mu.EstacionID {
			if pertenece(est) {
				filas = append(filas, fila{mu.X[i], indice[est], mu.Hora[i]})
			}
		}
	}

	inicio := time.Now()
	salida := make([]byte, BytesFilaPrediccion*len(filas))
	hilos := runtime.NumCPU()
	var wg sync.WaitGroup
	for h := 0; h < hilos; h++ {
		wg.Add(1)
		go func(h int) {
			defer wg.Done()
			for i := h; i < len(filas); i += hilos {
				fl := filas[i]
				b := salida[BytesFilaPrediccion*i:]
				// NO2 observado: la variable 0, sin normalizar.
				no2 := fl.x[0]*norm.Desviaciones[0] + norm.Medias[0]
				binary.LittleEndian.PutUint32(b, fl.est)
				binary.LittleEndian.PutUint64(b[4:], uint64(fl.h))
				binary.LittleEndian.PutUint32(b[12:], math.Float32bits(float32(predecir(fl.x))))
				binary.LittleEndian.PutUint32(b[16:], math.Float32bits(float32(no2)))
			}
		}(h)
	}
	wg.Wait()
	calc := time.Since(inicio)

	for ini := 0; ini < len(filas) || ini == 0; ini += filasPorTrozo {
		fin := min(ini+filasPorTrozo, len(filas))
		err := c.Enviar(protocolo.Mensaje{
			Tipo: protocolo.TipoPredicciones, WorkerID: cfg.ID, Corrida: m.Corrida, Total: len(filas), NMuestras: fin - ini,
			PrediccionesBin: salida[BytesFilaPrediccion*ini : BytesFilaPrediccion*fin], Ultimo: fin == len(filas), TCalculoMs: ms(calc),
		})
		if err != nil {
			return fmt.Errorf("%w: no se pudieron enviar las predicciones: %v", ErrConexionPerdida, err)
		}
		if len(filas) == 0 {
			break
		}
	}
	return nil
}

// decodificarPredicciones convierte un trozo en filas.
func decodificarPredicciones(b []byte, lista []string) ([]FilaPrediccion, error) {
	if len(b)%BytesFilaPrediccion != 0 {
		return nil, fmt.Errorf("trozo de predicciones de largo inválido")
	}
	filas := make([]FilaPrediccion, len(b)/BytesFilaPrediccion)
	for i := range filas {
		r := b[BytesFilaPrediccion*i:]
		est := binary.LittleEndian.Uint32(r)
		if int(est) >= len(lista) {
			return nil, fmt.Errorf("estación %d fuera de la lista", est)
		}
		filas[i] = FilaPrediccion{
			Estacion: lista[est],
			Hora:     int64(binary.LittleEndian.Uint64(r[4:])),
			Prob:     float64(math.Float32frombits(binary.LittleEndian.Uint32(r[12:]))),
			NO2:      float64(math.Float32frombits(binary.LittleEndian.Uint32(r[16:]))),
		}
	}
	return filas, nil
}
