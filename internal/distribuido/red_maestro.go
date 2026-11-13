package distribuido

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"proyecto9/internal/metricas"
	"proyecto9/internal/protocolo"
	"proyecto9/internal/red"
)

// Valores por defecto del entrenamiento de la red (Fase 2).
const (
	PasosRedPorDefecto  = 25
	EpocasRedPorDefecto = 20
	TasaRedPorDefecto   = 0.003
)

// ResultadoRed es el modelo entrenado cuando la corrida es de la red.
type ResultadoRed struct {
	Capas  []int
	Params []float64
}

// entrenarRed es el entrenamiento síncrono por mini-lotes: en cada paso el
// maestro difunde los parámetros (binario), cada nodo calcula el gradiente
// de su tramo s de la partición, el maestro los suma en orden de nodo y
// actualiza con Adam. Una época son PasosPorEpoca pasos: por la red viajan
// PasosPorEpoca × nodos × 2 vectores de miles de números por época.
func (c *Corrida) entrenarRed(ctx context.Context) (*Resultado, error) {
	m, op := c.m, c.op
	n := len(c.nodos)
	X := c.datos.Entrenamiento.X
	capas := red.Arquitectura(len(X[0]), op.Capas)
	p := red.Inicializar(capas, 1)
	adam := red.NuevoAdam(len(p), op.Tasa)
	pasos := op.PasosPorEpoca
	// Árbol o anillo: los nodos suman los gradientes entre ellos y cada uno
	// actualiza sus parámetros; el maestro solo da la señal de cada paso y
	// recibe la pérdida y los tiempos (y, al final, el modelo del rango 0).
	enPares := comunicacionMsg(op) != ""
	log.Printf("corrida %d: red %v (%d parámetros, %d KB por vector), %d pasos por época, %d épocas, comunicación %s",
		c.activa.id, capas, len(p), 8*len(p)/1024, pasos, op.Epocas, op.Comunicacion)

	res := &Resultado{ID: c.activa.id, Red: &ResultadoRed{Capas: capas, Params: p}, Normalizador: c.datos.Normalizador}
	tCalcNodo := make([]time.Duration, n)
	nNodo := make([]int, n)
	inicio := time.Now()

	for e := 0; e < op.Epocas; e++ {
		inicioEpoca := time.Now()
		var calcEpoca time.Duration
		calcNodoEpoca := make([]float64, n)
		perdidaEpoca, nEpoca, bytes := 0.0, 0, 0
		for s := 0; s < pasos; s++ {
			var params []byte
			if !enPares {
				params = red.Codificar(p)
			}
			for _, nodo := range c.nodos {
				err := nodo.conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoParametros, Corrida: c.activa.id, Epoca: e, Paso: s, Modelo: "red", ParamsBin: params})
				if err != nil {
					return nil, errCaida(e, nodo.vista.ID, "no se le pudieron enviar los parámetros: "+err.Error())
				}
				bytes += len(params)
			}
			grads := make([][]float64, n)
			llego := make([]bool, n)
			perdidas := make([]float64, n)
			cuantos := make([]int, n)
			var calcMax time.Duration
			limite := time.NewTimer(m.cfg.EsperaGradiente)
			for faltan := n; faltan > 0; {
				var ev evento
				select {
				case <-ctx.Done():
					limite.Stop()
					return nil, ctx.Err()
				case <-limite.C:
					return nil, fmt.Errorf("época %d paso %d: %d nodo(s) no enviaron su gradiente en %s", e, s, faltan, m.cfg.EsperaGradiente)
				case ev = <-c.activa.eventos:
				}
				i := c.activa.indice[ev.id]
				if ev.tipo == caida {
					limite.Stop()
					return nil, errCaida(e, ev.id, ev.motivo)
				}
				if llego[i] || ev.msg.Epoca != e || ev.msg.Paso != s {
					continue
				}
				llego[i] = true
				if enPares {
					bytes += int(ev.msg.BytesPares)
					if len(ev.msg.ParamsBin) > 0 { // el modelo final, del rango 0
						if final, err := red.Decodificar(ev.msg.ParamsBin); err == nil && len(final) == len(p) {
							copy(p, final)
						}
					}
				} else {
					g, err := red.Decodificar(ev.msg.GradBin)
					if err != nil || len(g) != len(p) {
						limite.Stop()
						return nil, fmt.Errorf("época %d paso %d: gradiente inválido del nodo %s", e, s, ev.id)
					}
					grads[i] = g
					bytes += len(ev.msg.GradBin)
				}
				perdidas[i], cuantos[i] = ev.msg.Perdida, ev.msg.NMuestras
				calc := time.Duration(ev.msg.TCalculoMs * float64(time.Millisecond))
				calcMax = max(calcMax, calc)
				tCalcNodo[i] += calc
				calcNodoEpoca[i] += ev.msg.TCalculoMs
				faltan--
			}
			limite.Stop()
			// Reducción en el maestro, en orden de nodo (determinista).
			suma := make([]float64, len(p))
			nPaso := 0
			for i := range grads {
				for j, v := range grads[i] {
					suma[j] += v
				}
				perdidaEpoca += perdidas[i]
				nPaso += cuantos[i]
				nNodo[i] += cuantos[i]
			}
			if nPaso > 0 && !enPares {
				adam.Paso(p, suma, nPaso)
			}
			nEpoca += nPaso
			calcEpoca += calcMax
		}
		if nEpoca != len(X) {
			return nil, fmt.Errorf("época %d: los nodos procesaron %d ejemplos y el entrenamiento tiene %d", e, nEpoca, len(X))
		}
		perdida := perdidaEpoca / float64(nEpoca)
		res.Perdidas = append(res.Perdidas, perdida)
		tEpoca := time.Since(inicioEpoca)
		redEsp := max(tEpoca-calcEpoca, 0)
		res.TCalculo += calcEpoca
		res.TComunicacion += redEsp
		mb := float64(bytes) / (1 << 20)
		res.MBRed += mb
		log.Printf("época %3d: pérdida=%.6f | %7.1f ms (cálculo %6.1f ms, red y espera %6.1f ms) | %.1f MB por la red",
			e, perdida, ms(tEpoca), ms(calcEpoca), ms(redEsp), mb)

		m.mu.Lock()
		est := c.estado
		est.Epoca = e + 1
		est.Perdidas = append(est.Perdidas, perdida)
		est.TEpocaMs = append(est.TEpocaMs, ms(tEpoca))
		est.TCalculoMs = append(est.TCalculoMs, ms(calcEpoca))
		est.TRedMs = append(est.TRedMs, ms(redEsp))
		est.MBEpoca = append(est.MBEpoca, mb)
		for i := range est.Nodos {
			est.Nodos[i].CalculoMs = append(est.Nodos[i].CalculoMs, calcNodoEpoca[i])
			est.Nodos[i].Ejemplos = nNodo[i] / (e + 1)
			est.Nodos[i].Motor = "go"
		}
		m.mu.Unlock()
		m.publicar()
	}
	res.Duracion = time.Since(inicio)

	final := res.Perdidas[len(res.Perdidas)-1]
	for _, nodo := range c.nodos {
		nodo.conexion.Enviar(protocolo.Mensaje{Tipo: protocolo.TipoEntrenamientoFinalizado, EpocaFinal: op.Epocas, PerdidaFinal: final, Continuar: op.Continuar})
	}
	m.mu.Lock()
	for _, nodo := range c.nodos {
		delete(m.ocupados, nodo.vista.ID)
	}
	m.mu.Unlock()
	for i, nodo := range c.nodos {
		res.Nodos = append(res.Nodos, InfoNodo{ID: nodo.vista.ID, Motor: "go", Dispositivo: nodo.vista.Dispositivo, Hardware: nodo.vista.Hardware,
			Remoto: nodo.vista.Remoto, Particion: i, N: nNodo[i] / max(op.Epocas, 1), TCalculo: tCalcNodo[i]})
	}
	res.Evaluacion = EvaluarRed(capas, p, c.datos.Prueba.X, c.datos.Prueba.Y)

	if op.Verificar {
		m.mu.Lock()
		c.estado.Estado = EstadoVerificando
		m.mu.Unlock()
		m.publicar()
		v, t1 := c.verificarRed(capas)
		v.Diferencia = math.Abs(res.Perdidas[0] - v.Perdidas[0])
		res.Verificacion, res.T1EpocaMs = v, t1
	}
	c.terminar(EstadoTerminado, "", res)
	return res, nil
}

// verificarRed repite en esta PC, en secuencial (1 hilo), la PRIMERA época
// con exactamente los mismos lotes: cada nodo procesa su partición en orden,
// así que el lote s del clúster es la unión de los tramos s de cada
// partición, y la suma se hace en el mismo orden de nodo. Una época basta
// para comprobar que el resultado distribuido es el mismo, y su duración es
// el T(1) por época para el speedup (una época completa de la red en un
// hilo tarda decenas de segundos; las 20 serían demasiado).
func (c *Corrida) verificarRed(capas []int) (*Verificacion, float64) {
	filtros, _ := c.filtros()
	ent := c.datos.Entrenamiento
	partX := make([][][]float64, len(filtros))
	partY := make([][]float64, len(filtros))
	for i, f := range filtros {
		if len(f.Restos) > 0 {
			partX[i], partY[i] = FiltrarRestos(ent, f.Mod, f.Restos)
		} else {
			partX[i], partY[i] = Filtrar(ent, f.Mod, f.Resto)
		}
	}
	p := red.Inicializar(capas, 1)
	adam := red.NuevoAdam(len(p), c.op.Tasa)
	pc := c.datos.PesosClase
	inicio := time.Now()
	perdida, total := 0.0, 0
	for s := 0; s < c.op.PasosPorEpoca; s++ {
		suma := make([]float64, len(p))
		nPaso := 0
		for i := range partX {
			ini, fin := red.Lote(len(partX[i]), s, c.op.PasosPorEpoca)
			g, l := red.Gradiente(capas, p, partX[i][ini:fin], partY[i][ini:fin], pc, 1)
			for j, v := range g {
				suma[j] += v
			}
			perdida += l
			nPaso += fin - ini
		}
		if nPaso > 0 {
			adam.Paso(p, suma, nPaso)
		}
		total += nPaso
		c.m.mu.Lock()
		c.estado.VerificacionEpoca = (s + 1) * c.op.Epocas / c.op.PasosPorEpoca
		c.m.mu.Unlock()
		c.m.publicar()
	}
	dur := time.Since(inicio)
	log.Printf("verificación de la red: primera época en secuencial (1 hilo) en %s, pérdida %.9f", dur.Round(time.Millisecond), perdida/float64(total))
	return &Verificacion{Perdidas: []float64{perdida / float64(total)}, Duracion: dur}, ms(dur)
}

// EvaluarRed calcula F1 y AUC de la red sobre el conjunto de prueba.
func EvaluarRed(capas []int, p []float64, X [][]float64, y []float64) metricas.Clasificacion {
	prob := make([]float64, len(X))
	const hilos = 8
	hecho := make(chan struct{}, hilos)
	for h := 0; h < hilos; h++ {
		go func(h int) {
			for i := h; i < len(X); i += hilos {
				prob[i] = red.Predecir(capas, p, X[i])
			}
			hecho <- struct{}{}
		}(h)
	}
	for h := 0; h < hilos; h++ {
		<-hecho
	}
	return metricas.Evaluar(y, prob, 0.5)
}

// describirModelo para el log y el CSV.
func describirModelo(op OpcionesCorrida) string {
	if op.Modelo != "red" {
		return "logistica"
	}
	partes := make([]string, len(op.Capas))
	for i, c := range op.Capas {
		partes[i] = fmt.Sprint(c)
	}
	return "red(" + strings.Join(partes, "-") + ")"
}
