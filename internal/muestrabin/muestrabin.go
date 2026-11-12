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
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
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
// recorre en su propia goroutine; después se ordena todo y se normaliza en
// una sola pasada secuencial.
func Construir(indices []map[string]*registro.RegistroProcesado, mesCorte int) (*Muestra, error) {
	partes := make([][]fila, len(indices))
	errs := make([]error, len(indices))
	var wg sync.WaitGroup
	for k, ix := range indices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			partes[k], errs[k] = etiquetar(ix)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	total := 0
	for _, p := range partes {
		total += len(p)
	}
	filas := make([]fila, 0, total)
	for _, p := range partes {
		filas = append(filas, p...)
	}
	slices.SortFunc(filas, func(a, b fila) int {
		if c := cmp.Compare(a.estacion, b.estacion); c != 0 {
			return c
		}
		return cmp.Compare(a.hora, b.hora)
	})
	return armar(filas, mesCorte)
}

func etiquetar(ix map[string]*registro.RegistroProcesado) ([]fila, error) {
	// NO2 por estación y hora (horas desde 1970), para los rezagos: una
	// búsqueda por entero en vez de armar claves de texto 24 veces por fila.
	no2 := map[string]map[int64]float64{}
	for _, r := range ix {
		if r.NO2Ppb == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, r.TimestampUTC)
		if err != nil {
			return nil, fmt.Errorf("hora inválida %q en %s", r.TimestampUTC, r.EstacionID)
		}
		serie := no2[r.EstacionID]
		if serie == nil {
			serie = map[int64]float64{}
			no2[r.EstacionID] = serie
		}
		serie[t.Unix()/3600] = *r.NO2Ppb
	}
	var filas []fila
	for _, r := range ix {
		if r.NO2Ppb == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, r.TimestampUTC)
		if err != nil {
			return nil, fmt.Errorf("hora inválida %q en %s", r.TimestampUTC, r.EstacionID)
		}
		futuro, ok := ix[r.EstacionID+"|"+t.Add(HorizonteHoras*time.Hour).Format(time.RFC3339)]
		if !ok || futuro.NO2Ppb == nil {
			continue
		}
		f := fila{estacion: r.EstacionID, hora: t.Unix() / 3600, mes: int(t.Month())}
		serie := no2[r.EstacionID]
		historia := func(k int) (float64, bool) {
			v, ok := serie[f.hora-int64(k)]
			return v, ok
		}
		copy(f.x[:], dataset.VectorCompleto(r, historia))
		if *futuro.NO2Ppb > dataset.UmbralNO2 {
			f.y = 1
		}
		filas = append(filas, f)
	}
	return filas, nil
}

func armar(filas []fila, mesCorte int) (*Muestra, error) {
	const d = D
	m := &Muestra{D: d}
	idEstacion := map[string]uint32{}
	for _, f := range filas {
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
	os.Remove(ruta) // en Windows Rename no reemplaza un archivo existente
	return os.Rename(tmp, ruta)
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
