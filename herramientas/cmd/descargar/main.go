// Descargador reproducible de los datos crudos de la EPA (AQS). Baja los
// ZIP oficiales, los descomprime en data/raw y deja constancia en
// data/manifiesto.json del tamaño, SHA-256 y número de filas de cada CSV.
// El manifiesto se versiona en git (los CSV no), así cualquier integrante
// puede comprobar con -verificar que trabaja exactamente con los mismos
// datos. Es idempotente: si un CSV ya coincide con el manifiesto, no se
// vuelve a descargar.
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

const urlBase = "https://aqs.epa.gov/aqsweb/airdata"

// Archivos del proyecto (.context/proyecto9.md §5): los 4 horarios de
// gases criterio y el resumen anual por monitor, usado como metadatos de
// estación.
var prefijos = []string{
	"hourly_44201", // O3
	"hourly_42602", // NO2
	"hourly_42401", // SO2
	"hourly_42101", // CO
	"annual_conc_by_monitor",
}

type Archivo struct {
	Nombre string `json:"nombre"`
	URL    string `json:"url"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	Filas  int64  `json:"filas"`
}

type Manifiesto struct {
	Anio      string    `json:"anio"`
	Generado  string    `json:"generado"`
	Archivos  []Archivo `json:"archivos"`
	TotalHora int64     `json:"total_filas_horarias"`
}

func main() {
	anio := flag.String("anio", "2025", "año de los archivos de la EPA")
	destino := flag.String("destino", "data/raw", "directorio donde quedan los CSV")
	rutaManifiesto := flag.String("manifiesto", "data/manifiesto.json", "archivo de manifiesto")
	verificar := flag.Bool("verificar", false, "solo comprobar los CSV locales contra el manifiesto, sin descargar")
	paralelas := flag.Int("paralelas", 3, "descargas simultáneas")
	flag.Parse()

	previo, _ := leerManifiesto(*rutaManifiesto)

	if *verificar {
		if previo == nil {
			log.Fatalf("no existe %s: primero ejecuta la descarga", *rutaManifiesto)
		}
		if err := verificarTodo(previo, *destino); err != nil {
			log.Fatal(err)
		}
		log.Print("todos los CSV coinciden con el manifiesto")
		return
	}

	if err := os.MkdirAll(*destino, 0o755); err != nil {
		log.Fatal(err)
	}

	esperado := map[string]Archivo{}
	if previo != nil && previo.Anio == *anio {
		for _, a := range previo.Archivos {
			esperado[a.Nombre] = a
		}
	}

	// Cada archivo se descarga en su propia goroutine; un semáforo con
	// buffer limita cuántas descargas corren a la vez para no saturar el
	// servidor de la EPA.
	var (
		mu        sync.Mutex
		resultado []Archivo
		errores   []error
		wg        sync.WaitGroup
		semaforo  = make(chan struct{}, max(1, *paralelas))
	)
	for _, p := range prefijos {
		wg.Add(1)
		go func(prefijo string) {
			defer wg.Done()
			semaforo <- struct{}{}
			defer func() { <-semaforo }()

			a, err := obtener(prefijo, *anio, *destino, esperado)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errores = append(errores, fmt.Errorf("%s: %w", prefijo, err))
				return
			}
			resultado = append(resultado, a)
		}(p)
	}
	wg.Wait()

	if len(errores) > 0 {
		log.Fatal(errors.Join(errores...))
	}

	sort.Slice(resultado, func(i, j int) bool { return resultado[i].Nombre < resultado[j].Nombre })
	m := Manifiesto{Anio: *anio, Generado: time.Now().UTC().Format(time.RFC3339), Archivos: resultado}
	for _, a := range resultado {
		if strings.HasPrefix(a.Nombre, "hourly") {
			m.TotalHora += a.Filas
		}
	}
	// Si los archivos no cambiaron, se conserva el manifiesto tal cual
	// (incluida su fecha), para no dejar cambios falsos en git.
	sinCambios := previo != nil && previo.Anio == m.Anio && reflect.DeepEqual(previo.Archivos, m.Archivos)
	if !sinCambios {
		if err := escribirManifiesto(*rutaManifiesto, &m); err != nil {
			log.Fatal(err)
		}
	}

	for _, a := range resultado {
		log.Printf("%-36s %12d filas  %6.1f MB", a.Nombre, a.Filas, float64(a.Bytes)/1e6)
	}
	log.Printf("total de filas horarias (4 gases): %d", m.TotalHora)
	if sinCambios {
		log.Printf("manifiesto %s sin cambios", *rutaManifiesto)
	} else {
		log.Printf("manifiesto escrito en %s", *rutaManifiesto)
	}
}

// obtener deja en destino el CSV de prefijo_anio y devuelve su entrada de
// manifiesto. Si el CSV local ya coincide con lo esperado, no descarga.
func obtener(prefijo, anio, destino string, esperado map[string]Archivo) (Archivo, error) {
	nombre := fmt.Sprintf("%s_%s.csv", prefijo, anio)
	url := fmt.Sprintf("%s/%s_%s.zip", urlBase, prefijo, anio)
	rutaCSV := filepath.Join(destino, nombre)

	if previo, ok := esperado[nombre]; ok {
		if info, err := os.Stat(rutaCSV); err == nil && info.Size() == previo.Bytes {
			actual, err := describir(rutaCSV)
			if err == nil && actual.SHA256 == previo.SHA256 {
				log.Printf("%s ya está y coincide con el manifiesto, se omite", nombre)
				actual.Nombre, actual.URL = nombre, url
				return actual, nil
			}
		}
	}

	rutaZip := rutaCSV + ".zip.part"
	defer os.Remove(rutaZip)
	if err := descargarConReintentos(url, rutaZip, 3); err != nil {
		return Archivo{}, err
	}
	if err := extraer(rutaZip, nombre, rutaCSV); err != nil {
		return Archivo{}, err
	}

	a, err := describir(rutaCSV)
	if err != nil {
		return Archivo{}, err
	}
	a.Nombre, a.URL = nombre, url
	return a, nil
}

func descargarConReintentos(url, ruta string, intentos int) error {
	var err error
	for i := 1; i <= intentos; i++ {
		log.Printf("descargando %s (intento %d/%d)", url, i, intentos)
		if err = descargar(url, ruta); err == nil {
			return nil
		}
		log.Printf("fallo: %v", err)
		time.Sleep(time.Duration(i*5) * time.Second)
	}
	return err
}

func descargar(url, ruta string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("respuesta HTTP %s", resp.Status)
	}

	f, err := os.Create(ruta)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// extraer copia del ZIP el CSV llamado nombre a rutaCSV, pasando por un
// archivo .part para no dejar nunca un CSV a medio escribir.
func extraer(rutaZip, nombre, rutaCSV string) error {
	z, err := zip.OpenReader(rutaZip)
	if err != nil {
		return err
	}
	defer z.Close()

	for _, entrada := range z.File {
		if filepath.Base(entrada.Name) != nombre {
			continue
		}
		origen, err := entrada.Open()
		if err != nil {
			return err
		}
		defer origen.Close()

		temporal := rutaCSV + ".part"
		f, err := os.Create(temporal)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, origen); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return os.Rename(temporal, rutaCSV)
	}
	return fmt.Errorf("el ZIP no contiene %s", nombre)
}

// describir recorre el CSV una sola vez calculando a la vez su SHA-256 y
// su número de filas de datos (líneas menos el encabezado).
func describir(ruta string) (Archivo, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return Archivo{}, err
	}
	defer f.Close()

	h := sha256.New()
	lector := bufio.NewReaderSize(io.TeeReader(f, h), 1<<20)
	var bytesTotales, lineas int64
	buf := make([]byte, 1<<20)
	ultimo := byte('\n')
	for {
		n, err := lector.Read(buf)
		if n > 0 {
			bytesTotales += int64(n)
			lineas += int64(bytes.Count(buf[:n], []byte{'\n'}))
			ultimo = buf[n-1]
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return Archivo{}, err
		}
	}
	if ultimo != '\n' {
		lineas++
	}

	return Archivo{Bytes: bytesTotales, SHA256: hex.EncodeToString(h.Sum(nil)), Filas: max(lineas-1, 0)}, nil
}

func verificarTodo(m *Manifiesto, destino string) error {
	var errs []error
	for _, esperado := range m.Archivos {
		actual, err := describir(filepath.Join(destino, esperado.Nombre))
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", esperado.Nombre, err))
		case actual.SHA256 != esperado.SHA256:
			errs = append(errs, fmt.Errorf("%s: SHA-256 distinto (la EPA pudo actualizar el archivo; vuelve a descargar)", esperado.Nombre))
		default:
			log.Printf("%-36s OK (%d filas)", esperado.Nombre, actual.Filas)
		}
	}
	return errors.Join(errs...)
}

func leerManifiesto(ruta string) (*Manifiesto, error) {
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return nil, err
	}
	var m Manifiesto
	if err := json.Unmarshal(datos, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func escribirManifiesto(ruta string, m *Manifiesto) error {
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	datos, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ruta, append(datos, '\n'), 0o644)
}
