// Lanzador local (lanzador/cmd/saca): dice si esta laptop es maestro, nodo
// o ninguno, qué clústeres se anuncian en la red y si están los programas
// y los CSV. Sus botones lanzan el maestro o el trabajador en esta laptop.
import { useCallback, useEffect, useState } from 'react'

export function useLocal() {
  const [estado, setEstado] = useState({ disponible: null, datos: null })

  const actualizar = useCallback(async () => {
    try {
      const resp = await fetch('/local/estado', { cache: 'no-store' })
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
      setEstado({ disponible: true, datos: await resp.json() })
    } catch {
      // Sin lanzador (página publicada o `npm run dev` sin bin/saca).
      setEstado((s) => ({ disponible: s.datos ? s.disponible : false, datos: s.datos }))
    }
  }, [])

  useEffect(() => {
    let vigente = true
    const ciclo = async () => {
      if (!vigente) return
      await actualizar()
      if (vigente) setTimeout(ciclo, 2000)
    }
    ciclo()
    return () => {
      vigente = false
    }
  }, [actualizar])

  return { ...estado, actualizar }
}

async function post(ruta, cuerpo) {
  const resp = await fetch(ruta, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cuerpo ?? {}),
  })
  const datos = await resp.json().catch(() => ({}))
  if (!resp.ok) throw new Error(datos.error ?? `HTTP ${resp.status}`)
  return datos
}

export const apiLocal = {
  iniciar: (id, participar) => post('/local/iniciar', { id, participar }),
  unirse: (id, maestro) => post('/local/unirse', { id, maestro }),
  detener: () => post('/local/detener'),
}
