// Resultados de los núcleos en C (public/datos/experimento5..8.json). Los
// escriben scripts/nativo.sh y experimentos/cmd/{analisis,gpu} al terminar
// cada experimento. Aquí solo se leen: si un archivo no existe todavía, la
// pestaña lo dice; nunca se rellena con datos de ejemplo.
//
// El archivo se vuelve a pedir cada pocos segundos, así que una corrida que
// termina con la página abierta aparece sola, sin recargar.
import { useEffect, useState } from 'react'

export const ARCHIVOS_NATIVOS = ['experimento5', 'experimento6', 'experimento7', 'experimento8']
const CADA_MS = 5000

async function pedir(nombre) {
  try {
    const resp = await fetch(`${import.meta.env.BASE_URL}datos/${nombre}.json?t=${Date.now()}`, { cache: 'no-store' })
    if (!resp.ok) return null
    const texto = await resp.text()
    return { texto, datos: JSON.parse(texto) }
  } catch {
    return null // no existe (el servidor devuelve la página) o no es JSON
  }
}

export function useResultadosNativos() {
  const [estado, setEstado] = useState({ cargando: true, datos: {} })

  useEffect(() => {
    let vigente = true
    const textos = {}
    async function actualizar() {
      const resultados = await Promise.all(ARCHIVOS_NATIVOS.map(pedir))
      if (!vigente) return
      let cambio = false
      const datos = {}
      ARCHIVOS_NATIVOS.forEach((nombre, i) => {
        const r = resultados[i]
        datos[nombre] = r?.datos ?? null
        if ((r?.texto ?? null) !== (textos[nombre] ?? null)) cambio = true
        textos[nombre] = r?.texto ?? null
      })
      setEstado((anterior) => (cambio || anterior.cargando ? { cargando: false, datos } : anterior))
    }
    actualizar()
    const id = setInterval(actualizar, CADA_MS)
    return () => {
      vigente = false
      clearInterval(id)
    }
  }, [])

  return estado
}
