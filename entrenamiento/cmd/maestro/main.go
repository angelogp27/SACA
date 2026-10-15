// Maestro del clúster de entrenamiento (Parte B). Escucha en un puerto
// TCP y registra a los nodos trabajadores (rechaza a los que tienen otra
// muestra). En cada corrida le asigna a cada nodo una partición disjunta
// de las estaciones y entrena la regresión logística en modo síncrono: en
// cada época difunde los parámetros, espera el gradiente parcial de cada
// nodo, los suma y actualiza (docs/contrato_protocolo_tcp.md).
//
// Dos modos:
//   - De una corrida (por defecto): espera -nodos nodos, entrena una vez,
//     muestra una tabla por nodo y el desglose cálculo/red, verifica contra
//     el secuencial (-verificar) y agrega las filas al CSV (-resultado).
//   - Servidor (-web 127.0.0.1:8080): queda encendido con una sala de
//     espera; desde la página (frontend) se eligen los nodos, se lanzan
//     corridas y se ven en vivo. Los nodos vuelven a la sala al terminar.
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"proyecto9/internal/distribuido"
)

type configuracion struct {
	epocas    int
	tasa      float64
	verificar bool
	resultado string
	modo      string
	maquina   string
}

func main() {
	direccion := flag.String("escuchar", ":9000", "dirección TCP donde escucha el maestro")
	nodos := flag.Int("nodos", 2, "modo de una corrida: número de nodos trabajadores; el entrenamiento empieza cuando están todos")
	web := flag.String("web", "", "modo servidor: dirección HTTP de la API para la página, p. ej. 127.0.0.1:8080")
	capacidad := flag.Int("capacidad", 16, "modo servidor: máximo de nodos en la sala")
	rutaMuestra := flag.String("muestra", "data/muestra.gob", "muestra de entrenamiento; los nodos deben tener una copia idéntica")
	mesCorte := flag.Int("mes-corte", 10, "los meses >= mes-corte se reservan para evaluar (partición temporal)")
	tasa := flag.Float64("tasa", 0.1, "tasa de aprendizaje")
	epocas := flag.Int("epocas", 200, "número de épocas (en modo servidor, el valor inicial de la página)")
	espera := flag.Duration("espera", 2*time.Minute, "cuánto se espera el gradiente de un nodo en cada época antes de abortar")
	verificar := flag.Bool("verificar", true, "entrenar también en secuencial para comparar la pérdida y calcular el speedup")
	resultado := flag.String("resultado", "", "CSV al que se agregan las filas de cada corrida, p. ej. resultados/experimento2.csv")
	modo := flag.String("modo", "tcp_sincrono", "nombre del modo en el CSV (p. ej. tcp_local o tcp_tailscale)")
	maquina := flag.String("maquina", "", "texto libre que describe el clúster; en modo servidor se arma solo con los nodos de cada corrida")
	flag.Parse()

	if *nodos < 1 {
		log.Fatal("-nodos debe ser >= 1")
	}
	datos, huella, err := distribuido.PrepararLocal(*rutaMuestra, *mesCorte)
	if err != nil {
		log.Fatalf("no se pudo preparar la muestra %s: %v", *rutaMuestra, err)
	}
	log.Printf("muestra %s, huella %s (los nodos deben mostrar la misma)", *rutaMuestra, distribuido.Corta(huella))

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancelar()

	oyente, err := net.Listen("tcp", *direccion)
	if err != nil {
		log.Fatalf("no se pudo escuchar en %s: %v", *direccion, err)
	}
	defer oyente.Close()
	puerto := oyente.Addr().(*net.TCPAddr).Port

	cfgMaestro := distribuido.ConfigMaestro{Capacidad: *nodos, Datos: datos, Huella: huella, EsperaGradiente: *espera}
	var difusor *difusor
	if *web != "" {
		difusor = nuevoDifusor()
		cfgMaestro.Capacidad = *capacidad
		cfgMaestro.Notificar = difusor.Publicar
	}
	m := distribuido.NuevoMaestro(cfgMaestro)

	if *web != "" {
		log.Printf("maestro en modo servidor: nodos en %s (sala de hasta %d)", oyente.Addr(), *capacidad)
	} else {
		log.Printf("maestro escuchando en %s, esperando %d nodos", oyente.Addr(), *nodos)
	}
	for _, ip := range direccionesLocales() {
		log.Printf("  los nodos se conectan con: -maestro %s", net.JoinHostPort(ip, strconv.Itoa(puerto)))
	}
	go func() {
		for {
			conn, err := oyente.Accept()
			if err != nil {
				return // el oyente se cerró al terminar
			}
			go m.Atender(conn)
		}
	}()

	cfg := configuracion{epocas: *epocas, tasa: *tasa, verificar: *verificar, resultado: *resultado, modo: *modo, maquina: *maquina}
	if *web != "" {
		if err := servir(ctx, *web, m, difusor, cfg); err != nil {
			log.Fatal(err)
		}
		return
	}
	unaCorrida(ctx, m, cfg)
}

// unaCorrida es el modo clásico: espera los nodos, entrena una vez y sale.
func unaCorrida(ctx context.Context, m *distribuido.Maestro, cfg configuracion) {
	if err := m.EsperarNodos(ctx, m.Instantanea().Capacidad); err != nil {
		m.Despedir(distribuido.MotivoMaestroCerrando)
		log.Fatal("maestro detenido con Ctrl+C mientras esperaba nodos")
	}
	res, err := m.Correr(ctx, distribuido.OpcionesCorrida{Epocas: cfg.epocas, Tasa: cfg.tasa, Verificar: cfg.verificar, CerrarSala: true})
	if err != nil {
		m.Despedir(distribuido.MotivoMaestroCerrando)
		if errors.Is(err, context.Canceled) {
			log.Fatal("maestro detenido con Ctrl+C en ESTA ventana (no fue un nodo). Si querías simular la caída, el Ctrl+C se presiona en la ventana del nodo")
		}
		log.Fatalf("entrenamiento abortado: %v", err)
	}
	reportar(res, cfg.epocas)
	if cfg.resultado != "" {
		if err := agregarCSV(cfg.resultado, filasCSV(res, cfg.modo, cfg.maquina, cfg.epocas)); err != nil {
			log.Fatalf("no se pudo escribir %s: %v", cfg.resultado, err)
		}
		log.Printf("resultados agregados a %s", cfg.resultado)
	}
}

func reportar(res *distribuido.Resultado, epocas int) {
	por := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 / float64(epocas) }
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-14s %4s  %-5s %-36s %9s %12s  %s\n", "nodo", "part", "motor", "dispositivo", "ejemplos", "cálculo/ép.", "desde")
	for _, n := range res.Nodos {
		fmt.Fprintf(&b, "%-14s %4d  %-5s %-36s %9d %9.2f ms  %s\n", n.ID, n.Particion, n.Motor, recortar(n.Dispositivo, 36), n.N, por(n.TCalculo), n.Remoto)
	}
	log.Print(b.String())
	total := res.TCalculo + res.TComunicacion
	pctRed := 0.0
	if total > 0 {
		pctRed = 100 * float64(res.TComunicacion) / float64(total)
	}
	log.Printf("T(%d nodos) = %s, %.2f ms por época: cálculo del más lento %.2f ms + red y espera %.2f ms (%.0f%% del tiempo es comunicación)",
		len(res.Nodos), res.Duracion.Round(time.Millisecond), por(res.Duracion), por(res.TCalculo), por(res.TComunicacion), pctRed)
	log.Printf("evaluación en prueba (meses de evaluación): %s", res.Evaluacion)
	if v := res.Verificacion; v != nil {
		final, finalSec := res.Perdidas[len(res.Perdidas)-1], v.Perdidas[len(v.Perdidas)-1]
		veredicto := "OK, mismo modelo"
		if v.Diferencia > 1e-9 {
			veredicto = "DISTINTO: revisar"
		}
		log.Printf("verificación: pérdida final distribuida %.9f, secuencial %.9f, diferencia %.1e → %s", final, finalSec, v.Diferencia, veredicto)
		log.Printf("secuencial en esta PC: %s → speedup S(%d) = %.2f", v.Duracion.Round(time.Millisecond), len(res.Nodos), v.Duracion.Seconds()/res.Duracion.Seconds())
	}
}

func recortar(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

var encabezado = []string{"modo", "goroutines", "epocas", "t_total_s", "t_epoca_ms", "t_calculo_ms", "t_comunicacion_ms", "perdida_final", "f1", "auc", "maquina"}

// filasCSV arma la fila de la corrida distribuida y, si se verificó, la
// del secuencial (el T(1) del análisis).
func filasCSV(res *distribuido.Resultado, modo, maquina string, epocas int) [][]string {
	filas := [][]string{fila(modo, len(res.Nodos), epocas, res.Duracion, res.TCalculo, res.TComunicacion, res.Perdidas, res.Evaluacion.F1, res.Evaluacion.AUC, maquina)}
	if v := res.Verificacion; v != nil {
		filas = append(filas, fila("secuencial", 1, epocas, v.Duracion, v.Duracion, 0, v.Perdidas, v.Evaluacion.F1, v.Evaluacion.AUC, maquina))
	}
	return filas
}

// fila sigue las columnas de entrenador (modo, goroutines, ..., auc) más
// el desglose cálculo/comunicación y la máquina; "goroutines" es aquí el
// número de nodos, para que experimentos/cmd/analisis lo procese igual.
func fila(modo string, nodos, epocas int, total, calculo, comunicacion time.Duration, perdidas []float64, f1, auc float64, maquina string) []string {
	num := func(v float64, dec int) string { return strconv.FormatFloat(v, 'f', dec, 64) }
	porEpoca := func(d time.Duration) string { return num(float64(d.Microseconds())/1000/float64(epocas), 4) }
	return []string{
		modo, strconv.Itoa(nodos), strconv.Itoa(epocas),
		num(total.Seconds(), 4), porEpoca(total), porEpoca(calculo), porEpoca(comunicacion),
		num(perdidas[len(perdidas)-1], 6), num(f1, 4), num(auc, 4), maquina,
	}
}

func agregarCSV(ruta string, filas [][]string) error {
	if f, err := os.Open(ruta); err == nil {
		primera, errLeer := csv.NewReader(f).Read()
		f.Close()
		if errLeer == nil && strings.Join(primera, ",") != strings.Join(encabezado, ",") {
			return fmt.Errorf("%s tiene otras columnas (%s); usa otro archivo", ruta, strings.Join(primera, ","))
		}
	}
	_, errStat := os.Stat(ruta)
	nuevo := os.IsNotExist(errStat)
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if nuevo {
		w.Write(encabezado)
	}
	w.WriteAll(filas)
	return w.Error()
}

// direccionesLocales lista las IPv4 de esta PC (incluida la 100.x.y.z de
// Tailscale), para decirle a los integrantes adónde conectarse.
func direccionesLocales() []string {
	var ips []string
	direcciones, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, d := range direcciones {
		if red, ok := d.(*net.IPNet); ok && red.IP.To4() != nil && !red.IP.IsLoopback() && !red.IP.IsLinkLocalUnicast() {
			ips = append(ips, red.IP.String())
		}
	}
	return ips
}
