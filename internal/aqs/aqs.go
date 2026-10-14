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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"proyecto9/internal/registro"
)

// Índices de columna del CSV horario de AQS (State Code, County Code, ...,
// Date of Last Change), confirmados contra los 4 archivos descargados.
const (
	idxStateCode         = 0
	idxCountyCode        = 1
	idxSiteNum           = 2
	idxPOC               = 4
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

const (
	unidadPPM = "Parts per million"
	unidadPPB = "Parts per billion"
)

// unidadEsperada por contaminante, según la tabla de umbrales AQI del
// proyecto. Si el CSV trae la otra unidad (ppm↔ppb) se convierte; si trae
// cualquier otra, la fila se descarta con MotivoUnidad.
var unidadEsperada = map[string]string{
	"o3":  unidadPPM,
	"co":  unidadPPM,
	"so2": unidadPPB,
	"no2": unidadPPB,
}

// Archivos devuelve la ruta del CSV horario de cada contaminante para un
// año, con el nombre que usa la EPA (hourly_<codigo>_<anio>.csv).
func Archivos(dir, anio string) map[string]string {
	codigos := map[string]string{"o3": "44201", "co": "42101", "so2": "42401", "no2": "42602"}
	archivos := make(map[string]string, len(codigos))
	for contaminante, codigo := range codigos {
		archivos[contaminante] = filepath.Join(dir, fmt.Sprintf("hourly_%s_%s.csv", codigo, anio))
	}
	return archivos
}

// Motivo explica por qué una fila se aceptó o se descartó; los cargadores
// reportan el conteo por motivo para documentar la limpieza.
type Motivo int

const (
	Aceptada Motivo = iota
	MotivoSinMedicion
	MotivoQualifier
	MotivoUnidad
	MotivoFormato
	numMotivos
)

var nombresMotivo = [numMotivos]string{"aceptada", "sin_medicion", "qualifier_excluido", "unidad_desconocida", "formato_invalido"}

func (m Motivo) String() string { return nombresMotivo[m] }

// Conteo acumula las filas leídas, las descartadas por motivo y las que
// requirieron convertir unidades.
type Conteo struct {
	Leidas      int64
	PorMotivo   [numMotivos]int64
	Convertidas int64
}

func (c *Conteo) Descartadas() int64 { return c.Leidas - c.PorMotivo[Aceptada] }

func (c *Conteo) Sumar(otro Conteo) {
	c.Leidas += otro.Leidas
	c.Convertidas += otro.Convertidas
	for i := range c.PorMotivo {
		c.PorMotivo[i] += otro.PorMotivo[i]
	}
}

func (c *Conteo) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "leidas=%d descartadas=%d convertidas=%d (", c.Leidas, c.Descartadas(), c.Convertidas)
	for m := MotivoSinMedicion; m < numMotivos; m++ {
		if m > MotivoSinMedicion {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%s=%d", m, c.PorMotivo[m])
	}
	b.WriteString(")")
	return b.String()
}

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
	// POC identifica el monitor dentro del sitio: algunos sitios miden el
	// mismo gas con varios monitores a la vez.
	POC int
}

// procesarFila valida y transforma una fila cruda; no toca el índice
// compartido, así que es segura de llamar desde cualquier goroutine. El
// segundo valor indica si hubo que convertir la unidad.
func procesarFila(fila []string, contaminante string) (ValorLeido, Motivo, bool) {
	if fila[idxSampleMeasurement] == "" {
		return ValorLeido{}, MotivoSinMedicion, false
	}
	if excluidoPorQualifier(fila[idxQualifier]) {
		return ValorLeido{}, MotivoQualifier, false
	}

	valor, errValor := strconv.ParseFloat(fila[idxSampleMeasurement], 64)
	lat, errLat := strconv.ParseFloat(fila[idxLatitude], 64)
	lon, errLon := strconv.ParseFloat(fila[idxLongitude], 64)
	if errValor != nil || errLat != nil || errLon != nil {
		return ValorLeido{}, MotivoFormato, false
	}

	valor, convertida, ok := convertirUnidad(valor, fila[idxUnitsOfMeasure], unidadEsperada[contaminante])
	if !ok {
		return ValorLeido{}, MotivoUnidad, false
	}

	marcaUTC, errFecha := timestampUTC(fila[idxDateGMT], fila[idxTimeGMT])
	poc, errPOC := strconv.Atoi(fila[idxPOC])
	if errFecha != nil || errPOC != nil || poc < 0 {
		return ValorLeido{}, MotivoFormato, false
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
		POC:          min(poc, 255),
	}, Aceptada, convertida
}

// convertirUnidad lleva valor de la unidad del archivo a la esperada,
// según el contrato: ppm↔ppb se convierte (factor 1000), nunca se descarta
// por eso. Cualquier otra unidad no se puede convertir con seguridad.
func convertirUnidad(valor float64, unidad, esperada string) (float64, bool, bool) {
	switch {
	case unidad == esperada:
		return valor, false, true
	case unidad == unidadPPB && esperada == unidadPPM:
		return valor / 1000, true, true
	case unidad == unidadPPM && esperada == unidadPPB:
		return valor * 1000, true, true
	default:
		return 0, false, false
	}
}

// excluidoPorQualifier descarta las mediciones marcadas por la agencia con
// un código de "solicitud de exclusión" por evento excepcional (prefijo R
// en el catálogo de qualifiers de AQS, p. ej. RT = incendio forestal). Los
// códigos informativos (prefijo I) y los de validación no se descartan:
// la medición es válida, solo viene anotada.
func excluidoPorQualifier(qualifier string) bool {
	return len(qualifier) >= 2 && qualifier[0] == 'R' && qualifier[1] >= 'A' && qualifier[1] <= 'Z'
}

// Indice une los valores leídos por (estacion_id, timestamp_utc). No es
// seguro para varias goroutines a la vez: lo usa siempre una única
// goroutina agregadora, tanto en el cargador secuencial como en el
// concurrente.
type Indice struct {
	Registros map[string]*registro.RegistroProcesado
	// textos interna estado y condado: hay ~50 estados y ~1000 condados
	// para millones de registros, así que cada registro comparte la misma
	// cadena en vez de tener la suya.
	textos map[string]string
	// Duplicados cuenta los valores que coincidieron en (estación, hora,
	// gas) con el de otro monitor del mismo sitio (ver Agregar).
	Duplicados int64
}

func NuevoIndice() *Indice {
	return &Indice{
		Registros: make(map[string]*registro.RegistroProcesado),
		textos:    make(map[string]string),
	}
}

func (ix *Indice) internar(s string) string {
	if t, ok := ix.textos[s]; ok {
		return t
	}
	t := strings.Clone(s)
	ix.textos[t] = t
	return t
}

// Agregar une v al registro de su (estacion, hora).
//
// Cuidado con la memoria: v.Estado y v.Condado son subcadenas de la línea
// completa que devolvió csv.Reader. Guardarlas tal cual, o tomar &v.Valor
// (que hace escapar a todo v al heap), mantendría viva cada línea del CSV
// mientras viva el índice: unos 4 GB extra para 16 M de filas, que era la
// causa de que el cargador se quedara sin memoria. Por eso se internan las
// cadenas y solo se reserva el float64.
//
// Monitores repetidos: en 2025 hay 53 sitios que miden el mismo gas con
// más de un monitor (POC), y ~2 % de las filas repiten (estación, hora,
// gas). Quedarse con "el último que llega" haría que el resultado
// dependiera del orden de las goroutines: una condición de carrera lógica
// que -race no detecta, porque no hay accesos simultáneos a memoria. La
// regla es determinista: gana el monitor de POC más bajo (normalmente el
// principal del sitio), llegue en el orden que llegue.
func (ix *Indice) Agregar(v ValorLeido) {
	clave := v.EstacionID + "|" + v.TimestampUTC

	reg, existe := ix.Registros[clave]
	if !existe {
		reg = &registro.RegistroProcesado{
			EstacionID:   v.EstacionID,
			TimestampUTC: v.TimestampUTC,
			Latitud:      v.Latitud,
			Longitud:     v.Longitud,
			Estado:       ix.internar(v.Estado),
			Condado:      ix.internar(v.Condado),
		}
		ix.Registros[clave] = reg
	}

	var campo **float64
	var g int
	switch v.Contaminante {
	case "o3":
		campo, g = &reg.O3Ppm, 0
	case "no2":
		campo, g = &reg.NO2Ppb, 1
	case "so2":
		campo, g = &reg.SO2Ppb, 2
	case "co":
		campo, g = &reg.COPpm, 3
	default:
		return
	}

	if *campo != nil {
		ix.Duplicados++
		if v.POC >= int(reg.POC[g]) {
			return
		}
	}
	valor := new(float64)
	*valor = v.Valor
	*campo = valor
	reg.POC[g] = uint8(v.POC)
}

// Procesar lee secuencialmente el CSV horario de un contaminante y agrega
// sus valores válidos a indice. Es la versión de una sola goroutine (línea
// base T(1)); para el cargador concurrente ver LimitesBloques y
// ProcesarRango. tiempoIndexado acumula lo que se tarda en Agregar, la
// parte que en el concurrente queda serializada en la agregadora.
func Procesar(entrada io.Reader, contaminante string, indice *Indice) (c Conteo, tiempoIndexado time.Duration, err error) {
	if _, ok := unidadEsperada[contaminante]; !ok {
		return c, 0, fmt.Errorf("contaminante desconocido: %s", contaminante)
	}

	lector := csv.NewReader(entrada)
	lector.ReuseRecord = true
	lector.FieldsPerRecord = numColumnas

	if _, err = lector.Read(); err != nil {
		return c, 0, fmt.Errorf("leyendo encabezado: %w", err)
	}

	for {
		fila, errLectura := lector.Read()
		if errLectura == io.EOF {
			break
		}
		if errLectura != nil {
			return c, tiempoIndexado, fmt.Errorf("leyendo fila %d: %w", c.Leidas+1, errLectura)
		}
		c.Leidas++

		valor, motivo, convertida := procesarFila(fila, contaminante)
		c.PorMotivo[motivo]++
		if motivo != Aceptada {
			continue
		}
		if convertida {
			c.Convertidas++
		}
		inicio := time.Now()
		indice.Agregar(valor)
		tiempoIndexado += time.Since(inicio)
	}

	return c, tiempoIndexado, nil
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

// ProcesarRango valida las filas del bloque [inicio, fin) de ruta y pasa
// cada valor válido a emitir a medida que lo encuentra (en el cargador
// concurrente, emitir lo envía por el channel de la agregadora dueña), sin acumular el
// bloque entero en memoria — el mismo principio de "nunca cargar el
// archivo completo en memoria" que ya sigue el cargador secuencial, ahora
// por bloque. La llama cada goroutine trabajadora sobre su propio bloque;
// no toca estado compartido, así que varias corren en paralelo sin
// necesidad de sincronización.
func ProcesarRango(ruta, contaminante string, inicio, fin int64, emitir func(ValorLeido)) (c Conteo, err error) {
	f, err := os.Open(ruta)
	if err != nil {
		return c, err
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
			return c, fmt.Errorf("leyendo fila %d de %s[%d:%d]: %w", c.Leidas+1, ruta, inicio, fin, errLectura)
		}
		c.Leidas++

		valor, motivo, convertida := procesarFila(fila, contaminante)
		c.PorMotivo[motivo]++
		if motivo != Aceptada {
			continue
		}
		if convertida {
			c.Convertidas++
		}
		emitir(valor)
	}

	return c, nil
}

func timestampUTC(fechaGMT, horaGMT string) (string, error) {
	t, err := time.Parse("2006-01-02 15:04", fechaGMT+" "+horaGMT)
	if err != nil {
		return "", err
	}
	return t.UTC().Format(time.RFC3339), nil
}
