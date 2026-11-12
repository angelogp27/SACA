package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// proceso es un hijo del lanzador (maestro o trabajador) con sus últimas
// líneas de log, para mostrarlas en la página si algo falla.
type proceso struct {
	nombre string
	cmd    *exec.Cmd
	fin    chan struct{}

	mu     sync.Mutex
	lineas []string
	err    error
}

const maxLineas = 200

func ejecutable(dirBin, nombre string) string {
	if runtime.GOOS == "windows" {
		nombre += ".exe"
	}
	return filepath.Join(dirBin, nombre)
}

func lanzar(nombre, exe string, args ...string) (*proceso, error) {
	cmd := exec.Command(exe, args...)
	lector, escritor := io.Pipe()
	cmd.Stdout, cmd.Stderr = escritor, escritor
	p := &proceso{nombre: nombre, cmd: cmd, fin: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		s := bufio.NewScanner(lector)
		for s.Scan() {
			linea := s.Text()
			os.Stderr.WriteString("[" + nombre + "] " + linea + "\n")
			p.mu.Lock()
			p.lineas = append(p.lineas, linea)
			if len(p.lineas) > maxLineas {
				p.lineas = p.lineas[len(p.lineas)-maxLineas:]
			}
			p.mu.Unlock()
		}
	}()
	go func() {
		err := cmd.Wait()
		escritor.Close()
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(p.fin)
	}()
	return p, nil
}

func (p *proceso) vivo() bool {
	if p == nil {
		return false
	}
	select {
	case <-p.fin:
		return false
	default:
		return true
	}
}

func (p *proceso) ultimas(n int) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.lineas[max(0, len(p.lineas)-n):]...)
}

// detener pide un cierre ordenado (Ctrl+C: el trabajador se despide con
// "baja" y el maestro despide a sus nodos) y, si no termina, lo mata. En
// Windows no se puede mandar Ctrl+C a otro proceso: se mata directamente.
func (p *proceso) detener() {
	if !p.vivo() {
		return
	}
	if runtime.GOOS != "windows" {
		p.cmd.Process.Signal(os.Interrupt)
		select {
		case <-p.fin:
			return
		case <-time.After(4 * time.Second):
		}
	}
	p.cmd.Process.Kill()
	<-p.fin
}
