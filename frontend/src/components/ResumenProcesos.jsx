// Resultados finales: el último proceso de cada fase (del historial del
// maestro) con sus números clave. Sin clúster no hay historial que leer y
// el resumen no se muestra.
import { useEffect, useState } from 'react'
import { apiCluster } from '../data/cluster'

const TIPOS = [
  { id: 'limpieza', titulo: 'Limpieza e ingeniería de features' },
  { id: 'entrenamiento', titulo: 'Entrenamiento' },
  { id: 'prediccion', titulo: 'Predicción distribuida' },
]
const n = (v, d = 0) => (v == null || Number.isNaN(v) ? '—' : v.toLocaleString('es-PE', { maximumFractionDigits: d, minimumFractionDigits: d }))
const media = (s) => (s?.length ? s.reduce((a, b) => a + b, 0) / s.length : null)

function datosLimpieza(l) {
  // Por tecnología, el nodo más lento (el que marca cuándo termina el clúster).
  return (l.tecnologias ?? []).map((tec) => {
    const ts = (l.nodos ?? []).map((nd) => nd.tecnologias.find((t) => t.tecnologia === tec)).filter((t) => t?.estado === 'terminada')
    const peor = ts.reduce((a, t) => (t.t_total_s > (a?.t_total_s ?? -1) ? t : a), null)
    return [{ pthreads: 'Pthreads', go: 'Go' }[tec] ?? tec, peor ? `${n(peor.t_limpieza_s, 1)} s + ${n(peor.t_variables_s, 1)} s` : '—']
  })
}

function datosEntrenamiento(c) {
  const r = c.resultado ?? {}
  return [
    ['Total', `${n(r.t_total_s, 1)} s`],
    ['Por época', `${n(media(c.t_epoca_ms))} ms`],
    ['Red y espera', `${n(media(c.t_red_ms))} ms por época`],
    ['Por la red', c.mb_epoca?.length ? `${n(c.mb_epoca.reduce((a, b) => a + b, 0), 1)} MB` : '—'],
    ['Speedup', r.speedup > 0 ? `${n(r.speedup, 2)}×` : '—'],
    ['AUC / F1', `${n(r.auc, 3)} / ${n(r.f1, 3)}`],
  ]
}

function datosPrediccion(p) {
  return [
    ['Filas', n(p.filas)],
    ['Por la red', `${n(p.mb, 1)} MB`],
    ['Tiempo', `${n(p.t_total_s, 2)} s`],
  ]
}

const ARMAR = { limpieza: datosLimpieza, entrenamiento: datosEntrenamiento, prediccion: datosPrediccion }

export default function ResumenProcesos({ activo, version }) {
  const [ultimos, setUltimos] = useState({})
  useEffect(() => {
    if (!activo) return undefined
    let vigente = true
    Promise.all(TIPOS.map(async ({ id }) => {
      const lista = await apiCluster.historial(id)
      return [id, lista[0] ? await apiCluster.historialDe(lista[0].id) : null]
    }))
      .then((pares) => vigente && setUltimos(Object.fromEntries(pares)))
      .catch(() => vigente && setUltimos({}))
    return () => {
      vigente = false
    }
  }, [activo, version])
  if (!activo || !Object.values(ultimos).some(Boolean)) return null
  return (
    <section className="panel resumen-procesos" aria-label="Último proceso de cada fase">
      <h2>Últimos procesos del clúster</h2>
      <div className="resumen-procesos-tarjetas">
        {TIPOS.map(({ id, titulo }) => {
          const e = ultimos[id]
          return (
            <article key={id} className="resumen-proceso">
              <h3>{titulo}</h3>
              {e ? (
                <>
                  <p className="detalle">{e.descripcion}</p>
                  <p className="detalle">{new Date(e.fecha).toLocaleString('es-PE')} · maestro {e.maestro}</p>
                  <dl>
                    {ARMAR[id](e.datos).map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
                  </dl>
                </>
              ) : (
                <p className="detalle">Sin procesos guardados.</p>
              )}
            </article>
          )
        })}
      </div>
    </section>
  )
}
