// Carga los datos reales que exporta el backend en public/datos (ver
// herramientas/cmd/exportar y experimentos/cmd/analisis). Mientras no
// exista la API (Parte C, semanas 9-11) son archivos estáticos; si alguno
// falta, el panel correspondiente usa los datos de ejemplo de mock.js.
import { useEffect, useState } from 'react'
import { estaciones as estacionesMock, lineaTiempoMock, experimentoSpeedupMock } from './mock'

const ARCHIVOS = ['estaciones', 'series', 'resumen', 'experimento1', 'experimento1b']

async function cargar(nombre) {
  const resp = await fetch(`${import.meta.env.BASE_URL}datos/${nombre}.json`)
  if (!resp.ok) throw new Error(`${nombre}: HTTP ${resp.status}`)
  return resp.json()
}

export function useDatosReales() {
  const [estado, setEstado] = useState({ cargando: true })

  useEffect(() => {
    let vigente = true
    Promise.allSettled(ARCHIVOS.map(cargar)).then((resultados) => {
      if (!vigente) return
      const datos = { cargando: false }
      ARCHIVOS.forEach((nombre, i) => {
        datos[nombre] = resultados[i].status === 'fulfilled' ? resultados[i].value : null
      })
      setEstado(datos)
    })
    return () => {
      vigente = false
    }
  }, [])

  return estado
}

// Estaciones con la forma que usan los paneles. Con datos reales, el
// riesgo es el índice histórico (p98 horario / umbral AQI 51); con el
// mock, la probabilidad de ejemplo.
export function estacionesParaPaneles(estaciones) {
  if (estaciones?.length) {
    return {
      real: true,
      lista: estaciones.map((e) => ({
        id: e.id,
        nombre: e.nombre,
        detalle: [e.ciudad, e.estado].filter(Boolean).join(', '),
        lat: e.lat,
        lon: e.lon,
        indice: e.indice,
        contaminante: e.contaminante,
        gases: e.gases,
      })),
    }
  }
  return {
    real: false,
    lista: estacionesMock.map((e) => ({ ...e, detalle: '', indice: e.probabilidad / 0.6 })),
  }
}

export function serieDeEstacion(series, estacion) {
  const real = series?.[estacion.id]
  if (real) return { real: true, ...real }
  const mock = lineaTiempoMock(estacion.id)
  return {
    real: false,
    fecha: 'ejemplo',
    contaminante: estacion.contaminante,
    unidad: 'ppb',
    umbral: 53,
    observado: mock.map((d) => d.observado),
    probabilidad: null,
  }
}

export function experimentoOMock(exp) {
  if (exp?.puntos?.length) return { real: true, ...exp }
  return {
    real: false,
    experimento: 'Experimento 1 (ejemplo)',
    modo_paralelo: 'concurrente',
    series: { concurrente: experimentoSpeedupMock.map((d) => ({ ...d, eficiencia: d.speedup / d.unidades })) },
    puntos: experimentoSpeedupMock,
  }
}
