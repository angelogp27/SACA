import { estaciones, nivelRiesgo, etiquetaNivel } from '../data/mock'
import './PanelMapa.css'

const LON_MIN = -125
const LON_MAX = -66
const LAT_MIN = 24
const LAT_MAX = 50
const ANCHO = 480
const ALTO = 260

function proyectar(lat, lon) {
  const x = ((lon - LON_MIN) / (LON_MAX - LON_MIN)) * ANCHO
  const y = ALTO - ((lat - LAT_MIN) / (LAT_MAX - LAT_MIN)) * ALTO
  return { x, y }
}

const colorPorNivel = {
  good: 'var(--status-good)',
  warning: 'var(--status-warning)',
  critical: 'var(--status-critical)',
}

export default function PanelMapa({ seleccionada, onSeleccionar }) {
  return (
    <figure className="panel panel-mapa">
      <figcaption>
        <h3>Mapa de estaciones</h3>
        <p>Riesgo previsto para las próximas horas</p>
      </figcaption>

      <svg viewBox={`0 0 ${ANCHO} ${ALTO}`} role="img" aria-label="Mapa de estaciones de monitoreo coloreadas por nivel de riesgo">
        <rect x="0" y="0" width={ANCHO} height={ALTO} fill="var(--surface)" />
        {estaciones.map((e) => {
          const { x, y } = proyectar(e.lat, e.lon)
          const nivel = nivelRiesgo(e.probabilidad)
          const activa = e.id === seleccionada
          return (
            <g
              key={e.id}
              className="estacion"
              onClick={() => onSeleccionar(e.id)}
              tabIndex={0}
              role="button"
              aria-pressed={activa}
            >
              <title>{`${e.nombre} · ${e.contaminante} · ${etiquetaNivel[nivel]} (${Math.round(e.probabilidad * 100)}%)`}</title>
              <circle cx={x} cy={y} r={activa ? 8 : 6} fill="var(--surface)" />
              <circle cx={x} cy={y} r={activa ? 6 : 4.5} fill={colorPorNivel[nivel]} />
            </g>
          )
        })}
      </svg>

      <ul className="leyenda-estado">
        <li><span className="pastilla" style={{ background: 'var(--status-good)' }} />Bueno</li>
        <li><span className="pastilla" style={{ background: 'var(--status-warning)' }} />Moderado</li>
        <li><span className="pastilla" style={{ background: 'var(--status-critical)' }} />Alerta</li>
      </ul>
    </figure>
  )
}
