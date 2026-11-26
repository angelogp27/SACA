//go:build !windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func activarColores() {}

func anchoTerminal() int {
	var ws struct{ filas, cols, x, y uint16 }
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws))); e != 0 || ws.cols == 0 {
		return 100
	}
	return int(ws.cols)
}
