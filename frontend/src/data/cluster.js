// Conexión en vivo con el maestro en modo servidor (entrenamiento/cmd/maestro
// -web). El estado llega por Server-Sent Events (/api/eventos): una
// instantánea completa con la sala de nodos, la corrida en curso época a
// época y el historial de la sesión. Los comandos (entrenar, cancelar,
// quitar un nodo, terminar la sesión) son POST a la misma API.
import { useEffect, useState } from 'react'

export function useCluster() {
  const [estado, setEstado] = useState({ conectado: false, inst: null })

  useEffect(() => {
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
  }, [])

  return estado
}

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
