import { useState } from 'react'
import './PanelLineaTiempo.css'

const ANCHO = 480
const ALTO = 170
const ALTO_PROB = 90
const PAD = { top: 12, right: 16, bottom: 22, left: 40 }
const HORAS = Array.from({ length: 24 }, (_, h) => h)

const escalaX = (hora) => PAD.left + (hora / 23) * (ANCHO - PAD.left - PAD.right)

// Ruta que se corta donde falta el dato (la estación no midió esa hora).
function ruta(valores, escalaY) {
  let d = ''
  let abierta = false
  valores.forEach((v, h) => {
    if (v == null) {
      abierta = false
      return
    }
    d += `${abierta ? 'L' : 'M'} ${escalaX(h)} ${escalaY(v)} `
    abierta = true
  })
  return d
}

function formatear(v, unidad) {
  if (v == null) return 'sin dato'
  return `${v < 1 ? v.toFixed(3) : v.toFixed(1)} ${unidad}`
}

export default function PanelLineaTiempo({ estacion, serie }) {
  const [hora, setHora] = useState(null)
  const observados = serie.observado.filter((v) => v != null)
  const yMax = Math.max(serie.umbral * 1.15, ...observados) || 1
  const escalaY = (v) => ALTO - PAD.bottom - (v / yMax) * (ALTO - PAD.top - PAD.bottom)
  const escalaP = (p) => ALTO_PROB - PAD.bottom - p * (ALTO_PROB - PAD.top - PAD.bottom)
  const conProbabilidad = Array.isArray(serie.probabilidad)

  const zonaHover = (alto) =>
    HORAS.map((h) => (
      <rect
        key={h}
        x={escalaX(h) - (ANCHO - PAD.left - PAD.right) / 46}
        width={(ANCHO - PAD.left - PAD.right) / 23}
        y={0}
        height={alto}
        fill="transparent"
        onMouseEnter={() => setHora(h)}
      />
    ))

  const cruz = (alto) =>
    hora != null && <line x1={escalaX(hora)} x2={escalaX(hora)} y1={PAD.top} y2={alto - PAD.bottom} className="cruz" />

  return (
    <figure className="panel panel-linea-tiempo" onMouseLeave={() => setHora(null)}>
      <figcaption>
        <h3>{serie.contaminante} observado {conProbabilidad && 'y alerta del modelo'}</h3>
        <p>
          {estacion.nombre} · {serie.real ? `${serie.fecha} (UTC)` : 'datos de ejemplo'}
          {serie.real && serie.fecha && ' · día con más horas de NO2 sobre el umbral en la red'}
        </p>
      </figcaption>

      <p className="lectura" aria-live="polite">
        {hora == null
          ? 'Pasa el cursor sobre la gráfica para ver cada hora.'
          : `${String(hora).padStart(2, '0')}:00 UTC · ${serie.contaminante} ${formatear(serie.observado[hora], serie.unidad)}` +
            (conProbabilidad
              ? ` · probabilidad de superar ${serie.umbral} ${serie.unidad} a la hora siguiente: ${
                  serie.probabilidad[hora] == null ? 'sin dato' : `${Math.round(serie.probabilidad[hora] * 100)}%`
                }`
              : '')}
      </p>

      <svg viewBox={`0 0 ${ANCHO} ${ALTO}`} role="img" aria-label={`${serie.contaminante} observado por hora en ${estacion.nombre}`}>
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line x1={PAD.left} x2={ANCHO - PAD.right} y1={escalaY(f * yMax)} y2={escalaY(f * yMax)} className="gridline" />
            <text x={PAD.left - 6} y={escalaY(f * yMax)} className="tick-y">
              {(f * yMax).toFixed(yMax < 1 ? 3 : 0)}
            </text>
          </g>
        ))}
        <line x1={PAD.left} x2={ANCHO - PAD.right} y1={escalaY(serie.umbral)} y2={escalaY(serie.umbral)} className="umbral" />
        <text x={ANCHO - PAD.right} y={escalaY(serie.umbral) - 4} className="etiqueta-umbral">
          umbral AQI 51 ({serie.umbral} {serie.unidad})
        </text>
        <path d={ruta(serie.observado, escalaY)} className="linea linea-observado" />
        {cruz(ALTO)}
        {hora != null && serie.observado[hora] != null && (
          <circle cx={escalaX(hora)} cy={escalaY(serie.observado[hora])} r="4" className="punto punto-observado" />
        )}
        {[0, 6, 12, 18, 23].map((h) => (
          <text key={h} x={escalaX(h)} y={ALTO - 6} className="tick-x">{`${h}h`}</text>
        ))}
        {zonaHover(ALTO)}
      </svg>

      {conProbabilidad && (
        <>
          <h4 className="subtitulo">Probabilidad de cruzar el umbral en la hora siguiente (modelo entrenado)</h4>
          <svg viewBox={`0 0 ${ANCHO} ${ALTO_PROB}`} role="img" aria-label="Probabilidad predicha por hora">
            {[0, 0.5, 1].map((f) => (
              <g key={f}>
                <line x1={PAD.left} x2={ANCHO - PAD.right} y1={escalaP(f)} y2={escalaP(f)} className="gridline" />
                <text x={PAD.left - 6} y={escalaP(f)} className="tick-y">{`${f * 100}%`}</text>
              </g>
            ))}
            <path d={ruta(serie.probabilidad, escalaP)} className="linea linea-predicho" />
            {cruz(ALTO_PROB)}
            {hora != null && serie.probabilidad[hora] != null && (
              <circle cx={escalaX(hora)} cy={escalaP(serie.probabilidad[hora])} r="4" className="punto punto-predicho" />
            )}
            {zonaHover(ALTO_PROB)}
          </svg>
        </>
      )}
      {!conProbabilidad && serie.real && (
        <p className="nota">Esta estación no mide NO2; el modelo actual solo predice NO2.</p>
      )}
    </figure>
  )
}
