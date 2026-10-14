import { useState } from 'react'
import './PanelMapa.css'

// Proyección equirectangular simple del territorio continental de EE. UU.
// Las estaciones fuera de este recuadro (Alaska, Hawái, Puerto Rico) se
// listan aparte en vez de dibujarse.
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

function dentroDelMapa(e) {
  return e.lat >= LAT_MIN && e.lat <= LAT_MAX && e.lon >= LON_MIN && e.lon <= LON_MAX
}

// Índice = percentil 98 horario / umbral de AQI 51 del gas dominante.
function nivelRiesgo(indice) {
  if (indice >= 1) return 'critical'
  if (indice >= 0.5) return 'warning'
  return 'good'
}

const NIVELES = {
  good: { etiqueta: 'Bajo', icono: '●', color: 'var(--status-good)', detalle: 'p98 < 50% del umbral' },
  warning: { etiqueta: 'Moderado', icono: '▲', color: 'var(--status-warning)', detalle: '50-100% del umbral' },
  critical: { etiqueta: 'Alto', icono: '◆', color: 'var(--status-critical)', detalle: 'p98 ≥ umbral AQI 51' },
}

export default function PanelMapa({ estaciones, seleccionada, onSeleccionar }) {
  const [hover, setHover] = useState(null)
  const visibles = estaciones.lista.filter(dentroDelMapa)
  const fuera = estaciones.lista.length - visibles.length
  // Las de mayor riesgo se dibujan al final para quedar encima.
  const ordenadas = [...visibles].sort((a, b) => a.indice - b.indice)
  const conteo = { good: 0, warning: 0, critical: 0 }
  estaciones.lista.forEach((e) => conteo[nivelRiesgo(e.indice)]++)
  const radio = estaciones.real ? 2.6 : 6
  const enfocada = hover ?? estaciones.lista.find((e) => e.id === seleccionada)

  return (
    <figure className="panel panel-mapa">
      <figcaption>
        <h3>Mapa de estaciones</h3>
        <p>
          {estaciones.real
            ? `${estaciones.lista.length} estaciones · riesgo histórico 2025 (percentil 98 horario frente al umbral AQI 51)`
            : 'Riesgo previsto (datos de ejemplo)'}
        </p>
      </figcaption>

      <svg viewBox={`0 0 ${ANCHO} ${ALTO}`} role="img" aria-label="Mapa de estaciones de monitoreo coloreadas por nivel de riesgo">
        <rect x="0" y="0" width={ANCHO} height={ALTO} fill="var(--surface)" />
        {ordenadas.map((e) => {
          const { x, y } = proyectar(e.lat, e.lon)
          const nivel = nivelRiesgo(e.indice)
          const activa = e.id === seleccionada
          return (
            <g
              key={e.id}
              className="estacion"
              onClick={() => onSeleccionar(e.id)}
              onMouseEnter={() => setHover(e)}
              onMouseLeave={() => setHover(null)}
              onKeyDown={(ev) => (ev.key === 'Enter' || ev.key === ' ') && onSeleccionar(e.id)}
              tabIndex={0}
              role="button"
              aria-pressed={activa}
              aria-label={`${e.nombre}: riesgo ${NIVELES[nivel].etiqueta}`}
            >
              <circle cx={x} cy={y} r={radio + 5} className="area-toque" />
              <circle cx={x} cy={y} r={(activa ? radio * 1.8 : radio) + 1.5} fill="var(--surface)" />
              <circle cx={x} cy={y} r={activa ? radio * 1.8 : radio} fill={NIVELES[nivel].color} />
            </g>
          )
        })}
      </svg>

      <div className="detalle-estacion" aria-live="polite">
        {enfocada ? (
          <>
            <strong>{enfocada.nombre}</strong>
            <span>
              {enfocada.detalle && `${enfocada.detalle} · `}
              {enfocada.id} · {NIVELES[nivelRiesgo(enfocada.indice)].icono} {NIVELES[nivelRiesgo(enfocada.indice)].etiqueta}
              {` (índice ${enfocada.indice.toFixed(2)}${enfocada.contaminante ? `, ${enfocada.contaminante}` : ''})`}
            </span>
          </>
        ) : (
          <span>Pasa el cursor o haz clic sobre una estación.</span>
        )}
      </div>

      <ul className="leyenda-estado">
        {Object.entries(NIVELES).map(([clave, n]) => (
          <li key={clave}>
            <span className="pastilla" style={{ background: n.color }} />
            {n.etiqueta} <span className="leyenda-detalle">({conteo[clave]}, {n.detalle})</span>
          </li>
        ))}
      </ul>
      {fuera > 0 && <p className="nota">{fuera} estaciones fuera del recuadro continental (Alaska, Hawái, Puerto Rico) no se dibujan.</p>}
    </figure>
  )
}
