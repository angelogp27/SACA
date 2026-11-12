package muestrabin

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"proyecto9/internal/aqs"
	"proyecto9/internal/particion"
	"proyecto9/internal/registro"
)

// valores sintéticos: 6 estaciones, 400 horas repartidas en dos "años" de
// meses, 4 gases, con monitores repetidos (mismo y distinto POC) y
// coordenadas que cambian entre gases.
func valores() []aqs.ValorLeido {
	r := rand.New(rand.NewSource(7))
	gases := []string{"o3", "no2", "so2", "co"}
	inicio := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var vs []aqs.ValorLeido
	for e := 0; e < 6; e++ {
		est := fmt.Sprintf("%02d-%03d-%04d", e+1, e*7, e*13)
		for h := 0; h < 400; h++ {
			t := inicio.Add(time.Duration((h/2)*44+h%2) * time.Hour) // pares de horas seguidas a lo largo del año
			for g, gas := range gases {
				if r.Intn(10) == 0 {
					continue // hueco
				}
				base := aqs.ValorLeido{EstacionID: est, TimestampUTC: t.Format(time.RFC3339), Contaminante: gas,
					Latitud: 30 + float64(e) + float64(g)*0.001, Longitud: -70 - float64(e), Estado: "E", Condado: "C",
					Valor: r.Float64() * 80, POC: 1 + r.Intn(2)}
				vs = append(vs, base)
				if r.Intn(5) == 0 { // monitor repetido
					dup := base
					dup.POC = 1
					dup.Valor = r.Float64() * 80
					dup.Latitud += 0.01
					vs = append(vs, dup)
				}
				if r.Intn(8) == 0 { // fila idéntica salvo las coordenadas
					dup := base
					dup.Latitud -= 0.02
					vs = append(vs, dup)
				}
			}
		}
	}
	return vs
}

func construirBytes(t *testing.T, vs []aqs.ValorLeido, k int, semilla int64) []byte {
	t.Helper()
	orden := append([]aqs.ValorLeido(nil), vs...)
	rand.New(rand.NewSource(semilla)).Shuffle(len(orden), func(i, j int) { orden[i], orden[j] = orden[j], orden[i] })
	indices := make([]*aqs.Indice, k)
	for i := range indices {
		indices[i] = aqs.NuevoIndice()
	}
	for _, v := range orden {
		indices[particion.Asignar(v.EstacionID, k)].Agregar(v)
	}
	mapas := make([]map[string]*registro.RegistroProcesado, k)
	for i, ix := range indices {
		mapas[i] = ix.Registros
	}
	m, err := Construir(mapas, MesCorte)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := m.escribir(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDeterminista(t *testing.T) {
	vs := valores()
	ref := construirBytes(t, vs, 1, 1)
	for _, k := range []int{1, 2, 3, 8} {
		for semilla := int64(2); semilla < 5; semilla++ {
			if got := construirBytes(t, vs, k, semilla); !bytes.Equal(got, ref) {
				t.Fatalf("k=%d semilla=%d: %x != %x", k, semilla, sha256.Sum256(got), sha256.Sum256(ref))
			}
		}
	}
}

func TestEscribirLeer(t *testing.T) {
	vs := valores()
	indices := []*aqs.Indice{aqs.NuevoIndice()}
	for _, v := range vs {
		indices[0].Agregar(v)
	}
	m, err := Construir([]map[string]*registro.RegistroProcesado{indices[0].Registros}, MesCorte)
	if err != nil {
		t.Fatal(err)
	}
	ruta := filepath.Join(t.TempDir(), "muestra.bin")
	if err := m.Escribir(ruta); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ruta + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("quedó el temporal")
	}
	datos, err := Leer(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if datos.Entrenamiento.Len() != len(m.YEnt) || datos.Prueba.Len() != len(m.YPru) {
		t.Fatalf("tamaños: %d/%d vs %d/%d", datos.Entrenamiento.Len(), datos.Prueba.Len(), len(m.YEnt), len(m.YPru))
	}
	if datos.PesosClase != m.PesosClase {
		t.Fatal("pesos de clase distintos")
	}
	for i, fila := range datos.Prueba.X {
		for j, v := range fila {
			if v != m.XPru[i*m.D+j] {
				t.Fatalf("X prueba [%d][%d]", i, j)
			}
		}
		if datos.Prueba.EstacionID[i] != m.Estaciones[m.EstPru[i]] {
			t.Fatalf("estación de la fila %d", i)
		}
	}
	// La media de cada columna normalizada de entrenamiento debe ser ~0.
	for j := 0; j < m.D; j++ {
		s := 0.0
		for _, fila := range datos.Entrenamiento.X {
			s += fila[j]
		}
		if s/float64(datos.Entrenamiento.Len()) > 1e-9 || s/float64(datos.Entrenamiento.Len()) < -1e-9 {
			t.Fatalf("columna %d sin centrar", j)
		}
	}
}
