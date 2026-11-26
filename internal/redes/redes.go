// Package redes decide por qué redes puede viajar el clúster: el cable del
// switch (192.168.50.0/24, docs/switch) o la VPN (Tailscale, 100.64.0.0/10).
// Nunca el Wi-Fi: es lento y compartido (la predicción tardaba 20 s en vez
// de 3). La misma laptop (127.0.0.1) siempre vale.
//
// SACA_REDES (variable de entorno, CIDR separados por comas) cambia la
// lista; "todas" lo desactiva. Solo para casos excepcionales.
package redes

import (
	"fmt"
	"net"
	"os"
	"strings"
)

const Switch = "192.168.50.0/24"
const VPN = "100.64.0.0/10"

func permitidas() ([]*net.IPNet, bool) {
	lista := Switch + "," + VPN
	if v := strings.TrimSpace(os.Getenv("SACA_REDES")); v != "" {
		if strings.EqualFold(v, "todas") {
			return nil, true
		}
		lista = v
	}
	var redes []*net.IPNet
	for _, c := range strings.Split(lista, ",") {
		if _, r, err := net.ParseCIDR(strings.TrimSpace(c)); err == nil {
			redes = append(redes, r)
		}
	}
	return redes, false
}

// Permitida: si el clúster puede usar esta IP.
func Permitida(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	redes, todas := permitidas()
	if todas {
		return true
	}
	for _, r := range redes {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

// PermitidaTexto: lo mismo, con la IP escrita ("192.168.50.2" o "ip:puerto").
func PermitidaTexto(s string) bool {
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	return Permitida(net.ParseIP(s))
}

// Motivo explica el rechazo de una IP, para logs y la página.
func Motivo(ip string) string {
	return fmt.Sprintf("la IP %s no es del switch (%s) ni de la VPN (%s): el clúster no usa el Wi-Fi; conecta el cable del switch con tu IP fija o usa Tailscale", ip, Switch, VPN)
}
