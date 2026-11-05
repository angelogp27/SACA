// Exporta la muestra de entrenamiento al formato binario que leen los
// núcleos nativos de nativo/ (docs/contrato_formato_binario.md, archivo 1)
// y entrena la versión secuencial en Go para dejar el resultado de
// referencia: la pérdida final de cualquier núcleo nativo debe coincidir
// con ella dentro de 1e-6.
//
//	go run ./herramientas/cmd/exportar-muestra -muestra data/muestra.gob -salida data/muestra.bin
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"proyecto9/internal/dataset"
	"proyecto9/internal/distribuido"
	"proyecto9/internal/logistica"
)

func main() {
	rutaMuestra := flag.String("muestra", "data/muestra.gob", "muestra en caché (la misma de los nodos)")
	salida := flag.String("salida", "data/muestra.bin", "archivo binario de salida; la referencia queda junto a él, en muestra_referencia.json")
	mesCorte := flag.Int("mes-corte", 10, "los meses >= mes-corte son de prueba")
	tasa := flag.Float64("tasa", 0.1, "tasa de aprendizaje de la referencia")
	epocas := flag.Int("epocas", 200, "épocas de la referencia")
	flag.Parse()

	datos, _, err := distribuido.PrepararLocal(*rutaMuestra, *mesCorte)
	if err != nil {
		log.Fatalf("no se pudo preparar la muestra: %v", err)
	}
	ent, prueba := datos.Entrenamiento, datos.Prueba
	d := len(ent.X[0])
	log.Printf("entrenamiento: %d ejemplos, prueba: %d, d=%d", len(ent.X), len(prueba.X), d)

	if err := escribir(*salida, d, datos.PesosClase, ent, prueba); err != nil {
		log.Fatal(err)
	}
	log.Printf("escrito %s", *salida)

	log.Printf("entrenando la referencia secuencial (%d épocas, tasa %g)...", *epocas, *tasa)
	inicio := time.Now()
	modelo := logistica.NuevoModelo(d)
	perdidas := modelo.Entrenar(ent.X, ent.Y, *tasa, *epocas, datos.PesosClase)
	log.Printf("referencia lista en %s, pérdida final %.6f", time.Since(inicio).Round(time.Second), perdidas[len(perdidas)-1])

	ref := map[string]any{"tasa": *tasa, "epocas": *epocas, "perdida_final": perdidas[len(perdidas)-1], "pesos": modelo.Pesos, "sesgo": modelo.Sesgo}
	rutaRef := filepath.Join(filepath.Dir(*salida), strings.TrimSuffix(filepath.Base(*salida), filepath.Ext(*salida))+"_referencia.json")
	b, _ := json.MarshalIndent(ref, "", "  ")
	if err := os.WriteFile(rutaRef, b, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("escrita la referencia %s", rutaRef)
}

func escribir(ruta string, d int, pc logistica.PesosClase, ent, prueba *dataset.Muestra) error {
	f, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)

	// Encabezado de 48 bytes (little-endian, sin relleno).
	enc := make([]byte, 48)
	copy(enc, "SACA")
	binary.LittleEndian.PutUint32(enc[4:], 1)
	binary.LittleEndian.PutUint32(enc[8:], 1) // tipo 1 = muestra
	binary.LittleEndian.PutUint32(enc[12:], uint32(d))
	binary.LittleEndian.PutUint64(enc[16:], uint64(len(ent.X)))
	binary.LittleEndian.PutUint64(enc[24:], uint64(len(prueba.X)))
	binary.LittleEndian.PutUint64(enc[32:], math.Float64bits(pc.Cero))
	binary.LittleEndian.PutUint64(enc[40:], math.Float64bits(pc.Uno))
	if _, err := w.Write(enc); err != nil {
		return err
	}

	buf := make([]byte, 8)
	real := func(v float64) error {
		binary.LittleEndian.PutUint64(buf, math.Float64bits(v))
		_, err := w.Write(buf)
		return err
	}
	for _, m := range []*dataset.Muestra{ent, prueba} {
		for _, fila := range m.X {
			if len(fila) != d {
				return fmt.Errorf("una fila tiene %d variables y se esperaban %d", len(fila), d)
			}
			for _, v := range fila {
				if err := real(v); err != nil {
					return err
				}
			}
		}
		for _, v := range m.Y {
			if err := real(v); err != nil {
				return err
			}
		}
	}
	return w.Flush()
}
