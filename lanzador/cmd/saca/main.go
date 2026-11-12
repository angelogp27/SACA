// Lanzador local de SACA: lo que corre cada integrante en su laptop (Linux
// o Windows) para usar la página. Sirve el frontend compilado en
// http://127.0.0.1:8090 y le da los botones de la primera pantalla:
//
//   - Iniciar clúster: lanza el maestro (bin/maestro -web) y un nodo
//     trabajador en esta misma laptop (el maestro también participa).
//     Se niega si otra laptop de la red ya inició uno.
//   - Unirse al clúster: lanza el trabajador conectado al maestro que se
//     anuncia en la red (o a la IP que se escriba).
//
// Además reenvía /api/* a la API del maestro (en esta laptop o en la del
// maestro), así todas las laptops ven el clúster en vivo.
//
//	go build -o bin/saca ./lanzador/cmd/saca && ./bin/saca
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"proyecto9/internal/aqs"
	"proyecto9/internal/distribuido"
	"proyecto9/internal/hardware"
)

// maxLaptops es el tamaño máximo del clúster, contando la del maestro.
const maxLaptops = 8

type config struct {
	puertoUI, puertoWeb, puertoTCP, puertoUDP, puertoPares int
	dirFrontend, dirBin, rutaMuestra                       string
	dirDatos, resultado                                    string
	datosPagina                                            string
}

type lanzador struct {
	cfg  config
	desc *descubridor
	host string

	mu         sync.Mutex
	rol        string // "", "maestro" o "nodo"
	id         string
	maestro    *proceso
	trabajador *proceso
	destino    string // ip:puerto TCP del maestro
	webMaestro string // URL de su API
	pararAviso chan struct{}
	ultimoErr  string
}

func main() {
	var c config
	flag.IntVar(&c.puertoUI, "puerto", 8090, "puerto local de la página")
	flag.IntVar(&c.puertoWeb, "puerto-web", 8080, "puerto de la API del maestro (debe estar abierto en el firewall de la laptop del maestro)")
	flag.IntVar(&c.puertoTCP, "puerto-tcp", 9000, "puerto TCP del protocolo maestro-trabajador")
	flag.IntVar(&c.puertoPares, "puerto-pares", 9002, "puerto TCP donde cada nodo escucha a los otros nodos (comunicación en árbol o anillo)")
	flag.IntVar(&c.puertoUDP, "puerto-anuncios", 9001, "puerto UDP de los anuncios del clúster en la LAN")
	flag.StringVar(&c.dirFrontend, "frontend", filepath.Join("frontend", "dist"), "frontend compilado (cd frontend; npm run build)")
	flag.StringVar(&c.dirBin, "bin", "bin", "directorio de los ejecutables compilados")
	flag.StringVar(&c.rutaMuestra, "muestra", filepath.Join("data", "muestra.bin"), "muestra de entrenamiento de esta laptop")
	flag.StringVar(&c.dirDatos, "datos", filepath.Join("data", "raw"), "CSV de la EPA de esta laptop")
	flag.StringVar(&c.resultado, "resultado", filepath.Join("resultados", "cluster.csv"), "CSV de resultados del entrenamiento (en la laptop del maestro)")
	flag.StringVar(&c.datosPagina, "datos-pagina", "", "dónde el maestro deja los JSON del mapa regenerados (separados por comas; vacío = frontend/public/datos y los del frontend compilado)")
	abrir := flag.Bool("abrir", true, "abrir la página en el navegador")
	flag.Parse()

	host, _ := os.Hostname()
	l := &lanzador{cfg: c, host: host, id: host, desc: nuevoDescubridor(c.puertoUDP, instancia())}
	go l.desc.escuchar()

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancelar()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /local/estado", l.estado)
	mux.HandleFunc("POST /local/iniciar", l.iniciar)
	mux.HandleFunc("POST /local/unirse", l.unirse)
	mux.HandleFunc("POST /local/detener", l.detenerHTTP)
	mux.Handle("/api/", http.HandlerFunc(l.proxy))
	mux.Handle("/", spa(c.dirFrontend))

	// Solo en 127.0.0.1: los botones lanzan procesos en esta laptop.
	oyente, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", itoa(c.puertoUI)))
	if err != nil {
		log.Fatalf("no se pudo abrir la página en el puerto %d (¿ya hay un lanzador abierto?): %v", c.puertoUI, err)
	}
	servidor := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		log.Print("cerrando: se detienen el maestro y el trabajador de esta laptop")
		l.detener()
		apagar, listo := context.WithTimeout(context.Background(), 2*time.Second)
		defer listo()
		servidor.Shutdown(apagar)
	}()
	direccion := "http://" + oyente.Addr().String()
	log.Printf("SACA en %s (Ctrl+C para salir)", direccion)
	if _, err := os.Stat(filepath.Join(c.dirFrontend, "index.html")); err != nil {
		log.Printf("AVISO: falta %s: compila el frontend (cd frontend; npm run build)", filepath.Join(c.dirFrontend, "index.html"))
	}
	if *abrir {
		abrirNavegador(direccion)
	}
	if err := servidor.Serve(oyente); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// ---------- estado ----------

type estadoLocal struct {
	Rol        string          `json:"rol"`
	ID         string          `json:"id"`
	Host       string          `json:"host"`
	Maestro    string          `json:"maestro,omitempty"`
	Clusters   []ClusterVisto  `json:"clusters"`
	Hardware   hardware.Info   `json:"hardware"`
	Huella     string          `json:"huella"`
	IPs        []string        `json:"ips"` // IPv4 de esta laptop (cable, Wi-Fi, Tailscale)
	IP         string          `json:"ip"`  // la que deben usar los demás (ver ipPrincipal)
	Listo      map[string]bool `json:"listo"`
	Error      string          `json:"error,omitempty"`
	LogMaestro []string        `json:"log_maestro,omitempty"`
	LogNodo    []string        `json:"log_nodo,omitempty"`
}

var hw = sync.OnceValue(hardware.Detectar)

func (l *lanzador) estado(w http.ResponseWriter, r *http.Request) {
	l.revisar()
	l.mu.Lock()
	e := estadoLocal{Rol: l.rol, ID: l.id, Host: l.host, Maestro: l.destino, Error: l.ultimoErr, Hardware: hw()}
	if l.maestro != nil {
		e.LogMaestro = l.maestro.ultimas(20)
	}
	if l.trabajador != nil {
		e.LogNodo = l.trabajador.ultimas(20)
	}
	l.mu.Unlock()
	e.IPs = ipsLocales()
	e.IP = ipPrincipal(e.IPs)
	e.Clusters = l.desc.Vistos()
	if e.Clusters == nil {
		e.Clusters = []ClusterVisto{}
	}
	e.Huella = l.huellaMuestra()
	e.Listo = map[string]bool{
		"maestro":           existe(ejecutable(l.cfg.dirBin, "maestro")),
		"trabajador":        existe(ejecutable(l.cfg.dirBin, "trabajador")),
		"cargador_pthreads": existe(ejecutable(l.cfg.dirBin, "cargador_pthreads")),
		"cargador_go":       existe(ejecutable(l.cfg.dirBin, "cargador_concurrente")),
		"csv":               true,
		"muestra":           e.Huella != "",
	}
	for _, ruta := range aqs.Archivos(l.cfg.dirDatos, "2025") {
		if !existe(ruta) {
			e.Listo["csv"] = false
		}
	}
	responder(w, http.StatusOK, e)
}

// revisar detecta si el maestro o el trabajador terminaron solos.
func (l *lanzador) revisar() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rol == "maestro" && !l.maestro.vivo() {
		l.ultimoErr = "el maestro se cerró: " + strings.Join(l.maestro.ultimas(3), " | ")
		l.limpiarLocked()
	}
	if l.rol == "nodo" && !l.trabajador.vivo() {
		l.ultimoErr = "el nodo se cerró: " + strings.Join(l.trabajador.ultimas(3), " | ")
		l.limpiarLocked()
	}
}

// ---------- iniciar / unirse / detener ----------

type pedido struct {
	ID      string `json:"id"`
	Maestro string `json:"maestro"` // unirse: ip o ip:puerto; vacío = el detectado
	Nodo    *bool  `json:"participar"`
}

func (l *lanzador) iniciar(w http.ResponseWriter, r *http.Request) {
	var p pedido
	json.NewDecoder(r.Body).Decode(&p)
	if vistos := l.desc.Vistos(); len(vistos) > 0 {
		fallo(w, http.StatusConflict, fmt.Errorf("ya hay un clúster iniciado por %s (%s): únete a él", vistos[0].Host, vistos[0].IP))
		return
	}
	l.mu.Lock()
	if l.rol != "" {
		l.mu.Unlock()
		fallo(w, http.StatusConflict, fmt.Errorf("esta laptop ya está en el clúster como %s", l.rol))
		return
	}
	l.rol, l.ultimoErr = "maestro", ""
	if p.ID != "" {
		l.id = p.ID
	}
	id := l.id
	l.mu.Unlock()

	err := l.arrancarMaestro(r.Context(), id, p.Nodo == nil || *p.Nodo)
	if err != nil {
		l.detener()
		l.mu.Lock()
		l.ultimoErr = err.Error()
		l.mu.Unlock()
		fallo(w, http.StatusInternalServerError, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"ok": true})
}

func (l *lanzador) arrancarMaestro(ctx context.Context, id string, participar bool) error {
	exe := ejecutable(l.cfg.dirBin, "maestro")
	if !existe(exe) {
		return fmt.Errorf("falta %s: compila con scripts/saca.sh (Linux) o scripts/saca.ps1 (Windows)", exe)
	}
	// Máximo 8 laptops contando la del maestro: si el maestro no participa
	// como nodo, la sala admite 7.
	capacidad := maxLaptops
	if !participar {
		capacidad--
	}
	args := []string{"-capacidad", itoa(capacidad), "-web", net.JoinHostPort("0.0.0.0", itoa(l.cfg.puertoWeb)), "-escuchar", ":" + itoa(l.cfg.puertoTCP),
		"-muestra", l.cfg.rutaMuestra, "-modo", "tcp_lan", "-resultado", l.cfg.resultado,
		// Al terminar cada entrenamiento: guardar el modelo y regenerar el
		// mapa (exportar usa MongoDB; si no está, el mapa queda como estaba).
		"-modelo", filepath.Join("data", "modelo.json"), "-exportar", ejecutable(l.cfg.dirBin, "exportar"),
		"-datos-pagina", l.datosPagina()}
	if participar {
		args = append(args, "-nodo-local", id)
	}
	m, err := lanzar("maestro", exe, args...)
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.maestro = m
	l.destino = net.JoinHostPort("127.0.0.1", itoa(l.cfg.puertoTCP))
	l.webMaestro = "http://" + net.JoinHostPort("127.0.0.1", itoa(l.cfg.puertoWeb))
	l.mu.Unlock()

	// La API responde cuando el maestro ya cargó su muestra (o vio que no
	// tiene): puede tardar unos segundos.
	limite := time.Now().Add(90 * time.Second)
	for {
		if !m.vivo() {
			return fmt.Errorf("el maestro no arrancó: %s", strings.Join(m.ultimas(4), " | "))
		}
		if resp, err := http.Get(l.webMaestro + "/api/estado"); err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(limite) {
			return errors.New("el maestro no respondió en 90 s")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	parar := make(chan struct{})
	l.mu.Lock()
	l.pararAviso = parar
	l.mu.Unlock()
	go l.desc.anunciar(l.host, l.cfg.puertoTCP, l.cfg.puertoWeb, parar)

	if participar {
		return l.arrancarTrabajador(id, net.JoinHostPort("127.0.0.1", itoa(l.cfg.puertoTCP)))
	}
	return nil
}

func (l *lanzador) arrancarTrabajador(id, destino string) error {
	exe := ejecutable(l.cfg.dirBin, "trabajador")
	if !existe(exe) {
		return fmt.Errorf("falta %s: compila con scripts/saca.sh (Linux) o scripts/saca.ps1 (Windows)", exe)
	}
	t, err := lanzar("nodo", exe, "-maestro", destino, "-id", id, "-muestra", l.cfg.rutaMuestra, "-datos", l.cfg.dirDatos, "-reconectar",
		"-pares", ":"+itoa(l.cfg.puertoPares))
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.trabajador = t
	l.mu.Unlock()
	return nil
}

func (l *lanzador) unirse(w http.ResponseWriter, r *http.Request) {
	var p pedido
	json.NewDecoder(r.Body).Decode(&p)
	l.mu.Lock()
	rol := l.rol
	l.mu.Unlock()
	if rol != "" {
		fallo(w, http.StatusConflict, fmt.Errorf("esta laptop ya está en el clúster como %s", rol))
		return
	}
	destino, web, err := l.elegirMaestro(p.Maestro)
	if err != nil {
		fallo(w, http.StatusBadRequest, err)
		return
	}
	l.mu.Lock()
	if l.rol != "" {
		l.mu.Unlock()
		fallo(w, http.StatusConflict, fmt.Errorf("esta laptop ya está en el clúster como %s", l.rol))
		return
	}
	l.rol, l.ultimoErr, l.destino, l.webMaestro = "nodo", "", destino, web
	if p.ID != "" {
		l.id = p.ID
	}
	id := l.id
	l.mu.Unlock()

	// Antes de lanzar el nodo se prueba la conexión: el error más común es
	// el firewall de la laptop del maestro.
	conn, err := net.DialTimeout("tcp", destino, 3*time.Second)
	if err != nil {
		l.detener()
		fallo(w, http.StatusBadGateway, fmt.Errorf("no se pudo conectar con el maestro %s (%v): revisa que esté encendido y que su firewall permita el puerto %d", destino, err, l.cfg.puertoTCP))
		return
	}
	conn.Close()
	if err := l.arrancarTrabajador(id, destino); err != nil {
		l.detener()
		fallo(w, http.StatusInternalServerError, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"ok": true})
}

// elegirMaestro: la dirección escrita, o el único clúster anunciado.
func (l *lanzador) elegirMaestro(escrito string) (destino, web string, err error) {
	escrito = strings.TrimSpace(escrito)
	if escrito == "" {
		vistos := l.desc.Vistos()
		switch len(vistos) {
		case 0:
			return "", "", errors.New("no se detectó ningún clúster en la red: escribe la IP del maestro")
		case 1:
			v := vistos[0]
			return net.JoinHostPort(v.IP, itoa(v.TCP)), "http://" + net.JoinHostPort(v.IP, itoa(v.Web)), nil
		default:
			return "", "", fmt.Errorf("hay %d clústeres en la red: elige uno escribiendo su IP", len(vistos))
		}
	}
	host, puerto, err := net.SplitHostPort(escrito)
	if err != nil {
		host, puerto = escrito, itoa(l.cfg.puertoTCP)
	}
	if net.ParseIP(host) == nil {
		if _, err := net.LookupHost(host); err != nil {
			return "", "", fmt.Errorf("dirección inválida %q", escrito)
		}
	}
	web = "http://" + net.JoinHostPort(host, itoa(l.cfg.puertoWeb))
	for _, v := range l.desc.Vistos() {
		if v.IP == host {
			web = "http://" + net.JoinHostPort(v.IP, itoa(v.Web))
		}
	}
	return net.JoinHostPort(host, puerto), web, nil
}

func (l *lanzador) detenerHTTP(w http.ResponseWriter, r *http.Request) {
	l.detener()
	responder(w, http.StatusOK, map[string]any{"ok": true})
}

// detener cierra el trabajador y el maestro de esta laptop (en ese orden:
// el trabajador se despide del maestro).
func (l *lanzador) detener() {
	l.mu.Lock()
	t, m := l.trabajador, l.maestro
	l.mu.Unlock()
	if t != nil {
		t.detener()
	}
	if m != nil {
		m.detener()
	}
	l.mu.Lock()
	l.limpiarLocked()
	l.mu.Unlock()
}

func (l *lanzador) limpiarLocked() {
	if l.pararAviso != nil {
		close(l.pararAviso)
		l.pararAviso = nil
	}
	if l.trabajador.vivo() {
		go l.trabajador.detener()
	}
	if l.maestro.vivo() {
		go l.maestro.detener()
	}
	l.rol, l.maestro, l.trabajador, l.destino, l.webMaestro = "", nil, nil, "", ""
}

// ---------- /api → maestro ----------

func (l *lanzador) proxy(w http.ResponseWriter, r *http.Request) {
	l.mu.Lock()
	web := l.webMaestro
	l.mu.Unlock()
	if web == "" {
		fallo(w, http.StatusServiceUnavailable, errors.New("esta laptop no está en un clúster"))
		return
	}
	destino, _ := url.Parse(web)
	p := httputil.NewSingleHostReverseProxy(destino)
	p.FlushInterval = -1 // eventos en vivo (SSE) sin demora
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		fallo(w, http.StatusBadGateway, fmt.Errorf("no se pudo hablar con el maestro en %s: %v", web, err))
	}
	p.ServeHTTP(w, r)
}

// ---------- utilidades ----------

func (l *lanzador) datosPagina() string {
	if l.cfg.datosPagina != "" {
		return l.cfg.datosPagina
	}
	return filepath.Join("frontend", "public", "datos") + "," + filepath.Join(l.cfg.dirFrontend, "datos")
}

// ipPrincipal elige la IP por la que los demás llegan al maestro: la del
// switch (subred 192.168.50.0/24 de docs/cluster/switch); si no hay
// switch, la de Tailscale (100.64.0.0/10); si tampoco, la primera.
func ipPrincipal(ips []string) string {
	_, tailscale, _ := net.ParseCIDR("100.64.0.0/10")
	var ts string
	for _, s := range ips {
		ip := net.ParseIP(s)
		if strings.HasPrefix(s, "192.168.50.") {
			return s
		}
		if ts == "" && tailscale.Contains(ip) {
			ts = s
		}
	}
	if ts != "" {
		return ts
	}
	if len(ips) > 0 {
		return ips[0]
	}
	return ""
}

// ipsLocales: las IPv4 por las que los nodos pueden llegar a esta laptop,
// sin las de loopback ni las de interfaces virtuales (Docker, libvirt,
// VMware, VirtualBox; en Windows se llaman "VMware Network Adapter…").
func ipsLocales() []string {
	ips := []string{}
	interfaces, _ := net.Interfaces()
	for _, i := range interfaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		n := strings.ToLower(i.Name)
		if strings.HasPrefix(n, "docker") || strings.HasPrefix(n, "br-") || strings.HasPrefix(n, "virbr") || strings.HasPrefix(n, "veth") || strings.HasPrefix(n, "vmnet") || strings.HasPrefix(n, "vboxnet") || strings.Contains(n, "vethernet") || strings.Contains(n, "vmware") || strings.Contains(n, "virtualbox") {
			continue
		}
		direcciones, _ := i.Addrs()
		for _, a := range direcciones {
			if red, ok := a.(*net.IPNet); ok && red.IP.To4() != nil && !red.IP.IsLinkLocalUnicast() {
				ips = append(ips, red.IP.String())
			}
		}
	}
	return ips
}

// spa sirve el frontend; las rutas desconocidas devuelven index.html.
func spa(dir string) http.Handler {
	archivos := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta := filepath.Join(dir, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))
		if info, err := os.Stat(ruta); err != nil || info.IsDir() && !existe(filepath.Join(ruta, "index.html")) {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		archivos.ServeHTTP(w, r)
	})
}

func responder(w http.ResponseWriter, codigo int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(v)
}

func fallo(w http.ResponseWriter, codigo int, err error) {
	responder(w, codigo, map[string]string{"error": err.Error()})
}

func existe(ruta string) bool {
	_, err := os.Stat(ruta)
	return err == nil
}

func itoa(n int) string { return strconv.Itoa(n) }

func instancia() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func abrirNavegador(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("abre %s en el navegador", u)
	}
}

// cacheHuella: la huella de muestra.bin y el tamaño y la fecha del archivo
// con que se calculó. La página pregunta el estado cada 2 s; leer 360 MB
// cada vez dejaba el archivo casi siempre abierto, y en Windows eso impide
// que la limpieza lo reemplace ("Access is denied").
var cacheHuella struct {
	sync.Mutex
	tam    int64
	fecha  time.Time
	huella string
}

func (l *lanzador) huellaMuestra() string {
	info, err := os.Stat(l.cfg.rutaMuestra)
	if err != nil {
		return ""
	}
	c := &cacheHuella
	c.Lock()
	defer c.Unlock()
	if c.huella != "" && info.Size() == c.tam && info.ModTime().Equal(c.fecha) {
		return c.huella
	}
	h, err := distribuido.Huella(l.cfg.rutaMuestra)
	if err != nil {
		return ""
	}
	c.tam, c.fecha, c.huella = info.Size(), info.ModTime(), h
	return h
}
