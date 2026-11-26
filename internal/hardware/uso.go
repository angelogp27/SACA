package hardware

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Uso es lo que gasta la máquina en este momento (para saca-top): CPU total
// y por núcleo lógico, RAM y, con NVIDIA, la GPU.
type Uso struct {
	CPU        float64   `json:"cpu"`               // % de todos los núcleos
	Nucleos    []float64 `json:"nucleos,omitempty"` // % de cada núcleo lógico
	RAMUsadaGB float64   `json:"ram_usada_gb"`
	RAMTotalGB float64   `json:"ram_total_gb"`
	GPU        *UsoGPU   `json:"gpu,omitempty"`
}

// UsoGPU: la primera GPU NVIDIA según nvidia-smi.
type UsoGPU struct {
	Nombre     string  `json:"nombre"`
	Uso        float64 `json:"uso"` // %
	MemUsadaMB float64 `json:"mem_usada_mb"`
	MemTotalMB float64 `json:"mem_total_mb"`
	TempC      float64 `json:"temp_c"`
	PotenciaW  float64 `json:"potencia_w"`
}

// tics de un núcleo: tiempo ocupado y total, en cualquier unidad.
type tics struct{ ocupado, total uint64 }

// Muestreador da el Uso entre dos lecturas.
type Muestreador struct {
	antes  []tics
	sinGPU bool // nvidia-smi falló una vez: no se vuelve a lanzar
}

func NuevoMuestreador() *Muestreador {
	m := &Muestreador{sinGPU: runtime.GOOS != "linux" && runtime.GOOS != "windows"}
	m.antes = ticsNucleos()
	return m
}

// Leer: la CPU desde la lectura anterior; la RAM y la GPU, de ahora.
func (m *Muestreador) Leer() Uso {
	var u Uso
	ahora := ticsNucleos()
	if len(ahora) == len(m.antes) && len(ahora) > 0 {
		u.Nucleos = make([]float64, len(ahora))
		var ocupado, total uint64
		for i, t := range ahora {
			do, dt := t.ocupado-m.antes[i].ocupado, t.total-m.antes[i].total
			if t.ocupado < m.antes[i].ocupado || t.total < m.antes[i].total {
				do, dt = 0, 0
			}
			if dt > 0 {
				u.Nucleos[i] = min(100, 100*float64(do)/float64(dt))
			}
			ocupado += do
			total += dt
		}
		if total > 0 {
			u.CPU = min(100, 100*float64(ocupado)/float64(total))
		}
	}
	m.antes = ahora
	u.RAMUsadaGB, u.RAMTotalGB = ramUso()
	if !m.sinGPU {
		g, err := leerGPU()
		if err != nil {
			m.sinGPU = true
		}
		u.GPU = g
	}
	return u
}

// ticsLinux lee /proc/stat: una línea "cpuN" por núcleo lógico.
func ticsLinux(ruta string) []tics {
	f, err := os.Open(ruta)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lista []tics
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		campos := strings.Fields(sc.Text())
		if len(campos) < 5 || !strings.HasPrefix(campos[0], "cpu") || campos[0] == "cpu" {
			continue
		}
		var t tics
		for i, c := range campos[1:] {
			v, _ := strconv.ParseUint(c, 10, 64)
			if i >= 8 { // guest y guest_nice ya están dentro de user y nice
				break
			}
			t.total += v
			if i != 3 && i != 4 { // idle e iowait
				t.ocupado += v
			}
		}
		lista = append(lista, t)
	}
	return lista
}

// ramLinux: total y usada (total - disponible), en GB.
func ramLinux(ruta string) (usada, total float64) {
	f, err := os.Open(ruta)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	var disponible float64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		campos := strings.Fields(sc.Text())
		if len(campos) < 2 {
			continue
		}
		kb, _ := strconv.ParseFloat(campos[1], 64)
		switch campos[0] {
		case "MemTotal:":
			total = kb / (1 << 20)
		case "MemAvailable:":
			disponible = kb / (1 << 20)
		}
	}
	return max(total-disponible, 0), total
}

func leerGPU() (*UsoGPU, error) {
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelar()
	salida, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil, err
	}
	return parsearUsoGPU(string(salida)), nil
}

func parsearUsoGPU(salida string) *UsoGPU {
	linea, _, _ := strings.Cut(strings.TrimSpace(salida), "\n")
	c := strings.Split(linea, ",")
	if len(c) < 6 {
		return nil
	}
	num := func(s string) float64 {
		v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64) // "[N/A]" queda en 0
		return v
	}
	return &UsoGPU{Nombre: strings.TrimSpace(c[0]), Uso: num(c[1]), MemUsadaMB: num(c[2]), MemTotalMB: num(c[3]), TempC: num(c[4]), PotenciaW: num(c[5])}
}
