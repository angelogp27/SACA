package pares

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MPI: los nodos suman con MPI_Allreduce y el maestro puede juntar las
// predicciones con MPI_Gatherv. MPI no lo habla Go: en cada laptop corre un
// proceso en C (nativo/mpi/puente) conectado al puerto de pares del
// trabajador; el maestro los lanza a todos con mpiexec.
const MPI = "mpi"

const (
	rangoPuente    = 0xFFFFFFFF
	ordenAllreduce = 1
	ordenGatherv   = 2
	esperaPuente   = 30 * time.Second
)

// unirseMPI espera al puente que lanza mpiexec y arma el grupo. El
// resultado se usa como cualquier otro Grupo.
func (p *Pares) unirseMPI(ctx context.Context, corrida, rango, n int) (*Grupo, error) {
	ctx, cancelar := context.WithTimeout(ctx, esperaPuente)
	defer cancelar()
	c, err := p.esperar(ctx, corrida, rangoPuente)
	if err != nil {
		return nil, fmt.Errorf("el puente MPI de este nodo no se conectó: %w", ctx.Err())
	}
	if tc, ok := c.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}
	e := nuevoEnlace(c)
	return &Grupo{topo: MPI, rango: rango, n: n, salida: map[int]*enlace{0: e}, entrada: map[int]*enlace{0: e}}, nil
}

func (e *enlace) orden(orden uint32, n int) {
	var b [12]byte
	binary.LittleEndian.PutUint32(b[:], orden)
	binary.LittleEndian.PutUint64(b[4:], uint64(n))
	e.w.Write(b[:])
}

func (g *Grupo) allreduceMPI(v []float64) error {
	e := g.salida[0]
	e.orden(ordenAllreduce, len(v))
	buf := make([]byte, 8*len(v))
	codificar(buf, v)
	if _, err := e.w.Write(buf); err != nil {
		return err
	}
	if err := e.w.Flush(); err != nil {
		return err
	}
	if _, err := io.ReadFull(e.r, buf); err != nil {
		return fmt.Errorf("el puente MPI se cerró: %w", err)
	}
	decodificar(v, buf)
	// MPI no dice cuántos bytes mandó: se anota lo mínimo que exige un
	// Allreduce por nodo, 2(n-1)/n vectores (estimado).
	g.Bytes += int64(2 * (g.n - 1) * len(buf) / g.n)
	return nil
}

// Gatherv junta los datos de todos en el rango 0 (los demás reciben nil).
func (g *Grupo) Gatherv(datos []byte) ([][]byte, error) {
	if g.topo != MPI {
		return nil, errors.New("Gatherv solo con MPI")
	}
	e := g.salida[0]
	e.orden(ordenGatherv, len(datos))
	if _, err := e.w.Write(datos); err != nil {
		return nil, err
	}
	if err := e.w.Flush(); err != nil {
		return nil, err
	}
	if g.rango != 0 {
		g.Bytes += int64(len(datos))
		return nil, nil
	}
	cab := make([]byte, 8*g.n)
	if _, err := io.ReadFull(e.r, cab); err != nil {
		return nil, fmt.Errorf("el puente MPI se cerró: %w", err)
	}
	partes := make([][]byte, g.n)
	for i := range partes {
		partes[i] = make([]byte, binary.LittleEndian.Uint64(cab[8*i:]))
		if _, err := io.ReadFull(e.r, partes[i]); err != nil {
			return nil, fmt.Errorf("el puente MPI se cerró: %w", err)
		}
	}
	return partes, nil
}

// NodoMPI es un rango: la IP de su laptop, su puerto de pares, la ruta del
// puente EN ESA laptop y su sistema ("linux/amd64").
type NodoMPI struct {
	ID, IP, Puente, SO string
	Puerto             int
}

// Puente devuelve la ruta absoluta de bin/puente_mpi si está compilado.
func Puente(dirBin string) string {
	ruta := filepath.Join(dirBin, "puente_mpi")
	if runtime.GOOS == "windows" {
		ruta += ".exe"
	}
	if _, err := os.Stat(ruta); err != nil {
		return ""
	}
	if abs, err := filepath.Abs(ruta); err == nil {
		return abs
	}
	return ruta
}

func buscarMpiexec() (string, error) {
	if e := os.Getenv("SACA_MPIEXEC"); e != "" {
		return e, nil
	}
	if e, err := exec.LookPath("mpiexec"); err == nil {
		return e, nil
	}
	for _, e := range []string{"/usr/lib64/openmpi/bin/mpiexec", `C:\Program Files\Microsoft MPI\Bin\mpiexec.exe`} {
		if _, err := os.Stat(e); err == nil {
			return e, nil
		}
	}
	return "", errors.New("no se encontró mpiexec: instala Open MPI (Linux) o MS-MPI (Windows)")
}

// ComprobarMPI dice por qué no se puede usar MPI con estos nodos (nil = sí).
func ComprobarMPI(nodos []NodoMPI) error {
	for _, n := range nodos {
		if n.Puente == "" || n.Puerto == 0 {
			return fmt.Errorf("%s no tiene bin/puente_mpi: instala MPI y vuelve a abrir SACA", n.ID)
		}
		so, _, _ := strings.Cut(n.SO, "/")
		if so != "" && so != runtime.GOOS {
			return fmt.Errorf("%s usa %s y este maestro %s: MS-MPI y Open MPI no se comunican entre sí; elige laptops con el mismo sistema o usa árbol o anillo", n.ID, so, runtime.GOOS)
		}
	}
	_, err := buscarMpiexec()
	return err
}

// LanzadoMPI es un mpiexec en marcha.
type LanzadoMPI struct {
	Fin      chan struct{} // se cierra cuando mpiexec termina
	cancelar context.CancelFunc
	mu       sync.Mutex
	err      error
}

// Err de mpiexec, después de Fin.
func (l *LanzadoMPI) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

// Detener espera un momento a que los puentes terminen solos (sus
// trabajadores cierran la conexión) y, si no, los mata.
func (l *LanzadoMPI) Detener() {
	select {
	case <-l.Fin:
	case <-time.After(3 * time.Second):
	}
	l.cancelar()
	<-l.Fin
}

// LanzarMPI arranca un puente por nodo, en orden de rango.
func LanzarMPI(corrida int, nodos []NodoMPI) (*LanzadoMPI, error) {
	if err := ComprobarMPI(nodos); err != nil {
		return nil, err
	}
	exe, err := buscarMpiexec()
	if err != nil {
		return nil, err
	}
	ctx, cancelar := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, exe, argumentosMPI(corrida, nodos, runtime.GOOS)...)
	if runtime.GOOS != "windows" { // en Windows no hay SIGINT: se mata
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	}
	cmd.WaitDelay = 5 * time.Second
	// Open MPI de Fedora está fuera del PATH; mpiexec busca ahí su prterun.
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(exe)+string(os.PathListSeparator)+os.Getenv("PATH"))
	var salida colaTexto
	cmd.Stdout, cmd.Stderr = &salida, &salida
	log.Printf("MPI: %s %s", exe, strings.Join(cmd.Args[1:], " "))
	if err := cmd.Start(); err != nil {
		cancelar()
		return nil, fmt.Errorf("no se pudo lanzar mpiexec: %w", err)
	}
	l := &LanzadoMPI{Fin: make(chan struct{}), cancelar: cancelar}
	go func() {
		err := cmd.Wait()
		if err != nil {
			err = fmt.Errorf("mpiexec terminó con error (%v): %s", err, salida.String())
		}
		l.mu.Lock()
		l.err = err
		l.mu.Unlock()
		close(l.Fin)
	}()
	return l, nil
}

// argumentosMPI arma la línea de mpiexec: un bloque por rango (MPMD), porque
// la ruta del puente puede ser distinta en cada laptop. -n y -host se
// escriben igual en Open MPI y en MS-MPI.
func argumentosMPI(corrida int, nodos []NodoMPI, so string) []string {
	local := true
	for _, n := range nodos {
		if ip := net.ParseIP(n.IP); ip == nil || !ip.IsLoopback() {
			local = false
		}
	}
	var args []string
	if so == "windows" {
		if red, mascara := redDe(nodos); !local && red != "" {
			args = append(args, "-env", "MPICH_NETMASK", red+"/"+mascara)
		}
	} else {
		// Sin esto, Open MPI prueba también las redes de Docker, VMware o
		// Tailscale de cada laptop y se cuelga.
		args = append(args, "--map-by", ":OVERSUBSCRIBE", "--bind-to", "none", "--mca", "mpi_yield_when_idle", "1")
		if red, _ := redDe(nodos); !local && red != "" {
			cidr := red + "/24"
			if strings.HasPrefix(red, "100.") {
				cidr = "100.64.0.0/10"
			}
			args = append(args, "--mca", "btl_tcp_if_include", cidr, "--prtemca", "prte_if_include", cidr)
		}
	}
	puertos := []string{strconv.Itoa(corrida)}
	for _, n := range nodos {
		puertos = append(puertos, strconv.Itoa(n.Puerto))
	}
	for i, n := range nodos {
		if i > 0 {
			args = append(args, ":")
		}
		args = append(args, "-n", "1")
		if !local {
			args = append(args, "-host", n.IP)
		}
		args = append(append(args, n.Puente), puertos...)
	}
	return args
}

// redDe: la red /24 del primer nodo que no es esta misma máquina.
func redDe(nodos []NodoMPI) (red, mascara string) {
	for _, n := range nodos {
		if ip := net.ParseIP(n.IP).To4(); ip != nil && !ip.IsLoopback() {
			return fmt.Sprintf("%d.%d.%d.0", ip[0], ip[1], ip[2]), "255.255.255.0"
		}
	}
	return "", ""
}

// colaTexto guarda lo último que escribió un proceso.
type colaTexto struct {
	mu sync.Mutex
	b  []byte
}

func (c *colaTexto) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.b = append(c.b, p...)
	if len(c.b) > 1500 {
		c.b = c.b[len(c.b)-1500:]
	}
	return len(p), nil
}

func (c *colaTexto) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(string(c.b))
}
