// Conexión en vivo con el maestro en modo servidor (entrenamiento/cmd/maestro
// -web). El estado llega por Server-Sent Events (/api/eventos): una
// instantánea completa con la sala de nodos, la corrida en curso época a
// época y el historial de la sesión. Los comandos (entrenar, cancelar,
// quitar un nodo, terminar la sesión) son POST a la misma API.
import { useEffect, useState } from 'react'

// activo = false no abre la conexión (p. ej. la laptop todavía no está en
// un clúster y el lanzador no tiene a quién reenviar /api).
export function useCluster(activo = true) {
  const [estado, setEstado] = useState({ conectado: false, inst: null })

  useEffect(() => {
    if (!activo) return undefined
    let fuente
    let reintento
    let vigente = true
    const abrir = () => {
      fuente = new EventSource('/api/eventos')
      fuente.onmessage = (e) => {
        try {
          setEstado({ conectado: true, inst: JSON.parse(e.data) })
        } catch {
          // un mensaje cortado: llega otro en 150 ms
        }
      }
      fuente.onerror = () => {
        fuente.close()
        if (!vigente) return
        setEstado((s) => ({ ...s, conectado: false }))
        reintento = setTimeout(abrir, 2000)
      }
    }
    abrir()
    return () => {
      vigente = false
      clearTimeout(reintento)
      fuente?.close()
    }
  }, [activo])

  return activo ? estado : SIN_CLUSTER
}

const SIN_CLUSTER = { conectado: false, inst: null }

async function post(ruta, cuerpo) {
  const resp = await fetch(ruta, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: cuerpo ? JSON.stringify(cuerpo) : undefined,
  })
  if (!resp.ok) {
    const datos = await resp.json().catch(() => ({}))
    throw new Error(datos.error ?? `HTTP ${resp.status}`)
  }
  return resp.json()
}

export const apiCluster = {
  entrenar: (nodos, epocas, verificar) => post('/api/corridas', { nodos, epocas, verificar }),
  // Limpieza: cada nodo limpia sus CSV y genera su muestra.bin; si ya hay
  // una en curso, los nodos se suman a ella.
  limpiar: (nodos, tecnologias = ['pthreads', 'go'], hilos = 0) => post('/api/limpiezas', { nodos, tecnologias, hilos }),
  pararLimpieza: (guardar) => post('/api/limpiezas/cancelar', { guardar }),
  verificarHuellas: () => post('/api/huellas/verificar'),
  cancelar: () => post('/api/corridas/cancelar'),
  quitar: (id) => post('/api/nodos/quitar', { id }),
  terminarSesion: () => post('/api/sesion/terminar'),
}

// Promedio de los últimos n valores (la gráfica en vivo se suaviza así).
export function promedioFinal(valores, n = 10) {
  if (!valores?.length) return 0
  const ultimos = valores.slice(-n)
  return ultimos.reduce((a, b) => a + b, 0) / ultimos.length
}

// esGPU: el motor del nodo calcula en una GPU NVIDIA (CUDA).
export const esGPU = (motor) => motor === 'cuda'
