package limpieza

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Corre los dos cargadores reales sobre data/raw (unos 40 s en total):
//
//	SACA_LIMPIEZA_REAL=1 go test ./internal/limpieza -run Real -v
func TestEjecutarReal(t *testing.T) {
	if os.Getenv("SACA_LIMPIEZA_REAL") == "" {
		t.Skip("define SACA_LIMPIEZA_REAL=1 para correrla")
	}
	c := Config{DirDatos: "../../data/raw", Anio: "2025", RutaBin: filepath.Join(t.TempDir(), "muestra.bin"), DirBin: "../../bin"}
	var huellas []string
	for _, tec := range []string{Pthreads, Go} {
		var avances []Avance
		res, err := c.Ejecutar(context.Background(), tec, 0, func(a Avance) { avances = append(avances, a) })
		if err != nil {
			t.Fatal(err)
		}
		maxCPU := 0.0
		for _, a := range avances {
			maxCPU = max(maxCPU, a.CPU)
		}
		t.Logf("%s: %s, %d hilos, %d avances, último %.3f, CPU máx %.0f%%, huella %s", tec, res.Duracion.Round(time.Millisecond), res.Hilos, len(avances), avances[len(avances)-1].Avance, maxCPU, res.Huella[:12])
		if avances[len(avances)-1].Avance != 1 || maxCPU <= 0 {
			t.Fatal("avance o CPU incorrectos")
		}
		huellas = append(huellas, res.Huella)
	}
	if huellas[0] != huellas[1] {
		t.Fatalf("huellas distintas: %s %s", huellas[0], huellas[1])
	}
}

func TestCancelar(t *testing.T) {
	if os.Getenv("SACA_LIMPIEZA_REAL") == "" {
		t.Skip("define SACA_LIMPIEZA_REAL=1 para correrla")
	}
	c := Config{DirDatos: "../../data/raw", Anio: "2025", RutaBin: filepath.Join(t.TempDir(), "muestra.bin"), DirBin: "../../bin"}
	ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelar()
	inicio := time.Now()
	_, err := c.Ejecutar(ctx, Go, 0, nil)
	if err == nil || time.Since(inicio) > 5*time.Second {
		t.Fatalf("no se canceló a tiempo: %v %s", err, time.Since(inicio))
	}
	if _, err := os.Stat(c.RutaBin); !os.IsNotExist(err) {
		t.Fatal("quedó un .bin de una limpieza cancelada")
	}
}

func TestFaltaEjecutable(t *testing.T) {
	c := Config{DirDatos: t.TempDir(), Anio: "2025", RutaBin: filepath.Join(t.TempDir(), "m.bin"), DirBin: t.TempDir()}
	if err := c.Comprobar(Pthreads); err == nil {
		t.Fatal("debía fallar sin ejecutable")
	}
}
