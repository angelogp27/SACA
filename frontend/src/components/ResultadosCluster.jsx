// Resultados guardados del Experimento 2 (resultados/experimento2*.csv →
// experimentos/cmd/analisis → public/datos/experimento2*.json): cuánto
// tarda cada época en cálculo y en red según la combinación de nodos.
// Cada serie del JSON es una combinación (p. ej.
// "tcp_tailscale:augusto-pc+josue"); se dibuja una barra por combinación.
import { BarrasH } from './Graficas'

const CAPAS = [
  { clave: 'calculo', nombre: 'Cálculo del nodo más lento', color: 'var(--serie-3)' },
  { clave: 'red', nombre: 'Red y espera', color: 'var(--serie-2)' },
]

const fmt = (v, d = 1) => (Number.isFinite(v) ? v.toLocaleString('es-PE', { minimumFractionDigits: d, maximumFractionDigits: d }) : '—')

// etiqueta convierte "augusto-pc(cuda)+angelo(go) · 2499636 ejemplos" en
// "augusto-pc (GPU) + angelo (CPU)". Las descripciones antiguas se limpian.
function etiquetaCombinacion(maquina, unidades) {
  const texto = (maquina ?? '').replace(/^web:\s*/, '').split(' · ')[0]
  const nodos = [...texto.matchAll(/([\w.-]+)\((cuda|go)\)/g)]
  if (nodos.length) return nodos.map(([, id, motor]) => `${id} (${motor === 'cuda' ? 'GPU' : 'CPU'})`).join(' + ')
  return texto.replace(/,?\s*muestra completa/i, '').trim() || `${unidades} nodo(s)`
}

// puntosCluster junta los puntos de todas las series distribuidas del JSON
// (todas menos la base secuencial), ordenados por cantidad de nodos.
function puntosCluster(exp) {
  if (!exp?.series) return []
  return Object.entries(exp.series)
    .filter(([modo]) => modo.startsWith('tcp'))
    .flatMap(([modo, ps]) => ps.map((p) => ({ ...p, modo })))
    .sort((a, b) => a.unidades - b.unidades || a.t_total_s - b.t_total_s)
}

function filaDe(p, prefijo, destacada) {
  const calculo = p.fases?.t_calculo_ms ?? 0
  const red = p.fases?.t_comunicacion_ms ?? 0
  const pct = calculo + red > 0 ? Math.round((100 * red) / (calculo + red)) : 0
  return {
    clave: `${prefijo}-${p.modo}-${p.unidades}`,
    etiqueta: `${prefijo === 'prueba' ? 'Muestra de prueba · ' : ''}${etiquetaCombinacion(p.maquina, p.unidades)}`,
    detalle: p.maquina,
    destacada,
    valores: { calculo, red },
    extra: `${fmt(p.t_total_s, 2)} s en total · S = ${fmt(p.speedup)} · ${pct} % de red · ${p.repeticiones} corrida${p.repeticiones === 1 ? '' : 's'}`,
  }
}

export default function ResultadosCluster({ experimento2, experimento2Prueba, compacto = false }) {
  const completa = puntosCluster(experimento2)
  const prueba = puntosCluster(experimento2Prueba).filter((p) => p.modo.startsWith('tcp_tailscale'))
  if (!completa.length && !prueba.length) {
    return <p>No hay resultados guardados del Experimento 2 todavía. Lanza corridas desde la página y genera la tabla (guía del maestro, paso 8).</p>
  }
  const mejor = completa.reduce((a, p) => (a && a.speedup >= p.speedup ? a : p), null)
  const filas = [...prueba.map((p) => filaDe(p, 'prueba', false)), ...completa.map((p) => filaDe(p, 'completa', p === mejor))]
  const t1 = experimento2?.fases_base
  return (
    <div className="resultados-cluster">
      <BarrasH titulo="Tiempo por época según la combinación de nodos (ms)" tema="Sem. 1 y 13 · Speedup y evaluación empírica" filas={filas} capas={CAPAS} />
      {!compacto && t1 && (
        <p className="nota-resultados">
          Referencia: el entrenamiento secuencial (1 hilo de CPU, PC del maestro) con la muestra completa tarda{' '}
          <strong>{fmt(t1.t_epoca_ms, 0)} ms por época</strong> ({fmt(t1.t_total_s)} s en total). Pérdida final idéntica en
          todas las corridas ({t1.perdida_final}). Con la muestra de prueba (146 mil ejemplos) la red domina y no conviene
          distribuir; con la completa (2,5 M), la mejor combinación ({mejor ? etiquetaCombinacion(mejor.maquina, mejor.unidades) : '—'}) entrena{' '}
          {mejor ? fmt(mejor.speedup) : '—'} veces más rápido. La eficiencia supera el 100 % porque los nodos son GPUs y la base es
          un hilo de CPU: Amdahl y Karp-Flatt no aplican aquí.
        </p>
      )}
    </div>
  )
}
