// Resultados guardados del Experimento 2 (resultados/experimento2*.csv →
// experimentos/cmd/analisis → public/datos/experimento2*.json): cuánto
// tarda cada época en cálculo y en red según la configuración del clúster.
import { BarrasH } from './Graficas'

const CAPAS = [
  { clave: 'calculo', nombre: 'Cálculo del nodo más lento', color: 'var(--serie-3)' },
  { clave: 'red', nombre: 'Red y espera', color: 'var(--serie-2)' },
]

const fmt = (v, d = 1) => v.toLocaleString('es-PE', { minimumFractionDigits: d, maximumFractionDigits: d })

function filaDe(p, clave, etiqueta, destacada) {
  const calculo = p.fases?.t_calculo_ms ?? 0
  const red = p.fases?.t_comunicacion_ms ?? 0
  const pct = calculo + red > 0 ? Math.round((100 * red) / (calculo + red)) : 0
  return {
    clave,
    etiqueta,
    detalle: p.maquina,
    destacada,
    valores: { calculo, red },
    extra: `${fmt(p.t_total_s, 2)} s en total · S = ${fmt(p.speedup)} · ${pct} % de red · mediana de ${p.repeticiones}`,
  }
}

const limpiar = (m) => (m ?? '').replace(/,?\s*muestra completa/i, '').trim()

export default function ResultadosCluster({ experimento2, experimento2Prueba, compacto = false }) {
  const completa = experimento2?.series?.tcp_tailscale ?? []
  const prueba = experimento2Prueba?.series?.tcp_tailscale ?? []
  if (!completa.length && !prueba.length) {
    return <p>No hay resultados guardados del Experimento 2 todavía.</p>
  }
  const mejor = completa.reduce((a, p) => (a && a.speedup > p.speedup ? a : p), null)
  const filas = [
    ...prueba.map((p) => filaDe(p, `prueba-${p.unidades}`, `Muestra de prueba · ${p.unidades} nodo CPU`, false)),
    ...completa.map((p) => filaDe(p, `completa-${p.unidades}`, limpiar(p.maquina) || `${p.unidades} nodos`, p === mejor)),
  ]
  const t1 = experimento2?.fases_base
  return (
    <div className="resultados-cluster">
      <BarrasH
        titulo="Tiempo por época según la configuración (ms)"
        tema="Sem. 1 y 13 · Speedup y evaluación empírica"
        filas={filas}
        capas={CAPAS}
      />
      {!compacto && t1 && (
        <p className="nota-resultados">
          Referencia: el entrenamiento secuencial (1 hilo de CPU, PC del maestro) con la muestra completa tarda{' '}
          <strong>{fmt(t1.t_epoca_ms, 0)} ms por época</strong> ({fmt(t1.t_total_s)} s en total). Pérdida final idéntica en
          todas las corridas ({t1.perdida_final}). Con la muestra de prueba (146 mil ejemplos) la red domina y no conviene
          distribuir; con la completa (2,5 M), una GPU remota entrena {mejor ? fmt(mejor.speedup) : '—'} veces más rápido.
          La eficiencia supera el 100 % porque los nodos son GPUs y la base es un hilo de CPU: Amdahl y Karp-Flatt no aplican aquí.
        </p>
      )}
    </div>
  )
}
