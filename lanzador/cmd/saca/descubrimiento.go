package main

import (
	"encoding/json"
	"log"
	"net"
	"sort"
	"sync"
	"time"
)

// Descubrimiento en la LAN: la laptop que inicia el clúster anuncia cada
// segundo, por broadcast UDP, "aquí hay un maestro" (host, puertos). Todas
// las laptops escuchan esos anuncios: así "Unirse" encuentra solo la IP
// del maestro y "Iniciar clúster" se niega si otra laptop ya lo inició.

type anuncio struct {
	Saca      int    `json:"saca"`
	Instancia string `json:"instancia"` // para ignorar los propios
	Host      string `json:"host"`
	TCP       int    `json:"tcp"`
	Web       int    `json:"web"`
}

// ClusterVisto es un maestro anunciado en la red.
type ClusterVisto struct {
	Host     string  `json:"host"`
	IP       string  `json:"ip"`
	TCP      int     `json:"tcp"`
	Web      int     `json:"web"`
	HaceSeg  float64 `json:"hace_s"`
	instante time.Time
}

const vigenciaAnuncio = 4 * time.Second

type descubridor struct {
	puerto    int
	instancia string

	mu     sync.Mutex
	vistos map[string]*ClusterVisto // por IP
}

func nuevoDescubridor(puerto int, instancia string) *descubridor {
	return &descubridor{puerto: puerto, instancia: instancia, vistos: map[string]*ClusterVisto{}}
}

// escuchar recibe anuncios hasta que falle el socket (p. ej. el puerto ya
// está tomado por otro lanzador en la misma PC: entonces no hay
// descubrimiento y se avisa en el log).
func (d *descubridor) escuchar() {
	pc, err := net.ListenPacket("udp4", net.JoinHostPort("", itoa(d.puerto)))
	if err != nil {
		log.Printf("AVISO: no se pudo escuchar anuncios en UDP %d (%v): habrá que escribir la IP del maestro a mano", d.puerto, err)
		return
	}
	defer pc.Close()
	buf := make([]byte, 1024)
	for {
		n, origen, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		var a anuncio
		if json.Unmarshal(buf[:n], &a) != nil || a.Saca != 1 || a.Instancia == d.instancia {
			continue
		}
		ip := origen.(*net.UDPAddr).IP.String()
		d.mu.Lock()
		d.vistos[ip] = &ClusterVisto{Host: a.Host, IP: ip, TCP: a.TCP, Web: a.Web, instante: time.Now()}
		d.mu.Unlock()
	}
}

// Vistos devuelve los maestros anunciados en los últimos segundos.
func (d *descubridor) Vistos() []ClusterVisto {
	d.mu.Lock()
	defer d.mu.Unlock()
	var lista []ClusterVisto
	for ip, v := range d.vistos {
		hace := time.Since(v.instante)
		if hace > vigenciaAnuncio {
			delete(d.vistos, ip)
			continue
		}
		c := *v
		c.HaceSeg = hace.Seconds()
		lista = append(lista, c)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].IP < lista[j].IP })
	return lista
}

// anunciar emite el anuncio cada segundo hasta que se cierre parar.
func (d *descubridor) anunciar(host string, tcp, web int, parar <-chan struct{}) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		log.Printf("AVISO: no se puede anunciar el clúster por UDP: %v", err)
		return
	}
	defer conn.Close()
	datos, _ := json.Marshal(anuncio{Saca: 1, Instancia: d.instancia, Host: host, TCP: tcp, Web: web})
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		for _, b := range direccionesBroadcast() {
			conn.WriteTo(datos, &net.UDPAddr{IP: b, Port: d.puerto})
		}
		select {
		case <-parar:
			return
		case <-t.C:
		}
	}
}

// direccionesBroadcast: la de cada red IPv4 de la PC (p. ej. 192.168.50.255)
// y la general 255.255.255.255. Windows no siempre envía la general por
// todas las interfaces, por eso se mandan las dos.
func direccionesBroadcast() []net.IP {
	lista := []net.IP{net.IPv4bcast}
	interfaces, _ := net.Interfaces()
	for _, i := range interfaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagBroadcast == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		direcciones, _ := i.Addrs()
		for _, a := range direcciones {
			red, ok := a.(*net.IPNet)
			if !ok || red.IP.To4() == nil {
				continue
			}
			ip, mascara := red.IP.To4(), red.Mask
			if len(mascara) == 16 {
				mascara = mascara[12:]
			}
			b := make(net.IP, 4)
			for k := range b {
				b[k] = ip[k] | ^mascara[k]
			}
			lista = append(lista, b)
		}
	}
	return lista
}
