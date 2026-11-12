// Package pares es la comunicación directa entre nodos trabajadores (Fase 4
// de docs/plan_avance.md): en vez de que todos los gradientes pasen por el
// maestro (un solo puerto del switch), los nodos los suman entre ellos con
// una reducción colectiva, como MPI_Allreduce:
//
//   - árbol: reducción binomial hacia el rango 0 y difusión de vuelta
//     (log2 n niveles; en cada nivel, varios pares hablan a la vez);
//   - anillo: reduce-scatter + allgather (cada nodo envía y recibe un
//     trozo en cada uno de los 2(n-1) pasos: todos los enlaces trabajan).
//
// En los dos, cada componente del resultado se suma una sola vez, en un solo
// nodo, y después se copia: todos los nodos terminan con exactamente los
// mismos bits. Así cada uno puede aplicar Adam por su cuenta y los
// parámetros siguen idénticos en todo el clúster.
//
// Formato en el cable: TCP crudo, cada mensaje = largo (uint64) + float64
// little-endian. El saludo al conectar: corrida (uint32) y rango (uint32).
package pares

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"
)

const (
	Central = "central" // todo por el maestro (sin pares)
	Arbol   = "arbol"
	Anillo  = "anillo"
)

// Valido dice si t es una topología conocida.
func Valido(t string) bool { return t == Central || t == Arbol || t == Anillo }

type clave struct{ corrida, rango int }

// Pares escucha las conexiones de los otros nodos.
type Pares struct {
	ln net.Listener

	mu        sync.Mutex
	llegadas  map[clave]net.Conn
	esperando map[clave]chan net.Conn
}

// Escuchar abre el puerto de pares (p. ej. ":9002").
func Escuchar(dir string) (*Pares, error) {
	ln, err := net.Listen("tcp", dir)
	if err != nil {
		return nil, err
	}
	p := &Pares{ln: ln, llegadas: map[clave]net.Conn{}, esperando: map[clave]chan net.Conn{}}
	go p.aceptar()
	return p, nil
}

// Puerto en el que escucha.
func (p *Pares) Puerto() int { return p.ln.Addr().(*net.TCPAddr).Port }

func (p *Pares) Cerrar() error { return p.ln.Close() }

func (p *Pares) aceptar() {
	for {
		conn, err := p.ln.Accept()
		if err != nil {
			return
		}
		go func() {
			conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			var saludo [8]byte
			if _, err := io.ReadFull(conn, saludo[:]); err != nil {
				conn.Close()
				return
			}
			conn.SetReadDeadline(time.Time{})
			k := clave{int(binary.LittleEndian.Uint32(saludo[:])), int(binary.LittleEndian.Uint32(saludo[4:]))}
			p.mu.Lock()
			if ch, ok := p.esperando[k]; ok {
				delete(p.esperando, k)
				ch <- conn
			} else {
				p.llegadas[k] = conn
			}
			p.mu.Unlock()
		}()
	}
}

// esperar la conexión que abre el nodo de rango desde la corrida.
func (p *Pares) esperar(ctx context.Context, corrida, rango int) (net.Conn, error) {
	k := clave{corrida, rango}
	p.mu.Lock()
	if c, ok := p.llegadas[k]; ok {
		delete(p.llegadas, k)
		p.mu.Unlock()
		return c, nil
	}
	ch := make(chan net.Conn, 1)
	p.esperando[k] = ch
	p.mu.Unlock()
	select {
	case c := <-ch:
		return c, nil
	case <-ctx.Done():
		p.mu.Lock()
		delete(p.esperando, k)
		p.mu.Unlock()
		return nil, fmt.Errorf("el nodo de rango %d no se conectó: %w", rango, ctx.Err())
	}
}

func marcar(ctx context.Context, dir string, corrida, rango int) (net.Conn, error) {
	var d net.Dialer
	for {
		conn, err := d.DialContext(ctx, "tcp", dir)
		if err == nil {
			var saludo [8]byte
			binary.LittleEndian.PutUint32(saludo[:], uint32(corrida))
			binary.LittleEndian.PutUint32(saludo[4:], uint32(rango))
			if _, err := conn.Write(saludo[:]); err != nil {
				conn.Close()
				return nil, err
			}
			if tc, ok := conn.(*net.TCPConn); ok {
				tc.SetNoDelay(true)
			}
			return conn, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("no se pudo conectar con el par %s (¿puerto de pares abierto en su firewall?): %v", dir, err)
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Grupo son las conexiones de un nodo con sus pares en una corrida.
type Grupo struct {
	topo     string
	rango, n int
	salida   map[int]*enlace // por rango del par
	entrada  map[int]*enlace
	Bytes    int64 // enviados por este nodo
}

type enlace struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

func nuevoEnlace(c net.Conn) *enlace {
	return &enlace{conn: c, r: bufio.NewReaderSize(c, 1<<16), w: bufio.NewWriterSize(c, 1<<16)}
}

// Unirse arma las conexiones que necesita el rango dentro de dirs (la
// dirección de pares de cada rango) para la topología. El que tiene el
// rango menor de cada par llama al otro; en el anillo, cada uno llama al
// siguiente.
func (p *Pares) Unirse(ctx context.Context, corrida, rango int, dirs []string, topo string) (*Grupo, error) {
	n := len(dirs)
	g := &Grupo{topo: topo, rango: rango, n: n, salida: map[int]*enlace{}, entrada: map[int]*enlace{}}
	if n <= 1 || topo == Central {
		return g, nil
	}
	ctx, cancelar := context.WithTimeout(ctx, 30*time.Second)
	defer cancelar()
	var marcarA, esperarDe []int
	switch topo {
	case Arbol:
		for k := 1; k < n; k *= 2 {
			if rango%(2*k) == 0 && rango+k < n {
				marcarA = append(marcarA, rango+k) // hijo
			} else if rango%(2*k) == k {
				esperarDe = append(esperarDe, rango-k) // padre
			}
		}
	case Anillo:
		marcarA = []int{(rango + 1) % n}
		esperarDe = []int{(rango - 1 + n) % n}
	default:
		return nil, fmt.Errorf("topología desconocida %q", topo)
	}
	errs := make(chan error, len(marcarA)+len(esperarDe))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, r := range marcarA {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			c, err := marcar(ctx, dirs[r], corrida, rango)
			if err != nil {
				errs <- err
				return
			}
			e := nuevoEnlace(c)
			mu.Lock()
			g.salida[r] = e
			if topo == Arbol {
				g.entrada[r] = e
			}
			mu.Unlock()
		}(r)
	}
	for _, r := range esperarDe {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			c, err := p.esperar(ctx, corrida, r)
			if err != nil {
				errs <- err
				return
			}
			e := nuevoEnlace(c)
			mu.Lock()
			g.entrada[r] = e
			if topo == Arbol {
				g.salida[r] = e
			}
			mu.Unlock()
		}(r)
	}
	wg.Wait()
	close(errs)
	if err := <-errs; err != nil {
		g.Cerrar()
		return nil, err
	}
	return g, nil
}

// Cerrar las conexiones del grupo.
func (g *Grupo) Cerrar() {
	vistos := map[net.Conn]bool{}
	for _, m := range []map[int]*enlace{g.salida, g.entrada} {
		for _, e := range m {
			if !vistos[e.conn] {
				vistos[e.conn] = true
				e.conn.Close()
			}
		}
	}
}

func (g *Grupo) enviar(r int, v []float64) error {
	e := g.salida[r]
	var largo [8]byte
	binary.LittleEndian.PutUint64(largo[:], uint64(len(v)))
	if _, err := e.w.Write(largo[:]); err != nil {
		return err
	}
	buf := make([]byte, 8*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint64(buf[8*i:], math.Float64bits(x))
	}
	if _, err := e.w.Write(buf); err != nil {
		return err
	}
	g.Bytes += int64(8 + len(buf))
	return e.w.Flush()
}

func (g *Grupo) recibir(r int, dst []float64) error {
	e := g.entrada[r]
	var largo [8]byte
	if _, err := io.ReadFull(e.r, largo[:]); err != nil {
		return err
	}
	if n := binary.LittleEndian.Uint64(largo[:]); n != uint64(len(dst)) {
		return fmt.Errorf("el par %d mandó %d valores y se esperaban %d", r, n, len(dst))
	}
	buf := make([]byte, 8*len(dst))
	if _, err := io.ReadFull(e.r, buf); err != nil {
		return err
	}
	for i := range dst {
		dst[i] = math.Float64frombits(binary.LittleEndian.Uint64(buf[8*i:]))
	}
	return nil
}

// AllReduce suma v entre todos los nodos del grupo, en el lugar: al volver,
// todos tienen la misma suma, bit a bit.
func (g *Grupo) AllReduce(v []float64) error {
	if g.n <= 1 {
		return nil
	}
	switch g.topo {
	case Arbol:
		return g.arbol(v)
	case Anillo:
		return g.anillo(v)
	}
	return errors.New("el grupo no tiene topología de pares")
}

// arbol: reducción binomial hacia 0 y difusión desde 0.
func (g *Grupo) arbol(v []float64) error {
	r, n := g.rango, g.n
	tmp := make([]float64, len(v))
	k := 1
	for ; k < n; k *= 2 {
		if r%(2*k) == k {
			if err := g.enviar(r-k, v); err != nil {
				return err
			}
			break
		}
		if r+k < n {
			if err := g.recibir(r+k, tmp); err != nil {
				return err
			}
			for i, x := range tmp {
				v[i] += x
			}
		}
	}
	// Difusión: el mismo árbol, al revés.
	if r != 0 {
		if err := g.recibir(r-lsb(r), v); err != nil {
			return err
		}
	}
	for k := hasta(r, n); k >= 1; k /= 2 {
		if r%(2*k) == 0 && r+k < n {
			if err := g.enviar(r+k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// lsb: el bit más bajo de r (la distancia a su padre en el árbol binomial).
func lsb(r int) int { return r & -r }

// hasta: el mayor k (potencia de 2) con el que r todavía envía a un hijo.
func hasta(r, n int) int {
	k := 1
	for k < n {
		k *= 2
	}
	if r != 0 {
		k = lsb(r)
	}
	return k
}

// anillo: reduce-scatter y allgather en trozos.
func (g *Grupo) anillo(v []float64) error {
	r, n := g.rango, g.n
	sig, ant := (r+1)%n, (r-1+n)%n
	trozo := func(i int) []float64 {
		i = ((i % n) + n) % n
		return v[i*len(v)/n : (i+1)*len(v)/n]
	}
	paso := func(enviarT, recibirT int, sumar bool) error {
		envio := append([]float64(nil), trozo(enviarT)...)
		destino := trozo(recibirT)
		llegada := make([]float64, len(destino))
		errEnvio := make(chan error, 1)
		go func() { errEnvio <- g.enviar(sig, envio) }()
		if err := g.recibir(ant, llegada); err != nil {
			return err
		}
		if err := <-errEnvio; err != nil {
			return err
		}
		for i, x := range llegada {
			if sumar {
				destino[i] += x
			} else {
				destino[i] = x
			}
		}
		return nil
	}
	for t := 0; t < n-1; t++ { // reduce-scatter
		if err := paso(r-t, r-t-1, true); err != nil {
			return err
		}
	}
	for t := 0; t < n-1; t++ { // allgather
		if err := paso(r-t+1, r-t, false); err != nil {
			return err
		}
	}
	return nil
}
