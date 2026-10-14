package aqs

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"proyecto9/internal/registro"
)

// filaCSV arma una fila de 24 columnas con el mismo orden que los
// archivos horarios de AQS; solo llena las columnas que usa el cargador.
func filaCSV(estado, condado, sitio, fecha, hora, valor, unidad, qualifier string) []string {
	f := make([]string, numColumnas)
	f[idxStateCode] = estado
	f[idxCountyCode] = condado
	f[idxSiteNum] = sitio
	f[idxLatitude] = "34.0669"
	f[idxLongitude] = "-118.2273"
	f[idxDateGMT] = fecha
	f[idxTimeGMT] = hora
	f[idxSampleMeasurement] = valor
	f[idxUnitsOfMeasure] = unidad
	f[idxQualifier] = qualifier
	f[idxStateName] = "California"
	f[idxCountyName] = "Los Angeles"
	return f
}

func TestProcesarFila(t *testing.T) {
	casos := []struct {
		nombre       string
		fila         []string
		contaminante string
		motivo       Motivo
		valor        float64
		convertida   bool
	}{
		{"valida", filaCSV("06", "037", "0002", "2025-03-14", "18:00", "12.3", unidadPPB, ""), "no2", Aceptada, 12.3, false},
		{"sin medicion", filaCSV("06", "037", "0002", "2025-03-14", "18:00", "", unidadPPB, ""), "no2", MotivoSinMedicion, 0, false},
		{"ppb a ppm", filaCSV("06", "037", "0002", "2025-03-14", "18:00", "41", unidadPPB, ""), "o3", Aceptada, 0.041, true},
		{"ppm a ppb", filaCSV("06", "037", "0002", "2025-03-14", "18:00", "0.0123", unidadPPM, ""), "no2", Aceptada, 12.3, true},
		{"unidad desconocida", filaCSV("06", "037", "0002", "2025-03-14", "18:00", "5", "Micrograms/cubic meter (LC)", ""), "so2", MotivoUnidad, 0, false},
		{"qualifier de exclusion", filaCSV("06", "037", "0002", "2025-03-14", "18:00", "5", unidadPPB, "RS"), "so2", MotivoQualifier, 0, false},
		{"qualifier informativo", filaCSV("06", "037", "0002", "2025-03-14", "18:00", "5", unidadPPB, "IT"), "so2", Aceptada, 5, false},
		{"qualifier numerico", filaCSV("06", "037", "0002", "2025-03-14", "18:00", "5", unidadPPB, "1"), "so2", Aceptada, 5, false},
		{"valor no numerico", filaCSV("06", "037", "0002", "2025-03-14", "18:00", "abc", unidadPPB, ""), "so2", MotivoFormato, 0, false},
		{"fecha invalida", filaCSV("06", "037", "0002", "2025-13-40", "18:00", "5", unidadPPB, ""), "so2", MotivoFormato, 0, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			v, motivo, convertida := procesarFila(c.fila, c.contaminante)
			if motivo != c.motivo {
				t.Fatalf("motivo = %s, se esperaba %s", motivo, c.motivo)
			}
			if motivo != Aceptada {
				return
			}
			if math.Abs(v.Valor-c.valor) > 1e-12 || convertida != c.convertida {
				t.Fatalf("valor = %v (convertida=%v), se esperaba %v (convertida=%v)", v.Valor, convertida, c.valor, c.convertida)
			}
			if v.EstacionID != "06-037-0002" || v.TimestampUTC != "2025-03-14T18:00:00Z" {
				t.Fatalf("clave = %s %s", v.EstacionID, v.TimestampUTC)
			}
		})
	}
}

// escribirFixture crea un CSV horario sintético con varias estaciones y
// horas, incluyendo filas que deben descartarse, y devuelve su ruta.
func escribirFixture(t *testing.T, filas int) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "hourly.csv")
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(f)
	encabezado := make([]string, numColumnas)
	for i := range encabezado {
		encabezado[i] = fmt.Sprintf("col%d", i)
	}
	w.Write(encabezado)
	for i := 0; i < filas; i++ {
		valor := fmt.Sprintf("%d.%d", i%90, i%10)
		qualifier := ""
		switch {
		case i%37 == 0:
			valor = ""
		case i%53 == 0:
			qualifier = "RT"
		}
		sitio := fmt.Sprintf("%04d", i%7)
		hora := fmt.Sprintf("%02d:00", (i/7)%24)
		dia := fmt.Sprintf("2025-01-%02d", 1+(i/(7*24))%28)
		w.Write(filaCSV("06", "037", sitio, dia, hora, valor, unidadPPB, qualifier))
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func TestLimitesBloquesNoPartenFilas(t *testing.T) {
	ruta := escribirFixture(t, 1000)
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	inicioDatos := int64(strings.IndexByte(string(contenido), '\n') + 1)

	for n := 1; n <= 16; n++ {
		limites, err := LimitesBloques(ruta, n)
		if err != nil {
			t.Fatal(err)
		}
		if len(limites) != n+1 || limites[0] != inicioDatos || limites[n] != int64(len(contenido)) {
			t.Fatalf("n=%d: limites extremos incorrectos: %v", n, limites)
		}
		for i := 1; i < n; i++ {
			if limites[i] < limites[i-1] {
				t.Fatalf("n=%d: limites no crecientes: %v", n, limites)
			}
			if contenido[limites[i]-1] != '\n' {
				t.Fatalf("n=%d: el limite %d=%d no empieza al inicio de una fila", n, i, limites[i])
			}
		}
	}
}

// TestConcurrenteIgualASecuencial es la garantía central de la Parte A:
// repartir el archivo en bloques entre N goroutines produce exactamente el
// mismo índice y los mismos conteos que leerlo con una sola.
func TestConcurrenteIgualASecuencial(t *testing.T) {
	ruta := escribirFixture(t, 5000)

	f, err := os.Open(ruta)
	if err != nil {
		t.Fatal(err)
	}
	esperado := NuevoIndice()
	conteoEsperado, _, err := Procesar(f, "no2", esperado)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if conteoEsperado.PorMotivo[MotivoSinMedicion] == 0 || conteoEsperado.PorMotivo[MotivoQualifier] == 0 {
		t.Fatalf("el fixture debería incluir filas descartadas: %s", &conteoEsperado)
	}

	for n := 1; n <= 8; n++ {
		limites, err := LimitesBloques(ruta, n)
		if err != nil {
			t.Fatal(err)
		}

		salida := make(chan ValorLeido, 16)
		var (
			wg    sync.WaitGroup
			mu    sync.Mutex
			total Conteo
		)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(inicio, fin int64) {
				defer wg.Done()
				c, err := ProcesarRango(ruta, "no2", inicio, fin, salida)
				if err != nil {
					t.Error(err)
				}
				mu.Lock()
				total.Sumar(c)
				mu.Unlock()
			}(limites[i], limites[i+1])
		}
		go func() {
			wg.Wait()
			close(salida)
		}()

		obtenido := NuevoIndice()
		for v := range salida {
			obtenido.Agregar(v)
		}

		if total != conteoEsperado {
			t.Fatalf("n=%d: conteo %s, se esperaba %s", n, &total, &conteoEsperado)
		}
		compararIndices(t, n, esperado.Registros, obtenido.Registros)
	}
}

func compararIndices(t *testing.T, n int, a, b map[string]*registro.RegistroProcesado) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("n=%d: %d registros, se esperaban %d", n, len(b), len(a))
	}
	igualPtr := func(x, y *float64) bool { return (x == nil) == (y == nil) && (x == nil || *x == *y) }
	for clave, ra := range a {
		rb, ok := b[clave]
		if !ok {
			t.Fatalf("n=%d: falta %s", n, clave)
		}
		if ra.EstacionID != rb.EstacionID || ra.TimestampUTC != rb.TimestampUTC || ra.Estado != rb.Estado ||
			ra.Condado != rb.Condado || !igualPtr(ra.NO2Ppb, rb.NO2Ppb) || !igualPtr(ra.O3Ppm, rb.O3Ppm) {
			t.Fatalf("n=%d: registro %s distinto", n, clave)
		}
	}
}

func TestIndiceInternaTextos(t *testing.T) {
	ix := NuevoIndice()
	linea1 := strings.Repeat("x", 100) + "California"
	linea2 := strings.Repeat("y", 100) + "California"
	ix.Agregar(ValorLeido{EstacionID: "a", TimestampUTC: "t", Estado: linea1[100:], Contaminante: "no2"})
	ix.Agregar(ValorLeido{EstacionID: "b", TimestampUTC: "t", Estado: linea2[100:], Contaminante: "no2"})
	if len(ix.textos) != 2 { // "California" y "" (condado vacío)
		t.Fatalf("se esperaban 2 textos internados, hay %d", len(ix.textos))
	}
	if ix.Registros["a|t"].Estado != "California" || ix.Registros["b|t"].Estado != "California" {
		t.Fatal("estado mal copiado")
	}
}
