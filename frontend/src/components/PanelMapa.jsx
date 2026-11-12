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

// Índice = fracción de horas del año sobre el umbral de AQI 51 del gas
// dominante (el de mayor fracción).
function nivelRiesgo(indice) {
  if (indice >= 0.1) return 'critical'
  if (indice >= 0.02) return 'warning'
  return 'good'
}

// Alerta del modelo: la probabilidad más alta del día de que el NO2 supere
// el umbral la hora siguiente (predicción distribuida del clúster).
function nivelAlerta(p) {
  if (p >= 0.5) return 'critical'
  if (p >= 0.2) return 'warning'
  return 'good'
}

const NIVELES_ALERTA = {
  good: { etiqueta: 'Sin alerta', icono: '●', color: 'var(--status-good)', detalle: 'prob. < 20 %' },
  warning: { etiqueta: 'Atención', icono: '▲', color: 'var(--status-warning)', detalle: '20-50 %' },
  critical: { etiqueta: 'Alerta', icono: '◆', color: 'var(--status-critical)', detalle: '≥ 50 %' },
}

const NIVELES = {
  good: { etiqueta: 'Bajo', icono: '●', color: 'var(--status-good)', detalle: '< 2% de horas' },
  warning: { etiqueta: 'Moderado', icono: '▲', color: 'var(--status-warning)', detalle: '2-10% de horas' },
  critical: { etiqueta: 'Alto', icono: '◆', color: 'var(--status-critical)', detalle: '≥ 10% de horas' },
}

export default function PanelMapa({ estaciones, seleccionada, onSeleccionar, dia }) {
  const [hover, setHover] = useState(null)
  const hayAlerta = estaciones.lista.some((e) => Number.isFinite(e.alerta))
  const [modoElegido, setModo] = useState('historico')
  const modo = hayAlerta ? modoElegido : 'historico'
  const enAlerta = modo === 'alerta'
  // En modo alerta solo se dibujan las estaciones con predicción (NO2).
  const lista = enAlerta ? estaciones.lista.filter((e) => Number.isFinite(e.alerta)) : estaciones.lista
  const valor = (e) => (enAlerta ? e.alerta : e.indice)
  const nivelDe = (e) => (enAlerta ? nivelAlerta(e.alerta) : nivelRiesgo(e.indice))
  const niveles = enAlerta ? NIVELES_ALERTA : NIVELES
  const visibles = lista.filter(dentroDelMapa)
  const fuera = lista.length - visibles.length
  // Las de mayor riesgo se dibujan al final para quedar encima.
  const ordenadas = [...visibles].sort((a, b) => valor(a) - valor(b))
  const conteo = { good: 0, warning: 0, critical: 0 }
  lista.forEach((e) => conteo[nivelDe(e)]++)
  const radio = estaciones.real ? 2.6 : 6
  const enfocada = hover ?? estaciones.lista.find((e) => e.id === seleccionada)

  return (
    <figure className="panel panel-mapa">
      <figcaption>
        <h3>Mapa de estaciones</h3>
        <p>
          {!estaciones.real
            ? 'Riesgo previsto (datos de ejemplo)'
            : enAlerta
              ? `${lista.length} estaciones con NO2 · alerta del modelo el ${dia ?? 'día de la serie'}: probabilidad más alta de superar 53 ppb la hora siguiente (predicción distribuida)`
              : `${estaciones.lista.length} estaciones · riesgo histórico 2025: % de horas sobre el umbral AQI 51 del gas dominante`}
        </p>
        {hayAlerta && (
          <div className="vistas mapa-modo" role="tablist" aria-label="Color del mapa">
            <button type="button" role="tab" aria-selected={!enAlerta} onClick={() => setModo('historico')}>Riesgo histórico</button>
            <button type="button" role="tab" aria-selected={enAlerta} onClick={() => setModo('alerta')}>Alerta del modelo</button>
          </div>
        )}
      </figcaption>

      <svg viewBox={`0 0 ${ANCHO} ${ALTO}`} role="img" aria-label="Mapa de estaciones de monitoreo coloreadas por nivel de riesgo">
        <rect x="0" y="0" width={ANCHO} height={ALTO} fill="var(--surface)" />
        {ordenadas.map((e) => {
          const { x, y } = proyectar(e.lat, e.lon)
          const nivel = nivelDe(e)
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
              aria-label={`${e.nombre}: ${niveles[nivel].etiqueta}`}
            >
              <circle cx={x} cy={y} r={radio + 5} className="area-toque" />
              <circle cx={x} cy={y} r={(activa ? radio * 1.8 : radio) + 1.5} fill="var(--surface)" />
              <circle cx={x} cy={y} r={activa ? radio * 1.8 : radio} fill={niveles[nivel].color} />
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
              {enfocada.id} · {niveles[nivelDe(enfocada)].icono} {niveles[nivelDe(enfocada)].etiqueta}
              {enAlerta
                ? ` (probabilidad máxima del día ${(enfocada.alerta * 100).toFixed(1)}%)`
                : ` (${(enfocada.indice * 100).toFixed(1)}% de horas sobre el umbral${enfocada.contaminante ? ` de ${enfocada.contaminante}` : ''})`}
            </span>
          </>
        ) : (
          <span>Pasa el cursor o haz clic sobre una estación.</span>
        )}
      </div>

      <ul className="leyenda-estado">
        {Object.entries(niveles).map(([clave, n]) => (
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
