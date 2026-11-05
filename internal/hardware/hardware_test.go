package hardware

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsearNVIDIA(t *testing.T) {
	gpu, driver := parsearNVIDIA("NVIDIA GeForce RTX 3060, 12288 MiB, 560.35.03\n")
	if gpu != "NVIDIA GeForce RTX 3060, 12288 MiB" || driver != "560.35.03" {
		t.Fatalf("gpu=%q driver=%q", gpu, driver)
	}
	if gpu, _ := parsearNVIDIA(""); gpu != "" {
		t.Fatalf("sin salida debe dar vacío, dio %q", gpu)
	}
}

func TestLeerCPUInfoYMemInfo(t *testing.T) {
	dir := t.TempDir()
	cpuinfo := filepath.Join(dir, "cpuinfo")
	os.WriteFile(cpuinfo, []byte("model name\t: AMD Ryzen 5 3600\nphysical id\t: 0\ncore id\t\t: 0\n\nmodel name\t: AMD Ryzen 5 3600\nphysical id\t: 0\ncore id\t\t: 0\n\nmodel name\t: AMD Ryzen 5 3600\nphysical id\t: 0\ncore id\t\t: 1\n"), 0o644)
	modelo, nucleos := leerCPUInfo(cpuinfo)
	if modelo != "AMD Ryzen 5 3600" || nucleos != 2 {
		t.Fatalf("modelo=%q nucleos=%d (dos hilos del mismo núcleo cuentan una vez)", modelo, nucleos)
	}
	meminfo := filepath.Join(dir, "meminfo")
	os.WriteFile(meminfo, []byte("MemTotal:       16777216 kB\nMemFree: 1 kB\n"), 0o644)
	if gb := leerMemInfo(meminfo); gb != 16 {
		t.Fatalf("RAM = %v GB, quería 16", gb)
	}
}

func TestResumen(t *testing.T) {
	r := Info{CPU: "Ryzen", Hilos: 12, Nucleos: 6, RAMGB: 16, GPU: "RTX 3060, 12288 MiB", Driver: "560"}.Resumen()
	for _, quiere := range []string{"Ryzen", "6N/12H", "16 GB", "RTX 3060", "driver 560"} {
		if !strings.Contains(r, quiere) {
			t.Errorf("el resumen %q no contiene %q", r, quiere)
		}
	}
	if r := (Info{Hilos: 4}).Resumen(); r != "4H" {
		t.Errorf("resumen mínimo = %q", r)
	}
}

func TestDetectarNoFalla(t *testing.T) {
	if i := Detectar(); i.Hilos < 1 || i.SO == "" {
		t.Fatalf("Detectar() = %+v", i)
	}
}
