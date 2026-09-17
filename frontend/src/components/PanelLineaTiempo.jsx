import { lineaTiempoMock } from '../data/mock'
import './PanelLineaTiempo.css'

const ANCHO = 480
const ALTO = 220
const PAD = { top: 16, right: 44, bottom: 24, left: 32 }

function construirRuta(puntos, accesor, escalaX, escalaY) {
  return puntos.map((p, i) => `${i === 0 ? 'M' : 'L'} ${escalaX(p.hora)} ${escalaY(accesor(p))}`).join(' ')
}

export default function PanelLineaTiempo({ estacion }) {
  const datos = lineaTiempoMock(estacion.id)
  const valores = datos.flatMap((d) => [d.observado, d.predicho])
  const yMin = Math.min(...valores) - 2
  const yMax = Math.max(...valores) + 2

  const escalaX = (hora) => PAD.left + (hora / 23) * (ANCHO - PAD.left - PAD.right)
  const escalaY = (v) => ALTO - PAD.bottom - ((v - yMin) / (yMax - yMin)) * (ALTO - PAD.top - PAD.bottom)

  const rutaObservado = construirRuta(datos, (d) => d.observado, escalaX, escalaY)
  const rutaPredicho = construirRuta(datos, (d) => d.predicho, escalaX, escalaY)

  const ultimo = datos[datos.length - 1]

  return (
    <figure className="panel panel-linea-tiempo">
      <figcaption>
        <h3>Concentración observada vs. predicha</h3>
        <p>{estacion.nombre} · {estacion.contaminante} · últimas 24 horas</p>
      </figcaption>

      <svg viewBox={`0 0 ${ANCHO} ${ALTO}`} role="img" aria-label={`Concentración observada y predicha de ${estacion.contaminante} en ${estacion.nombre}`}>
        {[0, 0.25, 0.5, 0.75, 1].map((f) => {
          const y = PAD.top + f * (ALTO - PAD.top - PAD.bottom)
          return <line key={f} x1={PAD.left} x2={ANCHO - PAD.right} y1={y} y2={y} className="gridline" />
        })}

        <path d={rutaObservado} className="linea linea-observado" />
        <path d={rutaPredicho} className="linea linea-predicho" />

        {datos.map((d) => (
          <g key={`o-${d.hora}`}>
            <title>{`Hora ${d.hora}: ${d.observado} observado`}</title>
            <circle cx={escalaX(d.hora)} cy={escalaY(d.observado)} r="3" className="punto punto-observado" />
          </g>
        ))}
        {datos.map((d) => (
          <g key={`p-${d.hora}`}>
            <title>{`Hora ${d.hora}: ${d.predicho} predicho`}</title>
            <circle cx={escalaX(d.hora)} cy={escalaY(d.predicho)} r="3" className="punto punto-predicho" />
          </g>
        ))}

        <text x={escalaX(ultimo.hora) + 4} y={escalaY(ultimo.observado)} className="etiqueta-final etiqueta-observado">
          {ultimo.observado}
        </text>
        <text x={escalaX(ultimo.hora) + 4} y={escalaY(ultimo.predicho)} className="etiqueta-final etiqueta-predicho">
          {ultimo.predicho}
        </text>
      </svg>

      <ul className="leyenda-series">
        <li><span className="linea-key" style={{ background: 'var(--serie-1)' }} />Observado</li>
        <li><span className="linea-key" style={{ background: 'var(--serie-2)' }} />Predicho</li>
      </ul>
    </figure>
  )
}
