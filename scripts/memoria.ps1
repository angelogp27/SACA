# Comprobación de memoria compartida por cluster-local.ps1 y experimento2.ps1.
# Cada proceso (maestro o trabajador) carga la muestra entera: en memoria
# ocupa unas 4,5 veces lo que pesa el archivo (filas como slices, ids de
# estación y el pico al decodificar). Con la muestra completa (337 MB) son
# ~1,5 GB por proceso. Antes de lanzar N procesos se comprueba que quepan en
# la RAM libre, para no colgar la PC (el 15/10/2026 un -Nodos mal
# interpretado lanzó 124 trabajadores y agotó la memoria).

function Assert-MemoriaSuficiente {
    param([string]$Muestra, [int]$Procesos)
    $porProcesoMB = [int]((Get-Item $Muestra).Length / 1MB * 4.5) + 150
    $necesariaMB = $porProcesoMB * $Procesos
    $libreMB = [int]((Get-CimInstance Win32_OperatingSystem).FreePhysicalMemory / 1KB)
    # Se deja 1 GB de margen para el sistema y los demás programas.
    if ($necesariaMB -gt ($libreMB - 1024)) {
        throw ("memoria insuficiente: $Procesos procesos con $Muestra necesitan ~{0:N1} GB y hay {1:N1} GB libres. " +
            "Usa menos nodos, cierra programas o usa -Muestra data/muestra_prueba.gob") -f ($necesariaMB / 1024), ($libreMB / 1024)
    }
    Write-Host ("   memoria: {0} procesos x ~{1:N1} GB = ~{2:N1} GB de {3:N1} GB libres" -f $Procesos, ($porProcesoMB / 1024), ($necesariaMB / 1024), ($libreMB / 1024))
}

# ConvertTo-ListaNodos acepta "1,2,4", @(1,2,4) o "1 2 4". Pasado desde
# otra terminal, "1,2,4" llega como un solo texto y [int[]] lo convertía
# en el número 124.
function ConvertTo-ListaNodos {
    param($Valor)
    $lista = @("$Valor" -split '[,\s]+' | Where-Object { $_ -ne "" } | ForEach-Object { [int]$_ })
    foreach ($n in $lista) {
        if ($n -lt 1 -or $n -gt 32) { throw "número de nodos fuera de rango: $n (debe estar entre 1 y 32)" }
    }
    return $lista
}
