// Package limpieza ejecuta en un nodo la limpieza de sus CSV con una
// tecnología (Pthreads o Go) y genera su data/muestra.bin. Lanza el
// cargador compilado como proceso hijo con --bin y --progreso, lee su
// avance de la salida estándar y mide el uso de CPU del proceso desde
// afuera (cpu_*.go), igual en Linux y en Windows.
package limpieza

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"proyecto9/internal/aqs"
	"proyecto9/internal/hardware"
)

// Tecnologías disponibles, en el orden en que se ejecutan.
const (
	Pthreads = "pthreads"
	Go       = "go"
)

// Config indica dónde están los datos y los cargadores compilados.
type Config struct {
	DirDatos string // data/raw
	Anio     string // 2025
	RutaBin  string // data/muestra.bin
	DirBin   string // bin (ejecutables)
}

// PorDefecto son las rutas de siempre, relativas a la raíz del proyecto.
func PorDefecto() Config {
	return Config{DirDatos: filepath.Join("data", "raw"), Anio: "2025", RutaBin: filepath.Join("data", "muestra.bin"), DirBin: "bin"}
}

// Fases de una tecnología: primero se limpian los CSV y después se
// preparan las variables (rezagos, etiqueta, normalización) y se escribe
// muestra.bin. Las dos corren en el mismo proceso; el cargador imprime
// "SACA_FASE variables" al pasar de una a otra.
const (
	FaseLimpieza  = "limpieza"
	FaseVariables = "variables"
)

// Avance es una muestra de la gráfica: en qué fase está, cuánto lleva de
// esa fase (0-1) y cuánta CPU usa el proceso (% de todos los núcleos
// lógicos del nodo).
type Avance struct {
	Fase   string
	Avance float64
	CPU    float64
	// TLimpieza: cuánto tardó la limpieza; se conoce desde que empieza la
	// fase de variables.
	TLimpieza time.Duration
}

// Resultado de una tecnología.
type Resultado struct {
	Tecnologia string
	Hilos      int
	Duracion   time.Duration // total
	Limpieza   time.Duration // leer, validar y unir los CSV
	Variables  time.Duration // preparar variables y escribir muestra.bin
	Huella     string
}

// Ejecutable devuelve la ruta del cargador de una tecnología.
func (c Config) Ejecutable(tecnologia string) (string, error) {
	nombre := map[string]string{Pthreads: "cargador_pthreads", Go: "cargador_concurrente"}[tecnologia]
	if nombre == "" {
		return "", fmt.Errorf("tecnología desconocida %q (usa %s o %s)", tecnologia, Pthreads, Go)
	}
	if runtime.GOOS == "windows" {
		nombre += ".exe"
	}
	return filepath.Join(c.DirBin, nombre), nil
}

// Comprobar revisa que estén los CSV y el ejecutable antes de empezar.
func (c Config) Comprobar(tecnologia string) error {
	exe, err := c.Ejecutable(tecnologia)
	if err != nil {
		return err
	}
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("falta %s: compílalo primero (scripts/saca.sh o scripts/saca.ps1)", exe)
	}
	for _, ruta := range aqs.Archivos(c.DirDatos, c.Anio) {
		if _, err := os.Stat(ruta); err != nil {
			return fmt.Errorf("falta el CSV %s", ruta)
		}
	}
	return nil
}

// Ejecutar corre una tecnología hasta que termina o se cancela ctx (en ese
// caso mata al proceso y borra el .bin a medias). hilos <= 0 usa todos los
// núcleos lógicos. alAvanzar se llama cada ~250 ms desde otra goroutine.
func (c Config) Ejecutar(ctx context.Context, tecnologia string, hilos int, alAvanzar func(Avance)) (*Resultado, error) {
	if err := c.Comprobar(tecnologia); err != nil {
		return nil, err
	}
	if hilos <= 0 {
		hilos = runtime.NumCPU()
	}
	exe, _ := c.Ejecutable(tecnologia)
	var args []string
	switch tecnologia {
	case Pthreads:
		args = []string{"--datos", c.DirDatos, "--anio", c.Anio, "--hilos", strconv.Itoa(hilos),
			"--agregadoras", strconv.Itoa(max(1, hilos/2)), "--bin", c.RutaBin, "--progreso"}
	case Go:
		args = []string{"-datos", c.DirDatos, "-anio", c.Anio, "-goroutines", strconv.Itoa(hilos),
			"-agregadoras", "0", "-sin-mongo", "-bin", c.RutaBin, "-progreso"}
	}
	cmd := exec.Command(exe, args...)
	if tecnologia == Go && os.Getenv("GOMEMLIMIT") == "" {
		// Sin límite, el recolector de Go deja crecer el proceso hasta ~8 GB
		// (el doble de lo vivo). Con la mitad de la RAM (entre 3 y 6 GiB) se
		// queda en ~4,5 GB a cambio de ~15 % más de tiempo, y cabe en una
		// laptop de 8 GB.
		gib := 4.0
		if ram := hardware.Detectar().RAMGB; ram > 0 {
			gib = min(6, max(3, ram/2))
		}
		cmd.Env = append(os.Environ(), fmt.Sprintf("GOMEMLIMIT=%dMiB", int(gib*1024)))
	}
	salida, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var errores strings.Builder
	cmd.Stderr = &limitado{w: &errores, n: 8192}

	inicio := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("no se pudo iniciar %s: %w", exe, err)
	}
	medidor := nuevoMedidorCPU(cmd.Process.Pid)
	defer medidor.cerrar()

	// Cancelar: matar al proceso desbloquea la lectura de su salida.
	terminado := make(chan struct{})
	defer close(terminado)
	go func() {
		select {
		case <-ctx.Done():
			cmd.Process.Kill()
		case <-terminado:
		}
	}()

	nucleos := float64(runtime.NumCPU())
	cpuAntes, tAntes, pct := 0.0, time.Now(), 0.0
	fase, finLimpieza := FaseLimpieza, time.Duration(0)
	lector := bufio.NewScanner(salida)
	for lector.Scan() {
		linea := lector.Text()
		if strings.HasPrefix(linea, "SACA_FASE variables") {
			fase, finLimpieza = FaseVariables, time.Since(inicio)
			if alAvanzar != nil {
				alAvanzar(Avance{Fase: FaseLimpieza, Avance: 1, CPU: pct, TLimpieza: finLimpieza})
			}
			continue
		}
		valor, ok := strings.CutPrefix(linea, "SACA_PROGRESO ")
		if !ok {
			continue
		}
		av, err := strconv.ParseFloat(strings.TrimSpace(valor), 64)
		if err != nil {
			continue
		}
		// Si no se pudo medir (el proceso ya estaba saliendo), se repite el
		// último valor en vez de dibujar una caída a 0 %.
		// (Un proceso zombi informa 0 s de CPU: también se descarta.)
		if cpu, err := medidor.segundos(); err == nil && cpu >= cpuAntes {
			ahora := time.Now()
			if dt := ahora.Sub(tAntes).Seconds(); dt > 0 {
				pct = min(100, max(0, 100*(cpu-cpuAntes)/(dt*nucleos)))
			}
			cpuAntes, tAntes = cpu, ahora
		}
		// El cargador informa un avance global: la lectura llega a 0,8 y la
		// preparación de variables, de 0,8 a 1. Aquí se pasa a 0-1 por fase.
		if fase == FaseLimpieza {
			av = min(av/0.8, 1)
		} else {
			av = max(0, (av-0.8)/0.2)
		}
		av = math.Round(av*1e4) / 1e4 // (1-0,8)/0,2 no da 1 exacto en punto flotante
		if alAvanzar != nil {
			alAvanzar(Avance{Fase: fase, Avance: av, CPU: pct, TLimpieza: finLimpieza})
		}
	}
	io.Copy(io.Discard, salida)
	errEspera := cmd.Wait()
	duracion := time.Since(inicio)

	if ctx.Err() != nil {
		os.Remove(c.RutaBin + ".tmp")
		return nil, ctx.Err()
	}
	if errEspera != nil {
		return nil, fmt.Errorf("%s terminó con error: %v: %s", tecnologia, errEspera, ultimaLinea(errores.String()))
	}
	huella, err := Huella(c.RutaBin)
	if err != nil {
		return nil, fmt.Errorf("%s no dejó %s: %w", tecnologia, c.RutaBin, err)
	}
	if finLimpieza == 0 {
		finLimpieza = duracion // un cargador viejo, sin el marcador de fase
	}
	return &Resultado{Tecnologia: tecnologia, Hilos: hilos, Duracion: duracion, Limpieza: finLimpieza, Variables: duracion - finLimpieza, Huella: huella}, nil
}

// HuellaActual es la huella del .bin del nodo, o "" si todavía no tiene.
func (c Config) HuellaActual() string {
	h, err := Huella(c.RutaBin)
	if err != nil {
		return ""
	}
	return h
}

// ErrCPUNoDisponible: en este sistema no se sabe medir la CPU de otro
// proceso; la gráfica muestra solo el avance.
var ErrCPUNoDisponible = errors.New("medición de CPU no disponible en este sistema")

func ultimaLinea(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// limitado guarda solo los primeros n bytes (el error del cargador cabe;
// un log enorme no se acumula en memoria).
type limitado struct {
	w io.Writer
	n int
}

func (l *limitado) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}

// Huella es el SHA-256 del archivo, en hexadecimal (la misma que calcula
// distribuido.Huella; se repite aquí para no importar ese paquete, que usa
// a este).
func Huella(ruta string) (string, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
