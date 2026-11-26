//go:build !linux && !windows

package hardware

func ticsNucleos() []tics            { return nil }
func ramUso() (usada, total float64) { return 0, 0 }
