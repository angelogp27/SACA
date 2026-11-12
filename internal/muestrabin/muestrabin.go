// Package muestrabin arma la muestra de entrenamiento directamente desde
// los registros limpios del cargador (sin MongoDB ni muestra.gob) y la
// escribe en data/muestra.bin, versión 2 de docs/contratos/contrato_formato_binario.md.
//
// El archivo tiene que salir idéntico byte a byte en cualquier nodo y con
// cualquier número de hilos, porque su SHA-256 (la huella) es lo que el
// maestro compara antes de entrenar. Por eso:
//   - las filas se ordenan por (estación, hora) antes de escribir;
//   - la normalización suma en ese orden, de forma secuencial;
//   - el seno y coseno de hora, día y mes salen de dataset.VectorFeatures,
//     y el cargador de Pthreads usa una tabla con esos mismos valores
//     (nativo/comun/tabla_ciclica.h) en lugar de la libm de C.
package muestrabin

import (
	"bufio"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/entrenador"
	"proyecto9/internal/logistica"
	"proyecto9/internal/registro"
)

const (
	// D es el número de variables (dataset.NombresFeatures).
	D = 20
	// Version del formato que escribe este paquete.
	Version = 2
	// MesCorte: los meses >= MesCorte son de prueba.
	MesCorte = 10
	// HorizonteHoras de la etiqueta.
	HorizonteHoras = 1
	// LargoID es el tamaño fijo de un id de estación en el archivo.
	LargoID = 16
)

// fila es un ejemplo etiquetado antes de normalizar.
type fila struct {
	estacion string
	hora     int64 // horas desde 1970-01-01 UTC
	mes      int
	x        [D]float64
	y        float64
}

// Muestra es el contenido del archivo ya armado y normalizado.
type Muestra struct {
	D            int
	XEnt, XPru   []float64 // por filas
	YEnt, YPru   []float64
	PesosClase   logistica.PesosClase
	Normalizador dataset.Normalizador
	Estaciones   []string // ordenadas
	EstEnt       []uint32 // índice en Estaciones de cada fila
	EstPru       []uint32
	HoraEnt      []int64 // hora de cada fila (horas desde 1970-01-01 UTC)
	HoraPru      []int64
}

// Construir etiqueta los registros de cada índice (uno por agregadora; una
// estación vive siempre en un solo índice, así que la lectura de la hora
// siguiente se busca en el mismo mapa) y arma la muestra. Cada índice se
// etiqueta y se ordena en su propia goroutine; después las partes se
// intercalan por estación y se normaliza en una sola pasada secuencial.
func Construir(indices []map[string]*registro.RegistroProcesado, mesCorte int) (*Muestra, error) {
	return construir(indices, mesCorte, false, nil)
}

// ConstruirLiberando es Construir, pero suelta cada índice (indices[k] =
// nil) apenas lo etiqueta, para que el recolector recupere esa memoria
// mientras se arma el resto: en una laptop de 8 GB es la diferencia entre
// terminar o no. Úsalo cuando los registros ya no hacen falta después.
// alAvanzar (puede ser nil) recibe la fracción de registros ya etiquetados.
func ConstruirLiberando(indices []map[string]*registro.RegistroProcesado, mesCorte int, alAvanzar func(float64)) (*Muestra, error) {
	return construir(indices, mesCorte, true, alAvanzar)
}

func construir(indices []map[string]*registro.RegistroProcesado, mesCorte int, liberar bool, alAvanzar func(float64)) (*Muestra, error) {
	total := 0
	for _, ix := range indices {
		total += len(ix)
	}
	var hechos atomic.Int64
	contar := func(n int) {
		if alAvanzar != nil && total > 0 {
			alAvanzar(float64(hechos.Add(int64(n))) / float64(total))
		}
	}
	partes := make([][]fila, len(indices))
	errs := make([]error, len(indices))
	var wg sync.WaitGroup
	for k := range indices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			partes[k], errs[k] = etiquetar(indices[k], liberar, contar)
			if liberar {
				indices[k] = nil
			}
			slices.SortFunc(partes[k], compararFilas)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return armar(intercalar(partes), mesCorte)
}

func compararFilas(a, b fila) int {
	if c := cmp.Compare(a.estacion, b.estacion); c != 0 {
		return c
	}
	return cmp.Compare(a.hora, b.hora)
}

// intercalar recorre las partes (cada una ya ordenada) en el orden global
// (estación, hora) sin copiarlas a un arreglo nuevo: como una estación está
// entera en una sola parte, basta con tomar, cada vez, el tramo completo de
// la estación menor entre las cabezas. El resultado es el mismo orden que
// ordenar todo junto.
func intercalar(partes [][]fila) iter.Seq[*fila] {
	return func(yield func(*fila) bool) {
		pos := make([]int, len(partes))
		for {
			k := -1
			for j, p := range partes {
				if pos[j] < len(p) && (k < 0 || p[pos[j]].estacion < partes[k][pos[k]].estacion) {
					k = j
				}
			}
			if k < 0 {
				return
			}
			p, est := partes[k], partes[k][pos[k]].estacion
			for pos[k] < len(p) && p[pos[k]].estacion == est {
				if !yield(&p[pos[k]]) {
					return
				}
				pos[k]++
			}
		}
	}
}

// etiquetar arma las filas de un índice. Con liberar, borra cada registro
// del mapa en cuanto lo usa (la etiqueta y los rezagos salen de los arreglos
// de NO2, no del mapa), así el recolector lo recupera de inmediato.
func etiquetar(ix map[string]*registro.RegistroProcesado, liberar bool, contar func(int)) ([]fila, error) {
	// NO2 por estación y hora, para los rezagos: un arreglo por estación con
	// una casilla por hora entre la primera y la última (NaN = sin lectura).
	// Un año son ~8 760 casillas por estación: unos 30 MB en total, frente a
	// los cientos de MB de un mapa con una entrada por lectura.
	type serieNO2 struct {
		desde int64
		v     []float64
	}
	rangos := map[string][2]int64{}
	for _, r := range ix {
		if r.NO2Ppb == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, r.TimestampUTC)
		if err != nil {
			return nil, fmt.Errorf("hora inválida %q en %s", r.TimestampUTC, r.EstacionID)
		}
		h := t.Unix() / 3600
		rg, ok := rangos[r.EstacionID]
		if !ok {
			rg = [2]int64{h, h}
		}
		rangos[r.EstacionID] = [2]int64{min(rg[0], h), max(rg[1], h)}
	}
	no2 := make(map[string]*serieNO2, len(rangos))
	for est, rg := range rangos {
		v := make([]float64, rg[1]-rg[0]+1)
		for i := range v {
			v[i] = math.NaN()
		}
		no2[est] = &serieNO2{desde: rg[0], v: v}
	}
	for _, r := range ix {
		if r.NO2Ppb == nil {
			continue
		}
		t, _ := time.Parse(time.RFC3339, r.TimestampUTC)
		s := no2[r.EstacionID]
		s.v[t.Unix()/3600-s.desde] = *r.NO2Ppb
	}
	// Una hora con lectura de NO2 en la serie de su estación.
	lectura := func(serie *serieNO2, h int64) (float64, bool) {
		i := h - serie.desde
		if i < 0 || i >= int64(len(serie.v)) || math.IsNaN(serie.v[i]) {
			return 0, false
		}
		return serie.v[i], true
	}
	var filas []fila
	vistos := 0
	for clave, r := range ix {
		if liberar {
			delete(ix, clave)
		}
		if vistos++; vistos == 50_000 {
			contar(vistos)
			vistos = 0
		}
		if r.NO2Ppb == nil {
			continue
		}
		t, _ := time.Parse(time.RFC3339, r.TimestampUTC)
		serie := no2[r.EstacionID]
		f := fila{estacion: r.EstacionID, hora: t.Unix() / 3600, mes: int(t.Month())}
		futuro, ok := lectura(serie, f.hora+HorizonteHoras)
		if !ok {
			continue
		}
		historia := func(k int) (float64, bool) { return lectura(serie, f.hora-int64(k)) }
		copy(f.x[:], dataset.VectorCompleto(r, historia))
		if futuro > dataset.UmbralNO2 {
			f.y = 1
		}
		filas = append(filas, f)
	}
	contar(vistos)
	return filas, nil
}

func armar(filas iter.Seq[*fila], mesCorte int) (*Muestra, error) {
	const d = D
	m := &Muestra{D: d}
	// Primero se cuentan, para reservar los arreglos con su tamaño exacto
	// (con append crecerían al doble y por un momento existirían dos copias).
	nEnt, nPru := 0, 0
	for f := range filas {
		if f.mes < mesCorte {
			nEnt++
		} else {
			nPru++
		}
	}
	m.XEnt, m.YEnt, m.EstEnt, m.HoraEnt = make([]float64, 0, d*nEnt), make([]float64, 0, nEnt), make([]uint32, 0, nEnt), make([]int64, 0, nEnt)
	m.XPru, m.YPru, m.EstPru, m.HoraPru = make([]float64, 0, d*nPru), make([]float64, 0, nPru), make([]uint32, 0, nPru), make([]int64, 0, nPru)
	idEstacion := map[string]uint32{}
	for f := range filas {
		if _, ok := idEstacion[f.estacion]; !ok {
			if len(f.estacion) >= LargoID {
				return nil, fmt.Errorf("id de estación demasiado largo: %q", f.estacion)
			}
			idEstacion[f.estacion] = uint32(len(m.Estaciones))
			m.Estaciones = append(m.Estaciones, f.estacion) // ya vienen ordenadas
		}
		est := idEstacion[f.estacion]
		if f.mes < mesCorte {
			m.XEnt = append(m.XEnt, f.x[:]...)
			m.YEnt = append(m.YEnt, f.y)
			m.EstEnt = append(m.EstEnt, est)
			m.HoraEnt = append(m.HoraEnt, f.hora)
		} else {
			m.XPru = append(m.XPru, f.x[:]...)
			m.YPru = append(m.YPru, f.y)
			m.EstPru = append(m.EstPru, est)
			m.HoraPru = append(m.HoraPru, f.hora)
		}
	}
	if len(m.YEnt) == 0 || len(m.YPru) == 0 {
		return nil, fmt.Errorf("la partición temporal dejó un conjunto vacío (entrenamiento=%d, prueba=%d)", len(m.YEnt), len(m.YPru))
	}
	m.Normalizador = Normalizar(m.XEnt, d)
	aplicar(m.XEnt, d, m.Normalizador)
	aplicar(m.XPru, d, m.Normalizador)
	m.PesosClase = logistica.CalcularPesosClase(m.YEnt)
	return m, nil
}

// Normalizar calcula media y desviación de cada columna sumando las filas
// en orden. Las conversiones explícitas a float64 impiden que el
// compilador fusione d*d + acumulado en una FMA (el resultado cambiaría en
// el último bit según la CPU); el cargador de C se compila con
// -ffp-contract=off por lo mismo.
func Normalizar(X []float64, d int) dataset.Normalizador {
	n := len(X) / d
	medias := make([]float64, d)
	for i := 0; i < n; i++ {
		for j := 0; j < d; j++ {
			medias[j] += X[i*d+j]
		}
	}
	for j := range medias {
		medias[j] /= float64(n)
	}
	desv := make([]float64, d)
	for i := 0; i < n; i++ {
		for j := 0; j < d; j++ {
			dif := X[i*d+j] - medias[j]
			desv[j] += float64(dif * dif)
		}
	}
	for j := range desv {
		desv[j] = math.Sqrt(desv[j] / float64(n))
		if desv[j] == 0 {
			desv[j] = 1
		}
	}
	return dataset.Normalizador{Medias: medias, Desviaciones: desv}
}

func aplicar(X []float64, d int, n dataset.Normalizador) {
	for i := range X {
		j := i % d
		X[i] = (X[i] - n.Medias[j]) / n.Desviaciones[j]
	}
}

// Escribir guarda la muestra en ruta (primero en un temporal, para que un
// corte a medias nunca deje un .bin incompleto con el nombre bueno).
func (m *Muestra) Escribir(ruta string) error {
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	tmp := ruta + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	if err := m.escribir(w); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return Reemplazar(tmp, ruta)
}

// Reemplazar mueve tmp a ruta, reemplazándola. En Windows un archivo que
// otro proceso tiene abierto (la página calculando su huella, un antivirus)
// no se puede reemplazar: se reintenta unos segundos antes de rendirse.
func Reemplazar(tmp, ruta string) error {
	var err error
	for intento := 0; intento < 60; intento++ {
		if err = os.Rename(tmp, ruta); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	os.Remove(tmp)
	return fmt.Errorf("%w (¿otro programa tiene abierto %s?)", err, ruta)
}

func (m *Muestra) escribir(w io.Writer) error {
	le := binary.LittleEndian
	enc := make([]byte, 48)
	copy(enc, "SACA")
	le.PutUint32(enc[4:], Version)
	le.PutUint32(enc[8:], 1) // tipo 1 = muestra
	le.PutUint32(enc[12:], uint32(m.D))
	le.PutUint64(enc[16:], uint64(len(m.YEnt)))
	le.PutUint64(enc[24:], uint64(len(m.YPru)))
	le.PutUint64(enc[32:], math.Float64bits(m.PesosClase.Cero))
	le.PutUint64(enc[40:], math.Float64bits(m.PesosClase.Uno))
	if _, err := w.Write(enc); err != nil {
		return err
	}
	for _, bloque := range [][]float64{m.XEnt, m.YEnt, m.XPru, m.YPru, m.Normalizador.Medias, m.Normalizador.Desviaciones} {
		if err := binary.Write(w, le, bloque); err != nil {
			return err
		}
	}
	if err := binary.Write(w, le, uint64(len(m.Estaciones))); err != nil {
		return err
	}
	ids := make([]byte, LargoID*len(m.Estaciones))
	for i, e := range m.Estaciones {
		copy(ids[i*LargoID:], e)
	}
	if _, err := w.Write(ids); err != nil {
		return err
	}
	if err := binary.Write(w, le, m.EstEnt); err != nil {
		return err
	}
	if err := binary.Write(w, le, m.EstPru); err != nil {
		return err
	}
	// Al final, la hora de cada fila (entrenamiento y prueba): los lectores
	// que no la conocen terminan antes y no se enteran.
	if err := binary.Write(w, le, m.HoraEnt); err != nil {
		return err
	}
	return binary.Write(w, le, m.HoraPru)
}

// Leer carga un muestra.bin versión 2 y lo devuelve listo para entrenar
// (ya viene normalizado y con los pesos de clase).
func Leer(ruta string) (*entrenador.Datos, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	le := binary.LittleEndian

	enc := make([]byte, 48)
	if _, err := io.ReadFull(r, enc); err != nil || string(enc[:4]) != "SACA" {
		return nil, fmt.Errorf("%s no es un archivo SACA", ruta)
	}
	if v, tipo := le.Uint32(enc[4:]), le.Uint32(enc[8:]); v != Version || tipo != 1 {
		return nil, fmt.Errorf("%s es una muestra versión %d tipo %d; se necesita la versión %d (genérala con el cargador: -bin)", ruta, v, tipo, Version)
	}
	d := int(le.Uint32(enc[12:]))
	nEnt, nPru := int(le.Uint64(enc[16:])), int(le.Uint64(enc[24:]))
	pc := logistica.PesosClase{Cero: math.Float64frombits(le.Uint64(enc[32:])), Uno: math.Float64frombits(le.Uint64(enc[40:]))}

	leer := func(n int) ([]float64, error) {
		v := make([]float64, n)
		return v, binary.Read(r, le, v)
	}
	xe, err := leer(nEnt * d)
	if err != nil {
		return nil, err
	}
	ye, err := leer(nEnt)
	if err != nil {
		return nil, err
	}
	xp, err := leer(nPru * d)
	if err != nil {
		return nil, err
	}
	yp, err := leer(nPru)
	if err != nil {
		return nil, err
	}
	medias, err := leer(d)
	if err != nil {
		return nil, err
	}
	desv, err := leer(d)
	if err != nil {
		return nil, err
	}
	var nEst uint64
	if err := binary.Read(r, le, &nEst); err != nil {
		return nil, err
	}
	ids := make([]byte, LargoID*int(nEst))
	if _, err := io.ReadFull(r, ids); err != nil {
		return nil, err
	}
	estaciones := make([]string, nEst)
	for i := range estaciones {
		b := ids[i*LargoID : (i+1)*LargoID]
		if k := slices.Index(b, 0); k >= 0 {
			b = b[:k]
		}
		estaciones[i] = string(b)
	}
	indices := func(n int) ([]string, error) {
		v := make([]uint32, n)
		if err := binary.Read(r, le, v); err != nil {
			return nil, err
		}
		s := make([]string, n)
		for i, k := range v {
			if int(k) >= len(estaciones) {
				return nil, fmt.Errorf("índice de estación fuera de rango en %s", ruta)
			}
			s[i] = estaciones[k]
		}
		return s, nil
	}
	estEnt, err := indices(nEnt)
	if err != nil {
		return nil, err
	}
	estPru, err := indices(nPru)
	if err != nil {
		return nil, err
	}

	// Horas de cada fila (archivos generados desde la Fase 5; si faltan, nil).
	horas := func(n int) []int64 {
		v := make([]int64, n)
		if binary.Read(r, le, v) != nil {
			return nil
		}
		return v
	}
	horaEnt := horas(nEnt)
	horaPru := horas(nPru)
	if horaEnt == nil || horaPru == nil {
		horaEnt, horaPru = nil, nil
	}

	filas := func(X []float64, n int) [][]float64 {
		out := make([][]float64, n)
		for i := range out {
			out[i] = X[i*d : (i+1)*d : (i+1)*d]
		}
		return out
	}
	return &entrenador.Datos{
		Entrenamiento: &dataset.Muestra{X: filas(xe, nEnt), Y: ye, EstacionID: estEnt, Hora: horaEnt},
		Prueba:        &dataset.Muestra{X: filas(xp, nPru), Y: yp, EstacionID: estPru, Hora: horaPru},
		PesosClase:    pc,
		Normalizador:  dataset.Normalizador{Medias: medias, Desviaciones: desv},
	}, nil
}
