// Prueba end-to-end de SACA: recorre todo el sistema con un subconjunto
// pequeño de los datos reales, en una base de datos y una carpeta
// temporales, y verifica que cada pieza funcione y que las versiones
// secuencial y concurrente den los mismos resultados. Tarda unos pocos
// minutos y no toca la base de datos principal, data/ ni resultados/
// (no vuelve a correr los experimentos).
//
// Uso (desde la raíz del repo, con MongoDB en localhost:27017):
//
//	go run ./herramientas/cmd/e2e
//	go run ./herramientas/cmd/e2e -sin-frontend -verificar-datos=false
//
// Termina con código 0 si todo pasa y 1 si algo falla.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"proyecto9/internal/aqs"
	"proyecto9/internal/persistencia"
)

const baseDatosE2E = "saca_e2e"

var (
	mongoURI       = flag.String("mongo-uri", "mongodb://localhost:27017", "MongoDB donde crear la base temporal "+baseDatosE2E)
	filas          = flag.Int("filas", 150000, "filas de datos que se toman de cada CSV horario")
	dirDatos       = flag.String("datos", "data/raw", "carpeta con los CSV crudos de la EPA")
	verificarDatos = flag.Bool("verificar-datos", true, "verificar los CSV contra data/manifiesto.json (unos 20 s)")
	sinFrontend    = flag.Bool("sin-frontend", false, "omitir lint, build y servidor del frontend")
	sinRace        = flag.Bool("sin-race", false, "correr go test sin el detector de carreras (si no hay compilador de C)")
	mantener       = flag.Bool("mantener", false, "no borrar la carpeta temporal ni la base "+baseDatosE2E)
)

// estado se comparte entre los pasos.
type estado struct {
	tmp, bin, subconjunto string
	ext                   string
	registros             int64
	huella                huella
	perdidaSecuencial     float64
}

func (e *estado) programa(nombre string) string { return filepath.Join(e.bin, nombre+e.ext) }

type paso struct {
	nombre string
	correr func(*estado) (string, error)
}

var errOmitido = errors.New("omitido")

func main() {
	flag.Parse()
	if !existe("go.mod") {
		fmt.Println("Ejecuta la prueba desde la raíz del repositorio (donde está go.mod).")
		os.Exit(1)
	}

	tmp, err := os.MkdirTemp("", "saca-e2e-")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	e := &estado{tmp: tmp, bin: filepath.Join(tmp, "bin"), subconjunto: filepath.Join(tmp, "datos")}
	if runtime.GOOS == "windows" {
		e.ext = ".exe"
	}

	pasos := []paso{
		{"Herramientas instaladas", pasoHerramientas},
		{"Compilar todos los programas", pasoCompilar},
		{"go vet", pasoVet},
		{"go test (pruebas unitarias)", pasoTests},
		{"Datos crudos contra el manifiesto", pasoVerificarDatos},
		{"Subconjunto de datos reales", pasoSubconjunto},
		{"Conexión a MongoDB", pasoMongo},
		{"Cargador secuencial → MongoDB", pasoCargadorSecuencial},
		{"Cargador concurrente = secuencial (registros y valores)", pasoCargadorConcurrente},
		{"Entrenamiento secuencial + modelo", pasoEntrenamiento},
		{"Entrenamiento paralelo (3 estrategias) = secuencial", pasoEntrenamientoParalelo},
		{"Clúster TCP: entrenamiento distribuido = secuencial", pasoCluster},
		{"Exportar datos para el frontend", pasoExportar},
		{"Análisis de experimentos (resultados existentes)", pasoAnalisis},
		{"Frontend: lint, build y servidor", pasoFrontend},
	}

	fmt.Printf("SACA — prueba end-to-end (carpeta temporal %s)\n\n", tmp)
	inicio := time.Now()
	fallo := false
	for i, p := range pasos {
		fmt.Printf("[%2d/%d] %-58s ", i+1, len(pasos), p.nombre)
		t := time.Now()
		detalle, err := p.correr(e)
		dur := time.Since(t).Round(100 * time.Millisecond)
		switch {
		case errors.Is(err, errOmitido):
			fmt.Printf("OMITIDO  %s\n", detalle)
		case err != nil:
			fmt.Printf("FALLÓ    (%s)\n\n%v\n", dur, err)
			fallo = true
		default:
			fmt.Printf("OK  %6s  %s\n", dur, detalle)
		}
		if fallo {
			break
		}
	}

	limpiar(e)
	fmt.Printf("\nTiempo total: %s\n", time.Since(inicio).Round(time.Second))
	if fallo {
		fmt.Println("RESULTADO: FALLÓ — revisa el paso marcado arriba.")
		os.Exit(1)
	}
	fmt.Println("RESULTADO: TODO OK")
}

// ----------------------------------------------------------------- pasos

func pasoHerramientas(e *estado) (string, error) {
	salida, err := ejecutar("", "go", "version")
	if err != nil {
		return "", err
	}
	detalle := strings.TrimSpace(strings.TrimPrefix(salida, "go version "))
	if !*sinFrontend {
		v, err := ejecutar("", "node", "--version")
		if err != nil {
			return "", fmt.Errorf("falta Node.js (o usa -sin-frontend): %w", err)
		}
		detalle += ", node " + strings.TrimSpace(v)
	}
	return detalle, nil
}

// programas se compilan con nombre explícito: hay dos paquetes main
// llamados "secuencial" (cargador y entrenamiento).
var programas = [][2]string{
	{"cargador-secuencial", "./cargador/cmd/secuencial"},
	{"cargador-concurrente", "./cargador/cmd/concurrente"},
	{"entrenamiento-secuencial", "./entrenamiento/cmd/secuencial"},
	{"entrenamiento-paralelo", "./entrenamiento/cmd/paralelo"},
	{"maestro", "./entrenamiento/cmd/maestro"},
	{"trabajador", "./entrenamiento/cmd/trabajador"},
	{"api", "./api/cmd/api"},
	{"descargar", "./herramientas/cmd/descargar"},
	{"exportar", "./herramientas/cmd/exportar"},
	{"analisis", "./experimentos/cmd/analisis"},
}

func pasoCompilar(e *estado) (string, error) {
	if _, err := ejecutar("", "go", "build", "./..."); err != nil {
		return "", err
	}
	for _, p := range programas {
		if _, err := ejecutar("", "go", "build", "-o", e.programa(p[0]), p[1]); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%d ejecutables", len(programas)), nil
}

func pasoVet(e *estado) (string, error) {
	_, err := ejecutar("", "go", "vet", "./...")
	return "", err
}

func pasoTests(e *estado) (string, error) {
	args := []string{"test", "-count=1", "./..."}
	modo := "sin -race"
	if !*sinRace {
		args = []string{"test", "-race", "-count=1", "./..."}
		modo = "con -race"
	}
	salida, err := ejecutar("", "go", args...)
	if err != nil && !*sinRace && (strings.Contains(salida, "cgo") || strings.Contains(salida, "gcc")) {
		return "", fmt.Errorf("el detector de carreras necesita CGO y un compilador de C; instala gcc o usa -sin-race:\n%w", err)
	}
	if err != nil {
		return "", err
	}
	paquetes := 0
	for _, linea := range strings.Split(salida, "\n") {
		if strings.HasPrefix(linea, "ok") {
			paquetes++
		}
	}
	return fmt.Sprintf("%d paquetes con pruebas, %s", paquetes, modo), nil
}

func pasoVerificarDatos(e *estado) (string, error) {
	if !*verificarDatos {
		return "-verificar-datos=false", errOmitido
	}
	if _, err := ejecutar("", e.programa("descargar"), "-verificar"); err != nil {
		return "", fmt.Errorf("los CSV no coinciden con el manifiesto (¿faltan? ejecuta go run ./herramientas/cmd/descargar): %w", err)
	}
	return "SHA-256 de los 5 CSV coincide", nil
}

// pasoSubconjunto copia el encabezado y las primeras N filas de cada CSV
// horario. Los archivos de la EPA vienen ordenados por estación y fecha,
// así que el subconjunto contiene el año completo de unas pocas
// estaciones, que es lo que necesitan el etiquetado (hora siguiente) y la
// partición temporal (meses 10-12 para prueba).
func pasoSubconjunto(e *estado) (string, error) {
	if err := os.MkdirAll(e.subconjunto, 0o755); err != nil {
		return "", err
	}
	total := 0
	for _, ruta := range aqs.Archivos(*dirDatos, "2025") {
		n, err := copiarPrimerasFilas(ruta, filepath.Join(e.subconjunto, filepath.Base(ruta)), *filas)
		if err != nil {
			return "", fmt.Errorf("%w (¿faltan los datos? ejecuta go run ./herramientas/cmd/descargar)", err)
		}
		total += n
	}
	return fmt.Sprintf("%d filas de los 4 CSV", total), nil
}

func copiarPrimerasFilas(origen, destino string, n int) (int, error) {
	in, err := os.Open(origen)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.Create(destino)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	lector := bufio.NewReaderSize(in, 1<<20)
	escritor := bufio.NewWriter(out)
	copiadas := 0
	for i := 0; i <= n; i++ { // encabezado + n filas
		linea, err := lector.ReadBytes('\n')
		escritor.Write(linea)
		if i > 0 && len(linea) > 0 {
			copiadas++
		}
		if err != nil {
			break
		}
	}
	return copiadas, escritor.Flush()
}

func pasoMongo(e *estado) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cliente, err := mongo.Connect(options.Client().ApplyURI(*mongoURI))
	if err != nil {
		return "", err
	}
	defer cliente.Disconnect(ctx)
	if err := cliente.Ping(ctx, nil); err != nil {
		return "", fmt.Errorf("MongoDB no responde en %s.\n  Windows sin Docker: powershell -ExecutionPolicy Bypass -File scripts/mongo-local.ps1\n  Con Docker: docker compose up -d mongo\n%w", *mongoURI, err)
	}
	return *mongoURI + ", base temporal " + baseDatosE2E, nil
}

func pasoCargadorSecuencial(e *estado) (string, error) {
	csvRes := filepath.Join(e.tmp, "cargador.csv")
	if _, err := ejecutar("", e.programa("cargador-secuencial"), "-datos", e.subconjunto, "-mongo-uri", *mongoURI, "-db", baseDatosE2E, "-resultado", csvRes); err != nil {
		return "", err
	}
	filas, err := leerCSV(csvRes)
	if err != nil {
		return "", err
	}
	e.registros = entero(filas[0]["registros"])
	enMongo, err := contarDocumentos()
	if err != nil {
		return "", err
	}
	if e.registros == 0 || enMongo != e.registros {
		return "", fmt.Errorf("el cargador reportó %d registros pero MongoDB tiene %d", e.registros, enMongo)
	}
	if e.huella, err = calcularHuella(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s filas → %d registros en MongoDB", filas[0]["filas_leidas"], e.registros), nil
}

// pasoCargadorConcurrente corre el diseño original (1 agregadora) y el
// particionado, y exige que ambos lean las mismas filas y dejen en MongoDB
// exactamente los mismos registros que el secuencial.
func pasoCargadorConcurrente(e *estado) (string, error) {
	csvRes := filepath.Join(e.tmp, "cargador.csv")
	var modos []string
	for _, agregadoras := range []string{"1", "0"} {
		if _, err := ejecutar("", e.programa("cargador-concurrente"), "-datos", e.subconjunto, "-goroutines", "4",
			"-agregadoras", agregadoras, "-mongo-uri", *mongoURI, "-db", baseDatosE2E, "-resultado", csvRes); err != nil {
			return "", err
		}
		filas, err := leerCSV(csvRes)
		if err != nil {
			return "", err
		}
		primera, ultima := filas[0], filas[len(filas)-1]
		if ultima["registros"] != primera["registros"] || ultima["filas_leidas"] != primera["filas_leidas"] {
			return "", fmt.Errorf("%s: %s filas / %s registros; el secuencial dio %s / %s",
				ultima["modo"], ultima["filas_leidas"], ultima["registros"], primera["filas_leidas"], primera["registros"])
		}
		enMongo, err := contarDocumentos()
		if err != nil {
			return "", err
		}
		if enMongo != e.registros {
			return "", fmt.Errorf("%s dejó %d documentos en MongoDB; se esperaban %d", ultima["modo"], enMongo, e.registros)
		}
		h, err := calcularHuella()
		if err != nil {
			return "", err
		}
		if dif := e.huella.diferencia(h); dif != "" {
			return "", fmt.Errorf("%s guardó valores distintos al secuencial (%s): el resultado depende del orden de las goroutines", ultima["modo"], dif)
		}
		modos = append(modos, ultima["modo"])
	}
	return strings.Join(modos, " y ") + ": mismos registros y mismos valores", nil
}

func (e *estado) argsEntrenamiento(csvRes string) []string {
	return []string{"-db", baseDatosE2E, "-mongo-uri", *mongoURI, "-muestra", "20000", "-epocas", "50",
		"-cache", filepath.Join(e.tmp, "muestra.gob"), "-modelo", filepath.Join(e.tmp, "modelo.json"), "-resultado", csvRes}
}

func pasoEntrenamiento(e *estado) (string, error) {
	csvRes := filepath.Join(e.tmp, "entrenamiento.csv")
	if _, err := ejecutar("", e.programa("entrenamiento-secuencial"), e.argsEntrenamiento(csvRes)...); err != nil {
		return "", err
	}
	filas, err := leerCSV(csvRes)
	if err != nil {
		return "", err
	}
	e.perdidaSecuencial = decimal(filas[0]["perdida_final"])
	if !(e.perdidaSecuencial > 0 && e.perdidaSecuencial < 0.6932) {
		return "", fmt.Errorf("pérdida final %v fuera de rango (debe bajar desde ln 2 = 0,693)", e.perdidaSecuencial)
	}

	var modelo struct {
		Pesos []float64 `json:"pesos"`
	}
	datos, err := os.ReadFile(filepath.Join(e.tmp, "modelo.json"))
	if err != nil {
		return "", fmt.Errorf("no se guardó el modelo: %w", err)
	}
	if err := json.Unmarshal(datos, &modelo); err != nil || len(modelo.Pesos) == 0 {
		return "", fmt.Errorf("modelo.json inválido: %v", err)
	}
	return fmt.Sprintf("pérdida %.6f, modelo de %d pesos guardado", e.perdidaSecuencial, len(modelo.Pesos)), nil
}

// pasoEntrenamientoParalelo verifica la propiedad central de la Parte B:
// repartir el gradiente entre goroutines, con cualquier estrategia de
// sincronización, da el mismo modelo que el secuencial.
func pasoEntrenamientoParalelo(e *estado) (string, error) {
	csvRes := filepath.Join(e.tmp, "paralelo.csv")
	for _, estrategia := range []string{"reduccion", "mutex", "atomico"} {
		args := append(e.argsEntrenamiento(csvRes), "-goroutines", "4", "-estrategia", estrategia)
		if _, err := ejecutar("", e.programa("entrenamiento-paralelo"), args...); err != nil {
			return "", err
		}
	}
	filas, err := leerCSV(csvRes)
	if err != nil {
		return "", err
	}
	for _, f := range filas {
		if d := math.Abs(decimal(f["perdida_final"]) - e.perdidaSecuencial); d > 1e-6 {
			return "", fmt.Errorf("%s: pérdida %s difiere de la secuencial %.6f en %.2e", f["modo"], f["perdida_final"], e.perdidaSecuencial, d)
		}
	}
	return "reducción, mutex y atómico dan la misma pérdida (±1e-6)", nil
}

// pasoCluster prueba el entrenamiento distribuido con procesos reales que
// solo se hablan por TCP. Primero, un maestro y 2 trabajadores entrenan
// con la misma muestra que el paso secuencial, y la pérdida final debe ser
// la misma. Después, con un entrenamiento largo en curso, un nodo sobrante
// y uno con otra muestra deben ser rechazados, y al matar un trabajador el
// maestro debe abortar en vez de colgarse.
func pasoCluster(e *estado) (string, error) {
	muestra := filepath.Join(e.tmp, "muestra.gob")
	csvRes := filepath.Join(e.tmp, "cluster.csv")
	direccion, err := direccionLibre()
	if err != nil {
		return "", err
	}
	maestro, err := arrancar("", e.programa("maestro"), "-nodos", "2", "-escuchar", direccion, "-muestra", muestra,
		"-epocas", "50", "-resultado", csvRes, "-modo", "tcp_e2e")
	if err != nil {
		return "", err
	}
	defer maestro.detener()
	if err := maestro.esperar("maestro escuchando", 20*time.Second); err != nil {
		return "", err
	}
	for _, id := range []string{"w1", "w2"} {
		t, err := arrancar("", e.programa("trabajador"), "-maestro", direccion, "-id", id, "-muestra", muestra, "-motor", "go", "-hilos", "2")
		if err != nil {
			return "", err
		}
		defer t.detener()
	}
	if err := maestro.esperar("resultados agregados", 60*time.Second); err != nil {
		return "", err
	}
	filas, err := leerCSV(csvRes)
	if err != nil {
		return "", err
	}
	distribuida := decimal(filas[0]["perdida_final"])
	if d := math.Abs(distribuida - e.perdidaSecuencial); filas[0]["modo"] != "tcp_e2e" || d > 1e-6 {
		return "", fmt.Errorf("pérdida distribuida %.6f (modo %s); la secuencial fue %.6f", distribuida, filas[0]["modo"], e.perdidaSecuencial)
	}

	// Segunda parte: rechazos y caída durante un entrenamiento largo.
	if direccion, err = direccionLibre(); err != nil {
		return "", err
	}
	largo, err := arrancar("", e.programa("maestro"), "-nodos", "2", "-escuchar", direccion, "-muestra", muestra,
		"-epocas", "1000000", "-verificar=false")
	if err != nil {
		return "", err
	}
	defer largo.detener()
	if err := largo.esperar("maestro escuchando", 20*time.Second); err != nil {
		return "", err
	}
	var nodos []*procesoConLog
	for _, id := range []string{"w1", "w2"} {
		t, err := arrancar("", e.programa("trabajador"), "-maestro", direccion, "-id", id, "-muestra", muestra, "-motor", "go", "-hilos", "1")
		if err != nil {
			return "", err
		}
		defer t.detener()
		nodos = append(nodos, t)
	}
	if err := largo.esperar("empieza el entrenamiento", 30*time.Second); err != nil {
		return "", err
	}
	salida, err := ejecutar("", e.programa("trabajador"), "-maestro", direccion, "-id", "w3", "-muestra", muestra, "-motor", "go")
	if err == nil || !strings.Contains(salida, "todas las particiones ya están asignadas") {
		return "", fmt.Errorf("el tercer trabajador debía ser rechazado por sobrante; salida:\n%s", ultimasLineas(salida, 5))
	}
	otra := filepath.Join(e.tmp, "otra_muestra.gob")
	if err := copiarConUnByteMas(muestra, otra); err != nil {
		return "", err
	}
	salida, err = ejecutar("", e.programa("trabajador"), "-maestro", direccion, "-id", "w4", "-muestra", otra, "-motor", "go")
	if err == nil || !strings.Contains(salida, "muestra_distinta") {
		return "", fmt.Errorf("el trabajador con otra muestra debía ser rechazado; salida:\n%s", ultimasLineas(salida, 5))
	}
	nodos[0].detener()
	if err := largo.esperar("el nodo w1 se cayó", 20*time.Second); err != nil {
		return "", err
	}
	return fmt.Sprintf("2 nodos: pérdida %.6f = secuencial; sobrante y muestra distinta rechazados; caída detectada", distribuida), nil
}

// direccionLibre devuelve 127.0.0.1:<puerto libre>.
func direccionLibre() (string, error) {
	puerto, err := puertoLibre()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("127.0.0.1:%d", puerto), nil
}

// copiarConUnByteMas copia la muestra agregando un byte al final: se sigue
// pudiendo leer, pero su huella SHA-256 cambia.
func copiarConUnByteMas(origen, destino string) error {
	datos, err := os.ReadFile(origen)
	if err != nil {
		return err
	}
	return os.WriteFile(destino, append(datos, 0), 0o644)
}

func pasoExportar(e *estado) (string, error) {
	salidaDir := filepath.Join(e.tmp, "frontend-datos")
	if _, err := ejecutar("", e.programa("exportar"), "-db", baseDatosE2E, "-mongo-uri", *mongoURI,
		"-modelo", filepath.Join(e.tmp, "modelo.json"), "-salida", salidaDir); err != nil {
		return "", err
	}
	var estaciones []map[string]any
	var series map[string]map[string]any
	var resumen map[string]any
	archivos := map[string]any{"estaciones.json": &estaciones, "series.json": &series, "resumen.json": &resumen}
	for nombre, destino := range archivos {
		datos, err := os.ReadFile(filepath.Join(salidaDir, nombre))
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(datos, destino); err != nil {
			return "", fmt.Errorf("%s inválido: %w", nombre, err)
		}
	}
	if len(estaciones) == 0 || len(series) == 0 {
		return "", fmt.Errorf("exportación vacía: %d estaciones, %d series", len(estaciones), len(series))
	}
	if registros, _ := resumen["registros_unicos"].(float64); int64(registros) != e.registros {
		return "", fmt.Errorf("resumen.json dice %v registros; MongoDB tiene %d", resumen["registros_unicos"], e.registros)
	}
	return fmt.Sprintf("%d estaciones, %d series del %v", len(estaciones), len(series), resumen["dia_series"]), nil
}

// pasoAnalisis vuelve a analizar el CSV del Experimento 1b que ya está en
// resultados/ (sin volver a correr el experimento) hacia la carpeta
// temporal, para comprobar que el análisis funciona.
func pasoAnalisis(e *estado) (string, error) {
	entrada := filepath.Join("resultados", "experimento1b.csv")
	if !existe(entrada) {
		return "no existe " + entrada, errOmitido
	}
	salidaJSON := filepath.Join(e.tmp, "analisis.json")
	if _, err := ejecutar("", e.programa("analisis"), "-entrada", entrada, "-json", salidaJSON, "-md", filepath.Join(e.tmp, "analisis.md"),
		"-paralelo", "paralelo_reduccion,paralelo_mutex,paralelo_atomico", "-serial="); err != nil {
		return "", err
	}
	var r struct {
		T1     float64 `json:"t1_s"`
		Puntos []struct {
			Unidades int     `json:"unidades"`
			Speedup  float64 `json:"speedup"`
		} `json:"puntos"`
	}
	datos, _ := os.ReadFile(salidaJSON)
	if err := json.Unmarshal(datos, &r); err != nil || len(r.Puntos) == 0 || r.T1 <= 0 {
		return "", fmt.Errorf("análisis inválido: %v", err)
	}
	mejor := r.Puntos[0]
	for _, p := range r.Puntos {
		if p.Speedup > mejor.Speedup {
			mejor = p
		}
	}
	return fmt.Sprintf("Experimento 1b releído: mejor S(%d) = %.2f", mejor.Unidades, mejor.Speedup), nil
}

func pasoFrontend(e *estado) (string, error) {
	if *sinFrontend {
		return "-sin-frontend", errOmitido
	}
	dir := "frontend"
	if !existe(filepath.Join(dir, "node_modules")) {
		if _, err := ejecutar(dir, "npm", "ci", "--no-audit", "--no-fund"); err != nil {
			return "", err
		}
	}
	if _, err := ejecutar(dir, "npm", "run", "lint"); err != nil {
		return "", err
	}
	if _, err := ejecutar(dir, "npm", "run", "build"); err != nil {
		return "", err
	}

	puerto, err := puertoLibre()
	if err != nil {
		return "", err
	}
	// vite se lanza directamente con node (no con npx) para poder detener
	// exactamente ese proceso al terminar.
	servidor, err := arrancar(dir, "node", "node_modules/vite/bin/vite.js", "preview", "--port", strconv.Itoa(puerto), "--strictPort")
	if err != nil {
		return "", err
	}
	defer servidor.detener()

	base := fmt.Sprintf("http://localhost:%d/", puerto)
	if err := esperarHTTP(base, 20*time.Second); err != nil {
		return "", fmt.Errorf("%w\n%s", err, ultimasLineas(servidor.texto(), 10))
	}
	rutas := []string{"", "datos/estaciones.json", "datos/series.json", "datos/resumen.json", "datos/experimento1.json", "datos/experimento1b.json"}
	for _, ruta := range rutas {
		resp, err := http.Get(base + ruta)
		if err != nil {
			return "", err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("GET /%s respondió %s", ruta, resp.Status)
		}
	}
	return fmt.Sprintf("build OK, %d recursos servidos", len(rutas)), nil
}

// ----------------------------------------------------------- utilidades

func limpiar(e *estado) {
	if *mantener {
		fmt.Printf("\nSe conservan %s y la base %s (-mantener).\n", e.tmp, baseDatosE2E)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if cliente, err := mongo.Connect(options.Client().ApplyURI(*mongoURI)); err == nil {
		cliente.Database(baseDatosE2E).Drop(ctx)
		cliente.Disconnect(ctx)
	}
	os.RemoveAll(e.tmp)
}

// huella resume los valores guardados: cantidad y suma de cada gas. Dos
// cargas de los mismos datos deben dar la misma huella, no solo la misma
// cantidad de documentos; así se detecta que un valor dependa del orden en
// que llegan los datos (por ejemplo, con monitores repetidos en un sitio).
type huella struct {
	cantidad [4]int64
	suma     [4]float64
}

var camposGases = [4]string{"o3_ppm", "no2_ppb", "so2_ppb", "co_ppm"}

func (a huella) diferencia(b huella) string {
	for i, campo := range camposGases {
		if a.cantidad[i] != b.cantidad[i] {
			return fmt.Sprintf("%s: %d valores frente a %d", campo, a.cantidad[i], b.cantidad[i])
		}
		if math.Abs(a.suma[i]-b.suma[i]) > 1e-9*math.Max(1, math.Abs(a.suma[i])) {
			return fmt.Sprintf("%s: suma %.6f frente a %.6f", campo, a.suma[i], b.suma[i])
		}
	}
	return ""
}

func calcularHuella() (huella, error) {
	var h huella
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cliente, err := mongo.Connect(options.Client().ApplyURI(*mongoURI))
	if err != nil {
		return h, err
	}
	defer cliente.Disconnect(ctx)

	grupo := bson.D{{Key: "_id", Value: nil}}
	for _, c := range camposGases {
		grupo = append(grupo,
			bson.E{Key: c + "_n", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$type", Value: "$" + c}}, "double"}}}, 1, 0}}}}}},
			bson.E{Key: c + "_s", Value: bson.D{{Key: "$sum", Value: "$" + c}}},
		)
	}
	cursor, err := cliente.Database(baseDatosE2E).Collection(persistencia.Coleccion).Aggregate(ctx, mongo.Pipeline{{{Key: "$group", Value: grupo}}})
	if err != nil {
		return h, err
	}
	var res []bson.M
	if err := cursor.All(ctx, &res); err != nil || len(res) == 0 {
		return h, fmt.Errorf("no se pudo calcular la huella: %v", err)
	}
	for i, c := range camposGases {
		h.cantidad[i] = int64(numeroBSON(res[0][c+"_n"]))
		h.suma[i] = numeroBSON(res[0][c+"_s"])
	}
	return h, nil
}

func numeroBSON(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

func contarDocumentos() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cliente, err := mongo.Connect(options.Client().ApplyURI(*mongoURI))
	if err != nil {
		return 0, err
	}
	defer cliente.Disconnect(ctx)
	return cliente.Database(baseDatosE2E).Collection(persistencia.Coleccion).CountDocuments(ctx, bson.D{})
}

// ejecutar corre un comando y devuelve su salida combinada; si falla, el
// error incluye las últimas líneas para diagnosticar.
func ejecutar(dir, nombre string, args ...string) (string, error) {
	cmd := exec.Command(nombre, args...)
	cmd.Dir = dir
	var salida bytes.Buffer
	cmd.Stdout = &salida
	cmd.Stderr = &salida
	if err := cmd.Run(); err != nil {
		return salida.String(), fmt.Errorf("%s %s: %w\n%s", filepath.Base(nombre), strings.Join(args, " "), err, ultimasLineas(salida.String(), 15))
	}
	return salida.String(), nil
}

func ultimasLineas(s string, n int) string {
	lineas := strings.Split(strings.TrimRight(s, "\r\n"), "\n")
	if len(lineas) > n {
		lineas = lineas[len(lineas)-n:]
	}
	return "    " + strings.Join(lineas, "\n    ")
}

func leerCSV(ruta string) ([]map[string]string, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	filas, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(filas) < 2 {
		return nil, fmt.Errorf("%s no tiene filas", ruta)
	}
	var res []map[string]string
	for _, fila := range filas[1:] {
		m := map[string]string{}
		for i, col := range filas[0] {
			m[col] = fila[i]
		}
		res = append(res, m)
	}
	return res, nil
}

func entero(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func decimal(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func existe(ruta string) bool {
	_, err := os.Stat(ruta)
	return err == nil
}

// puertoLibre pide al sistema un puerto TCP libre.
func puertoLibre() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func esperarHTTP(url string, limite time.Duration) error {
	fin := time.Now().Add(limite)
	for time.Now().Before(fin) {
		if resp, err := http.Get(url); err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("el servidor del frontend no respondió en %s", limite)
}

// procesoConLog arranca un proceso en segundo plano y acumula su salida
// para poder esperar a que aparezca un texto.
type procesoConLog struct {
	cmd *exec.Cmd
	mu  sync.Mutex
	log bytes.Buffer
}

type escritorSeguro struct{ p *procesoConLog }

func (w escritorSeguro) Write(b []byte) (int, error) {
	w.p.mu.Lock()
	defer w.p.mu.Unlock()
	return w.p.log.Write(b)
}

func arrancar(dir, nombre string, args ...string) (*procesoConLog, error) {
	p := &procesoConLog{cmd: exec.Command(nombre, args...)}
	p.cmd.Dir = dir
	p.cmd.Stdout = escritorSeguro{p}
	p.cmd.Stderr = escritorSeguro{p}
	if err := p.cmd.Start(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *procesoConLog) texto() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.log.String()
}

func (p *procesoConLog) esperar(fragmento string, limite time.Duration) error {
	fin := time.Now().Add(limite)
	for time.Now().Before(fin) {
		if strings.Contains(p.texto(), fragmento) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("no apareció %q en %s; salida:\n%s", fragmento, limite, ultimasLineas(p.texto(), 15))
}

func (p *procesoConLog) detener() {
	if p.cmd.Process != nil && p.cmd.ProcessState == nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
}
