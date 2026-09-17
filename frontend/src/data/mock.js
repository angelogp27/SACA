// Datos de ejemplo para la maqueta de la Parte D. No vienen de la API ni
// del WebSocket todavía — eso entra recién semanas 9-11, cuando existan
// Parte B y C. La forma de estos datos sí sigue los contratos ya escritos
// en docs/.

export const estaciones = [
  { id: '06-037-0002', nombre: 'Los Angeles, CA', lat: 34.07, lon: -118.23, probabilidad: 0.81, contaminante: 'O3' },
  { id: '48-201-0024', nombre: 'Houston, TX', lat: 29.76, lon: -95.37, probabilidad: 0.64, contaminante: 'NO2' },
  { id: '17-031-0001', nombre: 'Chicago, IL', lat: 41.88, lon: -87.63, probabilidad: 0.22, contaminante: 'O3' },
  { id: '36-061-0010', nombre: 'New York, NY', lat: 40.71, lon: -74.01, probabilidad: 0.45, contaminante: 'SO2' },
  { id: '04-013-0019', nombre: 'Phoenix, AZ', lat: 33.45, lon: -112.07, probabilidad: 0.71, contaminante: 'O3' },
  { id: '08-031-0002', nombre: 'Denver, CO', lat: 39.74, lon: -104.99, probabilidad: 0.18, contaminante: 'CO' },
  { id: '53-033-0017', nombre: 'Seattle, WA', lat: 47.61, lon: -122.33, probabilidad: 0.09, contaminante: 'O3' },
  { id: '12-086-0027', nombre: 'Miami, FL', lat: 25.76, lon: -80.19, probabilidad: 0.31, contaminante: 'SO2' },
  { id: '13-121-0055', nombre: 'Atlanta, GA', lat: 33.75, lon: -84.39, probabilidad: 0.53, contaminante: 'NO2' },
  { id: '42-101-0075', nombre: 'Philadelphia, PA', lat: 39.95, lon: -75.16, probabilidad: 0.38, contaminante: 'O3' },
]

export function nivelRiesgo(probabilidad) {
  if (probabilidad >= 0.6) return 'critical'
  if (probabilidad >= 0.3) return 'warning'
  return 'good'
}

export const etiquetaNivel = {
  good: 'Bueno',
  warning: 'Moderado',
  critical: 'Alerta',
}

// Línea de tiempo de 24h para la estación seleccionada: concentración
// observada vs. la predicha por el modelo.
export function lineaTiempoMock(estacionId) {
  const base = 20 + (estacionId.charCodeAt(1) % 15)
  const horas = Array.from({ length: 24 }, (_, h) => h)
  return horas.map((h) => {
    const ciclo = Math.sin((h - 6) / 24 * 2 * Math.PI) * 12
    const observado = Math.max(2, base + ciclo + Math.sin(h * 1.7) * 3)
    const predicho = Math.max(2, observado + Math.sin(h * 0.9) * 4 - 1.5)
    return { hora: h, observado: round1(observado), predicho: round1(predicho) }
  })
}

function round1(n) {
  return Math.round(n * 10) / 10
}

export const nodosCluster = [
  { id: 'w1', estado: 'activo', cpu: 62, ram: 512 },
  { id: 'w2', estado: 'activo', cpu: 58, ram: 498 },
  { id: 'w3', estado: 'activo', cpu: 71, ram: 540 },
  { id: 'w4', estado: 'inactivo', cpu: 0, ram: 0 },
]

export const epocaActual = 7
export const perdidaPorEpocaMock = [0.693, 0.41, 0.31, 0.26, 0.22, 0.19, 0.17, 0.155]

// Curva de speedup de ejemplo (forma plausible con la ley de Amdahl, no
// medida todavía con múltiples goroutines — el Experimento 1 real es de la
// semana 7).
export const experimentoSpeedupMock = [
  { unidades: 1, speedup: 1 },
  { unidades: 2, speedup: 1.8 },
  { unidades: 4, speedup: 3.1 },
  { unidades: 8, speedup: 4.6 },
  { unidades: 16, speedup: 5.7 },
]
