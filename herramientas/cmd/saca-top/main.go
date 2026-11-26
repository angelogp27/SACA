// saca-top muestra en la terminal, en vivo, lo que gasta cada laptop del
// clúster: CPU total y por núcleo, RAM y, en las que tienen NVIDIA, la GPU.
// No usa SSH: cada trabajador le manda su uso al maestro cada 2 s y este
// programa lo pide por la API (a través del SACA de esta laptop).
//
//	./bin/saca-top                      # en cualquier laptop del clúster
//	./bin/saca-top -api http://192.168.50.10:8080   # directo al maestro
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"proyecto9/internal/distribuido"
)

const (
	reset    = "\x1b[0m"
	tenue    = "\x1b[2m"
	negrita  = "\x1b[1m"
	verde    = "\x1b[32m"
	amarillo = "\x1b[33m"
	rojo     = "\x1b[31m"
	cian     = "\x1b[36m"
)

func main() {
	api := flag.String("api", "http://127.0.0.1:8090", "SACA de esta laptop (puerto de la página) o la API del maestro (:8080)")
	cada := flag.Duration("cada", time.Second, "cada cuánto se redibuja")
	una := flag.Bool("una-vez", false, "imprime una sola vez, sin colores ni pantalla completa (para logs)")
	flag.Parse()
	url := strings.TrimRight(*api, "/") + "/api/uso"
	cliente := &http.Client{Timeout: 3 * time.Second}

	if *una {
		nodos, err := pedir(cliente, url)
		if err != nil {
			fmt.Fprintln(os.Stderr, "saca-top:", err)
			os.Exit(1)
		}
		fmt.Print(quitarColores(dibujar(nodos, *api, 100)))
		return
	}

	activarColores()
	salir := make(chan os.Signal, 1)
	signal.Notify(salir, os.Interrupt)
	fmt.Print("\x1b[?1049h\x1b[?25l") // pantalla aparte, sin cursor
	defer fmt.Print("\x1b[?25h\x1b[?1049l")
	t := time.NewTicker(*cada)
	defer t.Stop()
	for {
		nodos, err := pedir(cliente, url)
		var pantalla string
		if err != nil {
			pantalla = fmt.Sprintf("%ssaca-top%s · %s\n\n%s%v%s\n\n¿Está SACA abierto y esta laptop en un clúster? Ctrl+C para salir.\n", negrita, reset, *api, rojo, err, reset)
		} else {
			pantalla = dibujar(nodos, *api, anchoTerminal())
		}
		// Arriba a la izquierda, cada línea borrada hasta el final: sin parpadeo.
		fmt.Print("\x1b[H" + strings.ReplaceAll(pantalla, "\n", "\x1b[K\n") + "\x1b[J")
		select {
		case <-salir:
			return
		case <-t.C:
		}
	}
}

func pedir(c *http.Client, url string) ([]distribuido.UsoNodo, error) {
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return nil, fmt.Errorf("%s", e.Error)
	}
	var nodos []distribuido.UsoNodo
	return nodos, json.NewDecoder(resp.Body).Decode(&nodos)
}

func dibujar(nodos []distribuido.UsoNodo, api string, ancho int) string {
	ancho = max(ancho, 60)
	var b strings.Builder
	fmt.Fprintf(&b, "%sSACA top%s %s· %s · %d nodos · %s · Ctrl+C para salir%s\n", negrita, reset, tenue, api, len(nodos), time.Now().Format("15:04:05"), reset)
	if len(nodos) == 0 {
		b.WriteString("\nNo hay nodos en la sala.\n")
	}
	for _, n := range nodos {
		b.WriteString("\n")
		nodo(&b, n, ancho)
	}
	return b.String()
}

func nodo(b *strings.Builder, n distribuido.UsoNodo, ancho int) {
	rol := ""
	if n.Maestro {
		rol = cian + " [maestro]" + reset
	}
	equipo := ""
	if h := n.Hardware; h != nil {
		so, _, _ := strings.Cut(h.SO, "/")
		equipo = fmt.Sprintf("%s · %d hilos · %s", strings.TrimSpace(h.CPU), h.Hilos, so)
	}
	fmt.Fprintf(b, "%s%s%s%s  %s%s · %s%s\n", negrita, n.ID, reset, rol, tenue, n.Estado, equipo, reset)
	u := n.Uso
	switch {
	case u == nil:
		fmt.Fprintf(b, "  %ssin datos todavía (¿su SACA es de otra versión? git pull y volver a abrir)%s\n", amarillo, reset)
		return
	case n.HaceS > 6:
		fmt.Fprintf(b, "  %súltimo dato hace %.0f s%s\n", amarillo, n.HaceS, reset)
	}
	media := (ancho - 4) / 2
	cpu := barra("CPU", u.CPU, fmt.Sprintf("%5.1f%%", u.CPU), media)
	ram := barra("RAM", pct(u.RAMUsadaGB, u.RAMTotalGB), fmt.Sprintf("%.1f/%.1f GB", u.RAMUsadaGB, u.RAMTotalGB), media)
	fmt.Fprintf(b, "  %s  %s\n", cpu, ram)

	// Un medidor por núcleo lógico, en columnas, como htop.
	if len(u.Nucleos) > 0 {
		cols := max(1, min(4, (ancho-2)/22))
		filas := (len(u.Nucleos) + cols - 1) / cols
		anchoCol := (ancho - 2) / cols
		for f := range filas {
			b.WriteString("  ")
			for c := range cols {
				i := c*filas + f
				if i >= len(u.Nucleos) {
					break
				}
				b.WriteString(barra(fmt.Sprintf("%2d", i), u.Nucleos[i], fmt.Sprintf("%3.0f%%", u.Nucleos[i]), anchoCol-1) + " ")
			}
			b.WriteString("\n")
		}
	}
	if g := u.GPU; g != nil {
		detalle := fmt.Sprintf("%3.0f%% · %.0f/%.0f MB · %.0f °C", g.Uso, g.MemUsadaMB, g.MemTotalMB, g.TempC)
		if g.PotenciaW > 0 {
			detalle += fmt.Sprintf(" · %.0f W", g.PotenciaW)
		}
		fmt.Fprintf(b, "  %sGPU %s%s\n", tenue, g.Nombre, reset)
		fmt.Fprintf(b, "  %s\n", barra("GPU", g.Uso, detalle, ancho-2))
		fmt.Fprintf(b, "  %s\n", barra("VRAM", pct(g.MemUsadaMB, g.MemTotalMB), fmt.Sprintf("%.0f%%", pct(g.MemUsadaMB, g.MemTotalMB)), ancho-2))
	}
}

func pct(a, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return 100 * a / total
}

// barra: "CPU [|||||||        45.2%]" en exactamente `ancho` columnas.
func barra(nombre string, p float64, texto string, ancho int) string {
	dentro := max(ancho-len([]rune(nombre))-3, len([]rune(texto))+1)
	llenas := int(float64(dentro) * min(max(p, 0), 100) / 100)
	color := verde
	switch {
	case p >= 90:
		color = rojo
	case p >= 60:
		color = amarillo
	}
	vacias := dentro - llenas
	// El texto va a la derecha, encima de la barra.
	t := []rune(texto)
	celdas := []rune(strings.Repeat("|", llenas) + strings.Repeat(" ", vacias))
	copy(celdas[dentro-len(t):], t)
	corte := min(llenas, dentro-len(t))
	return fmt.Sprintf("%s%s%s [%s%s%s%s]", cian, nombre, reset, color, string(celdas[:corte]), reset, string(celdas[corte:]))
}

func quitarColores(s string) string {
	for _, c := range []string{reset, tenue, negrita, verde, amarillo, rojo, cian} {
		s = strings.ReplaceAll(s, c, "")
	}
	return s
}
