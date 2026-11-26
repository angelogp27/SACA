package main

import (
	"os"
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

// activarColores: la consola clásica de Windows (PowerShell, cmd) solo
// entiende los códigos ANSI con ENABLE_VIRTUAL_TERMINAL_PROCESSING.
func activarColores() {
	h := syscall.Handle(os.Stdout.Fd())
	var modo uint32
	if syscall.GetConsoleMode(h, &modo) == nil {
		kernel32.NewProc("SetConsoleMode").Call(uintptr(h), uintptr(modo|0x0004))
	}
}

func anchoTerminal() int {
	var info struct {
		tam, cursor             [2]int16
		atributos               uint16
		izq, arriba, der, abajo int16
		maximo                  [2]int16
	}
	r, _, _ := kernel32.NewProc("GetConsoleScreenBufferInfo").Call(os.Stdout.Fd(), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 100
	}
	return int(info.der-info.izq) + 1
}
