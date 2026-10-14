import { useState } from 'react'
import { experimentoOMock } from '../data/datosReales'
import './PanelRendimiento.css'

const ANCHO = 240
const ALTO = 170
const PAD = { top: 12, right: 12, bottom: 22, left: 30 }

// Nombre legible y color fijo de cada serie (el color sigue a la serie,
// no a su posición).
const SERIES = {
  concurrente: { etiqueta: '1 agregadora', color: 'var(--serie-1)' },
  concurrente_particionado: { etiqueta: 'N agregadoras', color: 'var(--serie-2)' },
  paralelo_reduccion: { etiqueta: 'Reducción', color: 'var(--serie-1)' },
  paralelo_mutex: { etiqueta: 'Mutex', color: 'var(--serie-2)' },
  paralelo_atomico: { etiqueta: 'Atómico (CAS)', color: 'var(--serie-3)' },
}

const PESTANAS = [
  { clave: 'experimento1', titulo: 'Cargador', unidad: 'goroutines' },
  { clave: 'experimento1b', titulo: 'Entrenamiento', unidad: 'goroutines' },
]

function Grafica({ titulo, series, unidades, valor, referencia, maxY, formatoY, aria }) {
  const escalaX = (i) => PAD.left + (unidades.length === 1 ? 0 : (i / (unidades.length - 1)) * (ANCHO - PAD.left - PAD.right))
  const escalaY = (v) => ALTO - PAD.bottom - (v / maxY) * (ALTO - PAD.top - PAD.bottom)
  const indice = (u) => unidades.indexOf(u)
  const ruta = (puntos, f) => puntos.map((p, i) => `${i === 0 ? 'M' : 'L'} ${escalaX(indice(p.unidades))} ${escalaY(f(p))}`).join(' ')

  return (
    <div className="mini-grafica">
      <h4>{titulo}</h4>
      <svg viewBox={`0 0 ${ANCHO} ${ALTO}`} role="img" aria-label={aria}>
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line x1={PAD.left} x2={ANCHO - PAD.right} y1={escalaY(f * maxY)} y2={escalaY(f * maxY)} className="gridline" />
            <text x={PAD.left - 5} y={escalaY(f * maxY)} className="tick-y">{formatoY(f * maxY)}</text>
          </g>
        ))}
        {referencia.map((r) => (
          <path key={r.clase} d={ruta(r.puntos, r.valor)} className={`linea ${r.clase}`} />
        ))}
        {series.map(([modo, puntos]) => (
          <g key={modo}>
            <path d={ruta(puntos, valor)} className="linea" style={{ stroke: SERIES[modo]?.color }} />
            {puntos.map((p) => (
              <g key={p.unidades}>
                <title>{`${SERIES[modo]?.etiqueta ?? modo}, ${p.unidades}: ${formatoY(valor(p))}`}</title>
                <circle cx={escalaX(indice(p.unidades))} cy={escalaY(valor(p))} r="5" className="punto" style={{ fill: SERIES[modo]?.color }} />
              </g>
            ))}
          </g>
        ))}
        {unidades.map((u, i) => (
          <text key={u} x={escalaX(i)} y={ALTO - 6} className="tick-x">{u}</text>
        ))}
      </svg>
    </div>
  )
}

export default function PanelRendimiento({ experimento1, experimento1b }) {
  const [pestana, setPestana] = useState('experimento1')
  const [verTabla, setVerTabla] = useState(false)
  const exp = experimentoOMock(pestana === 'experimento1' ? experimento1 : experimento1b)
  const series = Object.entries(exp.series ?? {}).sort(([a], [b]) => (a === exp.modo_paralelo ? -1 : b === exp.modo_paralelo ? 1 : a.localeCompare(b)))
  const unidades = [...new Set(series.flatMap(([, ps]) => ps.map((p) => p.unidades)))].sort((a, b) => a - b)
  const principal = exp.puntos
  const maxSpeedup = Math.max(...unidades, ...series.flatMap(([, ps]) => ps.map((p) => p.speedup)))

  const referenciaSpeedup = [{ clase: 'linea-ideal', puntos: unidades.map((u) => ({ unidades: u })), valor: (p) => p.unidades }]
  if (exp.real) referenciaSpeedup.push({ clase: 'linea-amdahl', puntos: principal, valor: (p) => p.speedup_amdahl })

  return (
    <figure className="panel panel-rendimiento">
      <figcaption>
        <h3>Rendimiento</h3>
        <p>
          {exp.real
            ? `${exp.experimento}: ${exp.descripcion}. T(1) secuencial = ${exp.t1_s.toFixed(exp.t1_s < 10 ? 2 : 1)} s, mediana de ${principal[0].repeticiones} corridas, ${exp.cpus_logicas} CPUs lógicas.`
            : 'Datos de ejemplo: ejecuta scripts/experimento1 para ver resultados reales.'}
        </p>
      </figcaption>

      <div className="pestanas" role="tablist">
        {PESTANAS.map((p) => (
          <button key={p.clave} role="tab" aria-selected={pestana === p.clave} className="pestana" onClick={() => setPestana(p.clave)}>
            {p.titulo}
          </button>
        ))}
      </div>

      <ul className="leyenda-series">
        {series.map(([modo]) => (
          <li key={modo}><span className="linea-key" style={{ background: SERIES[modo]?.color }} />{SERIES[modo]?.etiqueta ?? modo}</li>
        ))}
        <li><span className="linea-key linea-key-punteada" />Ideal S(p)=p</li>
        {exp.real && <li><span className="linea-key linea-key-amdahl" />Amdahl (f={exp.fraccion_serial.toFixed(2)})</li>}
      </ul>

      <div className="graficas-rendimiento">
        <Grafica
          titulo="Speedup S(p)"
          series={series}
          unidades={unidades}
          valor={(p) => p.speedup}
          referencia={referenciaSpeedup}
          maxY={Math.ceil(maxSpeedup)}
          formatoY={(v) => `${v.toFixed(1)}×`}
          aria="Speedup por número de goroutines frente al ideal y a la ley de Amdahl"
        />
        <Grafica
          titulo="Eficiencia E(p)"
          series={series}
          unidades={unidades}
          valor={(p) => p.eficiencia}
          referencia={[{ clase: 'linea-ideal', puntos: unidades.map((u) => ({ unidades: u })), valor: () => 1 }]}
          maxY={Math.max(1.05, ...series.flatMap(([, ps]) => ps.map((p) => p.eficiencia)))}
          formatoY={(v) => `${Math.round(v * 100)}%`}
          aria="Eficiencia por número de goroutines"
        />
      </div>

      <button className="ver-tabla" onClick={() => setVerTabla(!verTabla)} aria-expanded={verTabla}>
        {verTabla ? 'Ocultar tabla' : 'Ver tabla'}
      </button>
      {verTabla && (
        <div className="tabla-contenedor">
          <table>
            <thead>
              <tr><th>Serie</th><th>p</th><th>T(p) [s]</th><th>S(p)</th><th>E(p)</th></tr>
            </thead>
            <tbody>
              {series.flatMap(([modo, ps]) =>
                ps.map((p) => (
                  <tr key={`${modo}-${p.unidades}`}>
                    <td>{SERIES[modo]?.etiqueta ?? modo}</td>
                    <td>{p.unidades}</td>
                    <td>{p.t_total_s != null ? p.t_total_s.toFixed(p.t_total_s < 10 ? 3 : 1) : '—'}</td>
                    <td>{p.speedup.toFixed(2)}</td>
                    <td>{Math.round(p.eficiencia * 100)}%</td>
                  </tr>
                )),
              )}
            </tbody>
          </table>
        </div>
      )}
    </figure>
  )
}
