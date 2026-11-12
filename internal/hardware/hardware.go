// Package hardware describe la máquina en la que corre un nodo: CPU,
// núcleos, memoria y GPU. Cada trabajador la envía al registrarse y el
// maestro la guarda junto a cada corrida, para poder comprobar (y mostrar)
// en qué equipos se midió de verdad.
package hardware

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Info es la huella del hardware de un nodo.
type Info struct {
	Host    string  `json:"host"`
	SO      string  `json:"so"`
	CPU     string  `json:"cpu"`
	Hilos   int     `json:"hilos"`             // núcleos lógicos
	Nucleos int     `json:"nucleos,omitempty"` // núcleos físicos, 0 si no se sabe
	RAMGB   float64 `json:"ram_gb,omitempty"`
	GPU     string  `json:"gpu,omitempty"`    // modelo y memoria, vacío si no hay NVIDIA
	Driver  string  `json:"driver,omitempty"` // versión del driver NVIDIA
}

// Detectar lee el hardware de esta máquina. Lo que no se puede averiguar
// queda vacío o en cero: nunca falla.
func Detectar() Info {
	host, _ := os.Hostname()
	info := Info{Host: host, SO: runtime.GOOS + "/" + runtime.GOARCH, Hilos: runtime.NumCPU()}
	switch runtime.GOOS {
	case "linux":
		cpu, nucleos := leerCPUInfo("/proc/cpuinfo")
		info.CPU, info.Nucleos = cpu, nucleos
		info.RAMGB = leerMemInfo("/proc/meminfo")
	case "windows":
		info.CPU = os.Getenv("PROCESSOR_IDENTIFIER")
		info.RAMGB = ramWindows()
	}
	info.GPU, info.Driver = detectarNVIDIA()
	return info
}

// Resumen es una línea corta para logs, tablas y la columna "maquina" de
// los CSV, p. ej. "AMD Ryzen 5 3600 · 6N/12H · 16 GB · NVIDIA RTX 3060, 12288 MiB (driver 560.35)".
func (i Info) Resumen() string {
	partes := []string{}
	if i.CPU != "" {
		partes = append(partes, i.CPU)
	}
	if i.Nucleos > 0 {
		partes = append(partes, fmt.Sprintf("%dN/%dH", i.Nucleos, i.Hilos))
	} else {
		partes = append(partes, fmt.Sprintf("%dH", i.Hilos))
	}
	if i.RAMGB > 0 {
		partes = append(partes, fmt.Sprintf("%.0f GB", i.RAMGB))
	}
	if i.GPU != "" {
		gpu := i.GPU
		if i.Driver != "" {
			gpu += " (driver " + i.Driver + ")"
		}
		partes = append(partes, gpu)
	}
	return strings.Join(partes, " · ")
}

// leerCPUInfo devuelve el modelo y los núcleos físicos (pares únicos de
// "physical id" y "core id") de un /proc/cpuinfo.
func leerCPUInfo(ruta string) (modelo string, nucleos int) {
	f, err := os.Open(ruta)
	if err != nil {
		return "", 0
	}
	defer f.Close()
	fisicos := map[string]bool{}
	var paquete string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		clave, valor, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		clave, valor = strings.TrimSpace(clave), strings.TrimSpace(valor)
		switch clave {
		case "model name":
			if modelo == "" {
				modelo = valor
			}
		case "physical id":
			paquete = valor
		case "core id":
			fisicos[paquete+"/"+valor] = true
		}
	}
	return modelo, len(fisicos)
}

// leerMemInfo devuelve la RAM total, en GB, de un /proc/meminfo.
func leerMemInfo(ruta string) float64 {
	f, err := os.Open(ruta)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if campos := strings.Fields(sc.Text()); len(campos) >= 2 && campos[0] == "MemTotal:" {
			kb, err := strconv.ParseFloat(campos[1], 64)
			if err != nil {
				return 0
			}
			return kb / 1024 / 1024
		}
	}
	return 0
}

// detectarNVIDIA pregunta a nvidia-smi por la primera GPU. Sin NVIDIA (o
// sin driver) devuelve vacío.
func detectarNVIDIA() (gpu, driver string) {
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	salida, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total,driver_version", "--format=csv,noheader").Output()
	if err != nil {
		return "", ""
	}
	return parsearNVIDIA(string(salida))
}

func parsearNVIDIA(salida string) (gpu, driver string) {
	linea, _, _ := strings.Cut(strings.TrimSpace(salida), "\n")
	campos := strings.Split(linea, ",")
	if len(campos) < 3 {
		return "", ""
	}
	nombre, memoria, driver := strings.TrimSpace(campos[0]), strings.TrimSpace(campos[1]), strings.TrimSpace(campos[2])
	return nombre + ", " + memoria, driver
}
