package hardware

func ticsNucleos() []tics            { return ticsLinux("/proc/stat") }
func ramUso() (usada, total float64) { return ramLinux("/proc/meminfo") }
