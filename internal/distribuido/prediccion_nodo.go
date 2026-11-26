package distribuido

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/entrenador"
	"proyecto9/internal/pares"
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

// capasPrediccion valida el modelo: red neuronal (capas) o regresión
// logística (params = pesos y, al final, el sesgo), que es la red {d, 1}.
func capasPrediccion(capas []int, params []float64, d int) ([]int, error) {
	if len(capas) == 0 {
		capas = []int{d, 1}
	}
	if len(params) != red.NumParametros(capas) {
		return nil, fmt.Errorf("el modelo %v necesita %d parámetros y llegaron %d", capas, red.NumParametros(capas), len(params))
	}
	return capas, nil
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
	capas, err := capasPrediccion(m.Capas, params, len(cfg.Datos.Entrenamiento.X[0]))
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

	// Todas las filas (entrenamiento y, después, prueba) de mis estaciones.
	var idx []uint32
	nEnt := uint32(cfg.Datos.Entrenamiento.Len())
	for k, mu := range []*dataset.Muestra{cfg.Datos.Entrenamiento, cfg.Datos.Prueba} {
		if mu.Hora == nil {
			return fmt.Errorf("muestra.bin sin las horas de cada fila: regenérala con la limpieza")
		}
		for i, est := range mu.EstacionID {
			if pertenece(est) {
				idx = append(idx, uint32(k)*nEnt+uint32(i))
			}
		}
	}
	cpu := m.MotorCPU
	if cpu == "" {
		cpu = MotorCPUGo
	}
	motor, err := cfg.Motores.armar(cfg.Datos, cpu, m.CUDA)
	if err != nil {
		return err
	}
	var grupo *pares.Grupo
	if m.Comunicacion == pares.MPI {
		if cfg.Pares == nil {
			return fmt.Errorf("este nodo no tiene puerto de pares para MPI")
		}
		if grupo, err = cfg.Pares.Unirse(context.Background(), m.Corrida+corridaPrediccion, m.Rango, m.Pares, pares.MPI); err != nil {
			return err
		}
		defer grupo.Cerrar()
	}

	inicio := time.Now()
	prob, err := motor.predecir(capas, params, idx)
	if err != nil {
		return err
	}
	salida := make([]byte, BytesFilaPrediccion*len(idx))
	for i, f := range idx {
		mu, k := cfg.Datos.Entrenamiento, f
		if f >= nEnt {
			mu, k = cfg.Datos.Prueba, f-nEnt
		}
		b := salida[BytesFilaPrediccion*i:]
		// NO2 observado: la variable 0, sin normalizar.
		no2 := mu.X[k][0]*norm.Desviaciones[0] + norm.Medias[0]
		binary.LittleEndian.PutUint32(b, indice[mu.EstacionID[k]])
		binary.LittleEndian.PutUint64(b[4:], uint64(mu.Hora[k]))
		binary.LittleEndian.PutUint32(b[12:], math.Float32bits(float32(prob[i])))
		binary.LittleEndian.PutUint32(b[16:], math.Float32bits(float32(no2)))
	}
	calc := time.Since(inicio)
	resumen := resumenPrediccion{CalculoMs: ms(calc), ParteGPU: motor.parteGPU(), Motor: motor.nombre()}

	if grupo == nil {
		return enviarPredicciones(c, cfg.ID, "", m.Corrida, salida, resumen)
	}
	// MPI_Gatherv: cada nodo aporta su resumen y sus filas; el rango 0 le
	// entrega al maestro lo de todos.
	cab, err := json.Marshal(resumen)
	if err != nil {
		return err
	}
	paquete := binary.LittleEndian.AppendUint32(nil, uint32(len(cab)))
	partes, err := grupo.Gatherv(append(append(paquete, cab...), salida...))
	if err != nil {
		return fmt.Errorf("MPI_Gatherv: %w", err)
	}
	for r, p := range partes {
		if len(p) < 4 || len(p) < 4+int(binary.LittleEndian.Uint32(p)) || r >= len(m.Nodos) {
			return fmt.Errorf("MPI_Gatherv: parte %d inválida", r)
		}
		largo := 4 + int(binary.LittleEndian.Uint32(p))
		var res resumenPrediccion
		if err := json.Unmarshal(p[4:largo], &res); err != nil {
			return err
		}
		if err := enviarPredicciones(c, cfg.ID, m.Nodos[r], m.Corrida, p[largo:], res); err != nil {
			return err
		}
	}
	return nil
}

// corridaPrediccion separa los números de predicción de los de corrida en
// el saludo de los puentes MPI.
const corridaPrediccion = 1_000_000

type resumenPrediccion struct {
	CalculoMs float64
	ParteGPU  float64
	Motor     string
}

// enviarPredicciones manda las filas al maestro en trozos; dueno es el nodo
// que las calculó, si no es este.
func enviarPredicciones(c *protocolo.Conexion, id, dueno string, corrida int, salida []byte, r resumenPrediccion) error {
	total := len(salida) / BytesFilaPrediccion
	for ini := 0; ini < total || ini == 0; ini += filasPorTrozo {
		fin := min(ini+filasPorTrozo, total)
		err := c.Enviar(protocolo.Mensaje{
			Tipo: protocolo.TipoPredicciones, WorkerID: id, Nodo: dueno, Corrida: corrida, Total: total, NMuestras: fin - ini,
			PrediccionesBin: salida[BytesFilaPrediccion*ini : BytesFilaPrediccion*fin], Ultimo: fin == total,
			TCalculoMs: r.CalculoMs, ParteGPU: r.ParteGPU, Motor: r.Motor,
		})
		if err != nil {
			return fmt.Errorf("%w: no se pudieron enviar las predicciones: %v", ErrConexionPerdida, err)
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
