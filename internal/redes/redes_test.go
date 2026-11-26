package redes

import "testing"

func TestPermitida(t *testing.T) {
	for ip, ok := range map[string]bool{"192.168.50.3": true, "100.118.152.101": true, "127.0.0.1": true, "192.168.50.3:9000": true,
		"192.168.18.33": false, "10.0.0.5": false, "172.17.0.1": false, "": false} {
		if PermitidaTexto(ip) != ok {
			t.Errorf("%q: %v", ip, !ok)
		}
	}
	t.Setenv("SACA_REDES", "todas")
	if !PermitidaTexto("192.168.18.33") {
		t.Error("SACA_REDES=todas")
	}
	t.Setenv("SACA_REDES", "10.0.0.0/8")
	if !PermitidaTexto("10.0.0.5") || PermitidaTexto("192.168.50.3") {
		t.Error("SACA_REDES=10.0.0.0/8")
	}
}
