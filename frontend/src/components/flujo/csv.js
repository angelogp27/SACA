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

// Entrenamiento: una fila por época (y motor, si se compararon), con el
// cálculo, la CPU, la parte de la GPU y la parte de las filas de cada nodo.
export function csvEntrenamiento(c) {
  const nodos = c.nodos ?? []
  const filas = [[
    'motor', 'epoca', 'perdida', 't_epoca_ms', 't_calculo_ms', 't_red_ms', 'mb_red',
    ...nodos.flatMap((n) => [`calculo_ms_${n.id}`, `cpu_${n.id}`, `gpu_${n.id}`, `fraccion_${n.id}`]),
  ]]
  for (const k of [...(c.anteriores ?? []), c]) {
    const motor = k.motor_cpu ? `${k.motor_cpu}${k.cuda ? '+cuda' : ''}` : ''
    for (let e = 0; e < (k.perdidas ?? []).length; e++) {
      filas.push([
        motor, e + 1, k.perdidas[e], k.t_epoca_ms?.[e], k.t_calculo_ms?.[e], k.t_red_ms?.[e], k.mb_epoca?.[e],
        ...(k.nodos ?? []).flatMap((n, i) => [n.calculo_ms?.[e], n.cpu?.[e], n.gpu?.[e], k.fraccion?.[e]?.[i]]),
      ])
    }
    // La verificación: el mismo entrenamiento en secuencial (1 hilo) en la
    // laptop del maestro; su época es la que se comparó.
    const r = k.resultado
    if (r?.verificado) filas.push([`secuencial en ${r.secuencial_en ?? 'el maestro'}`, r.epocas_secuencial, r.perdida_secuencial, r.t1_epoca_ms, r.t1_epoca_ms, 0, 0])
  }
  return filas
}

// Predicción: un resumen por nodo (las 3,3 millones de filas predichas no:
// pesarían ~150 MB; ya están en el mapa y en la línea de tiempo).
export function csvPrediccion(ultima) {
  const filas = [['prediccion', 'modelo', 'comunicacion', 'nodo', 'motor', 'parte_gpu', 'estado', 'filas', 'mb_enviados', 't_calculo_s', 't_hasta_ultimo_trozo_s', 'mensaje']]
  // Con la comparación de motores, una tanda de filas por motor.
  for (const p of [...(ultima.anteriores ?? []), ultima]) {
    for (const n of p.nodos ?? []) {
      filas.push([p.id, p.modelo, p.comunicacion, n.id, n.motor, n.parte_gpu, n.estado, n.filas, n.mb, (n.t_calculo_ms ?? 0) / 1000, (n.t_total_ms ?? 0) / 1000, n.mensaje])
    }
    filas.push([p.id, p.modelo, p.comunicacion, 'total', p.motor_cpu, '', p.estado, p.filas, p.mb, '', p.t_total_s, p.mensaje])
  }
  return filas
}
