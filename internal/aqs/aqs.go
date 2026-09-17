// Package aqs procesa los archivos horarios crudos de la EPA (AQS) y los
// convierte en registro.RegistroProcesado, aplicando las reglas de limpieza
// de docs/contrato_registro_procesado.md. La validación de una fila
// (procesarFila) es la misma tanto para el cargador secuencial como para
// el concurrente: lo único que cambia es qué porción del archivo lee cada
// uno.
package aqs

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"proyecto9/internal/registro"
)

// Índices de columna del CSV horario de AQS (State Code, County Code, ...,
// Date of Last Change), confirmados contra los 4 archivos descargados.
const (
	idxStateCode         = 0
	idxCountyCode        = 1
	idxSiteNum           = 2
	idxLatitude          = 5
	idxLongitude         = 6
	idxDateGMT           = 11
	idxTimeGMT           = 12
	idxSampleMeasurement = 13
	idxUnitsOfMeasure    = 14
	idxQualifier         = 17
	idxStateName         = 21
	idxCountyName        = 22

	numColumnas = 24
)

// unidadEsperada por contaminante, según la tabla de umbrales AQI del
// proyecto. Si el CSV trae una unidad distinta, la fila se descarta en vez
// de convertirla a ciegas.
var unidadEsperada = map[string]string{
	"o3":  "Parts per million",
	"co":  "Parts per million",
	"so2": "Parts per billion",
	"no2": "Parts per billion",
}

// qualifiersExcluidos marca los códigos de evento excepcional a descartar.
// Pendiente de completar contra el catálogo oficial de AQS (ver
// docs/contrato_registro_procesado.md); vacío por ahora porque ninguno de
// los 4 archivos de 2025 trae qualifiers de ese tipo.
var qualifiersExcluidos = map[string]bool{}

// ValorLeido es una fila del CSV ya validada y lista para unirse al
// registro procesado de su (estacion, hora). Es el tipo que cruza el
// channel entre las goroutines trabajadoras y la agregadora.
type ValorLeido struct {
	EstacionID   string
	TimestampUTC string
	Latitud      float64
	Longitud     float64
	Estado       string
	Condado      string
	Contaminante string
	Valor        float64
}

// procesarFila valida y transforma una fila cruda; no toca el índice
// compartido, así que es segura de llamar desde cualquier goroutine.
func procesarFila(fila []string, contaminante string) (ValorLeido, bool) {
	unidad, ok := unidadEsperada[contaminante]
	if !ok || fila[idxSampleMeasurement] == "" || qualifiersExcluidos[fila[idxQualifier]] || fila[idxUnitsOfMeasure] != unidad {
		return ValorLeido{}, false
	}

	valor, errValor := strconv.ParseFloat(fila[idxSampleMeasurement], 64)
	lat, errLat := strconv.ParseFloat(fila[idxLatitude], 64)
	lon, errLon := strconv.ParseFloat(fila[idxLongitude], 64)
	if errValor != nil || errLat != nil || errLon != nil {
		return ValorLeido{}, false
	}

	marcaUTC, errFecha := timestampUTC(fila[idxDateGMT], fila[idxTimeGMT])
	if errFecha != nil {
		return ValorLeido{}, false
	}

	return ValorLeido{
		EstacionID:   fila[idxStateCode] + "-" + fila[idxCountyCode] + "-" + fila[idxSiteNum],
		TimestampUTC: marcaUTC,
		Latitud:      lat,
		Longitud:     lon,
		Estado:       fila[idxStateName],
		Condado:      fila[idxCountyName],
		Contaminante: contaminante,
		Valor:        valor,
	}, true
}

// Indexar agrega un ValorLeido al índice compartido, uniendo por
// (estacion_id, timestamp_utc). No es segura para llamar desde varias
// goroutines a la vez sobre el mismo índice — la usa siempre una única
// goroutina agregadora, tanto en el cargador secuencial como en el
// concurrente.
func Indexar(indice map[string]*registro.RegistroProcesado, v ValorLeido) {
	clave := v.EstacionID + "|" + v.TimestampUTC

	reg, existe := indice[clave]
	if !existe {
		reg = &registro.RegistroProcesado{
			EstacionID:   v.EstacionID,
			TimestampUTC: v.TimestampUTC,
			Latitud:      v.Latitud,
			Longitud:     v.Longitud,
			Estado:       v.Estado,
			Condado:      v.Condado,
		}
		indice[clave] = reg
	}

	switch v.Contaminante {
	case "o3":
		reg.O3Ppm = &v.Valor
	case "no2":
		reg.NO2Ppb = &v.Valor
	case "so2":
		reg.SO2Ppb = &v.Valor
	case "co":
		reg.COPpm = &v.Valor
	}
}

// Procesar lee secuencialmente el CSV horario de un contaminante y agrega
// sus valores válidos a indice. Devuelve cuántas filas leyó y cuántas
// descartó. Es la versión de una sola goroutine (línea base T(1)); para el
// cargador concurrente ver LimitesBloques y ProcesarRango.
func Procesar(entrada io.Reader, contaminante string, indice map[string]*registro.RegistroProcesado) (leidas, descartadas int64, err error) {
	if _, ok := unidadEsperada[contaminante]; !ok {
		return 0, 0, fmt.Errorf("contaminante desconocido: %s", contaminante)
	}

	lector := csv.NewReader(entrada)
	lector.ReuseRecord = true

	if _, err = lector.Read(); err != nil {
		return 0, 0, fmt.Errorf("leyendo encabezado: %w", err)
	}

	for {
		fila, errLectura := lector.Read()
		if errLectura == io.EOF {
			break
		}
		if errLectura != nil {
			return leidas, descartadas, fmt.Errorf("leyendo fila %d: %w", leidas+descartadas+1, errLectura)
		}
		leidas++

		valor, ok := procesarFila(fila, contaminante)
		if !ok {
			descartadas++
			continue
		}
		Indexar(indice, valor)
	}

	return leidas, descartadas, nil
}

// LimitesBloques divide ruta en n bloques de bytes contiguos, cada corte
// ajustado hacia adelante hasta el siguiente salto de línea para no partir
// una fila por la mitad. El primer límite queda justo después del
// encabezado, así que ningún bloque necesita saltarlo aparte. Devuelve n+1
// offsets: el bloque i cubre [limites[i], limites[i+1]).
func LimitesBloques(ruta string, n int) ([]int64, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	tam := info.Size()

	inicioDatos, err := finDeLinea(f, 0)
	if err != nil {
		return nil, err
	}

	limites := make([]int64, 0, n+1)
	limites = append(limites, inicioDatos)
	for i := 1; i < n; i++ {
		aprox := inicioDatos + (tam-inicioDatos)*int64(i)/int64(n)
		ajustado, err := finDeLinea(f, aprox)
		if err != nil {
			return nil, err
		}
		limites = append(limites, ajustado)
	}
	limites = append(limites, tam)

	return limites, nil
}

// finDeLinea devuelve el offset del byte siguiente al primer '\n' que
// aparece a partir de offset (o el fin de archivo si no hay ninguno).
func finDeLinea(f *os.File, offset int64) (int64, error) {
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	linea, err := bufio.NewReader(f).ReadString('\n')
	if err != nil && err != io.EOF {
		return 0, err
	}
	return offset + int64(len(linea)), nil
}

// ProcesarRango valida las filas del bloque [inicio, fin) de ruta y envía
// cada valor válido a salida a medida que lo encuentra, sin acumular el
// bloque entero en memoria — el mismo principio de "nunca cargar el
// archivo completo en memoria" que ya sigue el cargador secuencial, ahora
// por bloque. La llama cada goroutine trabajadora sobre su propio bloque;
// no toca estado compartido, así que varias corren en paralelo sin
// necesidad de sincronización.
func ProcesarRango(ruta, contaminante string, inicio, fin int64, salida chan<- ValorLeido) (leidas, descartadas int64, err error) {
	f, err := os.Open(ruta)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	lector := csv.NewReader(io.NewSectionReader(f, inicio, fin-inicio))
	lector.FieldsPerRecord = numColumnas

	for {
		fila, errLectura := lector.Read()
		if errLectura == io.EOF {
			break
		}
		if errLectura != nil {
			return leidas, descartadas, fmt.Errorf("leyendo fila %d de %s[%d:%d]: %w", leidas+descartadas+1, ruta, inicio, fin, errLectura)
		}
		leidas++

		valor, ok := procesarFila(fila, contaminante)
		if !ok {
			descartadas++
			continue
		}
		salida <- valor
	}

	return leidas, descartadas, nil
}

func timestampUTC(fechaGMT, horaGMT string) (string, error) {
	t, err := time.Parse("2006-01-02 15:04", fechaGMT+" "+horaGMT)
	if err != nil {
		return "", err
	}
	return t.UTC().Format(time.RFC3339), nil
}
