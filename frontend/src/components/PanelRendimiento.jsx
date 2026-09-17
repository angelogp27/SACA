import { experimentoSpeedupMock } from '../data/mock'
import './PanelRendimiento.css'

const ANCHO = 220
const ALTO = 160
const PAD = { top: 12, right: 12, bottom: 22, left: 28 }

function escalasIndice(n) {
  return (i) => PAD.left + (i / (n - 1)) * (ANCHO - PAD.left - PAD.right)
}

function escalaY(min, max) {
  return (v) => ALTO - PAD.bottom - ((v - min) / (max - min || 1)) * (ALTO - PAD.top - PAD.bottom)
}

export default function PanelRendimiento() {
  const datos = experimentoSpeedupMock
  const ideal = datos.map((d) => d.unidades)
  const escalaX = escalasIndice(datos.length)

  const speedupMax = Math.max(...ideal, ...datos.map((d) => d.speedup))
  const escYSpeedup = escalaY(0, speedupMax)

  const eficiencia = datos.map((d) => d.speedup / d.unidades)
  const escYEficiencia = escalaY(0, 1.05)

  return (
    <figure className="panel panel-rendimiento">
      <figcaption>
        <h3>Rendimiento</h3>
        <p>Speedup y eficiencia del cargador, Experimento 1 (ejemplo)</p>
      </figcaption>

      <div className="graficas-rendimiento">
        <div className="mini-grafica">
          <h4>Speedup S(p)</h4>
          <svg viewBox={`0 0 ${ANCHO} ${ALTO}`} role="img" aria-label="Speedup real comparado con el speedup ideal, por numero de goroutines">
            {[0, 1].map((f) => (
              <line key={f} x1={PAD.left} x2={ANCHO - PAD.right} y1={escYSpeedup(f * speedupMax)} y2={escYSpeedup(f * speedupMax)} className="gridline" />
            ))}
            <path d={datos.map((d, i) => `${i === 0 ? 'M' : 'L'} ${escalaX(i)} ${escYSpeedup(ideal[i])}`).join(' ')} className="linea linea-ideal" />
            <path d={datos.map((d, i) => `${i === 0 ? 'M' : 'L'} ${escalaX(i)} ${escYSpeedup(d.speedup)}`).join(' ')} className="linea linea-real" />
            {datos.map((d, i) => (
              <g key={d.unidades}>
                <title>{`${d.unidades} goroutines: speedup ${d.speedup}x`}</title>
                <circle cx={escalaX(i)} cy={escYSpeedup(d.speedup)} r="3" className="punto-real" />
              </g>
            ))}
            {datos.map((d, i) => (
              <text key={d.unidades} x={escalaX(i)} y={ALTO - 6} className="tick-x">{d.unidades}</text>
            ))}
          </svg>
          <ul className="leyenda-series">
            <li><span className="linea-key" style={{ background: 'var(--serie-1)' }} />Real</li>
            <li><span className="linea-key linea-key-punteada" />Ideal S(p)=p</li>
          </ul>
        </div>

        <div className="mini-grafica">
          <h4>Eficiencia E(p)</h4>
          <svg viewBox={`0 0 ${ANCHO} ${ALTO}`} role="img" aria-label="Eficiencia por numero de goroutines">
            <line x1={PAD.left} x2={ANCHO - PAD.right} y1={escYEficiencia(1)} y2={escYEficiencia(1)} className="gridline" />
            <path d={datos.map((d, i) => `${i === 0 ? 'M' : 'L'} ${escalaX(i)} ${escYEficiencia(eficiencia[i])}`).join(' ')} className="linea linea-real" />
            {datos.map((d, i) => (
              <g key={d.unidades}>
                <title>{`${d.unidades} goroutines: eficiencia ${(eficiencia[i] * 100).toFixed(0)}%`}</title>
                <circle cx={escalaX(i)} cy={escYEficiencia(eficiencia[i])} r="3" className="punto-real" />
              </g>
            ))}
            {datos.map((d, i) => (
              <text key={d.unidades} x={escalaX(i)} y={ALTO - 6} className="tick-x">{d.unidades}</text>
            ))}
          </svg>
        </div>
      </div>
    </figure>
  )
}
