// CSV de cada fase, armados en la página con los mismos datos que se ven
// (en vivo o del historial). Separador coma, decimales con punto.

function celda(v) {
  if (v == null) return ''
  const s = typeof v === 'number' ? (Number.isFinite(v) ? String(Math.round(v * 1e6) / 1e6) : '') : String(v)
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

export function descargarCSV(nombre, filas) {
  const texto = filas.map((f) => f.map(celda).join(',')).join('\n') + '\n'
  const url = URL.createObjectURL(new Blob([texto], { type: 'text/csv;charset=utf-8' }))
  const a = document.createElement('a')
  a.href = url
  a.download = nombre
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

const promedio = (serie) => (serie?.length ? serie.reduce((a, p) => a + p[1], 0) / serie.length : null)

// Limpieza: una fila por nodo y tecnología.
export function csvLimpieza(l) {
  const filas = [['limpieza', 'nodo', 'procesador', 'tecnologia', 'estado', 'hilos', 't_limpieza_s', 't_features_s', 't_total_s', 'cpu_media_limpieza', 'cpu_media_features', 'huella', 'mensaje']]
  for (const n of l.nodos ?? []) {
    for (const t of n.tecnologias ?? []) {
      filas.push([l.id, n.id, n.hardware?.cpu, t.tecnologia, t.estado, t.hilos, t.t_limpieza_s, t.t_variables_s, t.t_total_s,
        promedio(t.serie), promedio(t.serie_variables), t.huella, t.mensaje])
    }
  }
  return filas
}

// Entrenamiento: una fila por época, con el cálculo y la parte de las
// filas de cada nodo.
export function csvEntrenamiento(c) {
  const nodos = c.nodos ?? []
  const filas = [[
    'epoca', 'perdida', 't_epoca_ms', 't_calculo_ms', 't_red_ms', 'mb_red',
    ...nodos.flatMap((n) => [`calculo_ms_${n.id}`, `fraccion_${n.id}`]),
  ]]
  const total = (c.perdidas ?? []).length
  for (let e = 0; e < total; e++) {
    filas.push([
      e + 1, c.perdidas[e], c.t_epoca_ms?.[e], c.t_calculo_ms?.[e], c.t_red_ms?.[e], c.mb_epoca?.[e],
      ...nodos.flatMap((n, i) => [n.calculo_ms?.[e], c.fraccion?.[e]?.[i]]),
    ])
  }
  return filas
}

// Predicción: un resumen por nodo (las 3,3 millones de filas predichas no:
// pesarían ~150 MB; ya están en el mapa y en la línea de tiempo).
export function csvPrediccion(p) {
  const filas = [['prediccion', 'modelo', 'nodo', 'estado', 'filas', 'mb_enviados', 't_calculo_s', 't_hasta_ultimo_trozo_s', 'mensaje']]
  for (const n of p.nodos ?? []) {
    filas.push([p.id, p.modelo, n.id, n.estado, n.filas, n.mb, (n.t_calculo_ms ?? 0) / 1000, (n.t_total_ms ?? 0) / 1000, n.mensaje])
  }
  filas.push([p.id, p.modelo, 'total', p.estado, p.filas, p.mb, '', p.t_total_s, p.mensaje])
  return filas
}
