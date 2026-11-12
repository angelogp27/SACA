package main

import (
	"encoding/json"
	"log"
	"net"
	"sort"
	"strings"
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
	vistos map[string]*ClusterVisto // por instancia del maestro
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
		// El maestro anuncia por todas sus redes (Wi-Fi, switch…), así que el
		// mismo maestro llega desde varias IP. Se guarda una entrada por
		// maestro y, de sus IP, la del switch (192.168.50.x) si llega por ahí:
		// si no, el clúster iría por el Wi-Fi.
		ip := origen.(*net.UDPAddr).IP.String()
		d.mu.Lock()
		v, ok := d.vistos[a.Instancia]
		switch {
		case !ok:
			d.vistos[a.Instancia] = &ClusterVisto{Host: a.Host, IP: ip, TCP: a.TCP, Web: a.Web, instante: time.Now()}
		case ip == v.IP || esSwitch(ip) && !esSwitch(v.IP):
			v.IP, v.instante = ip, time.Now() // la misma IP, o una mejor (la del switch)
		case !esSwitch(v.IP):
			v.instante = time.Now() // otra red cualquiera: sigue vivo
		}
		d.mu.Unlock()
	}
}

// esSwitch: la subred del switch del grupo (docs/cluster/switch).
func esSwitch(ip string) bool { return strings.HasPrefix(ip, "192.168.50.") }

// Vistos devuelve los maestros anunciados en los últimos segundos.
func (d *descubridor) Vistos() []ClusterVisto {
	d.mu.Lock()
	defer d.mu.Unlock()
	var lista []ClusterVisto
	for clave, v := range d.vistos {
		hace := time.Since(v.instante)
		if hace > vigenciaAnuncio {
			delete(d.vistos, clave)
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
