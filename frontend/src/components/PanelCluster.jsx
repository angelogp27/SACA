import { nodosCluster, epocaActual, perdidaPorEpocaMock } from '../data/mock'
import './PanelCluster.css'

const ANCHO_MINI = 200
const ALTO_MINI = 60

function rutaSparkline(valores) {
  const min = Math.min(...valores)
  const max = Math.max(...valores)
  const paso = ANCHO_MINI / (valores.length - 1)
  return valores
    .map((v, i) => {
      const x = i * paso
      const y = ALTO_MINI - ((v - min) / (max - min || 1)) * (ALTO_MINI - 8) - 4
      return `${i === 0 ? 'M' : 'L'} ${x} ${y}`
    })
    .join(' ')
}

export default function PanelCluster() {
  return (
    <figure className="panel panel-cluster">
      <figcaption>
        <h3>Estado del clúster</h3>
        <p>Telemetría de entrenamiento en vivo</p>
      </figcaption>

      <div className="cluster-resumen">
        <div className="stat-tile">
          <span className="stat-label">Época actual</span>
          <span className="stat-valor">{epocaActual}</span>
        </div>
        <div className="stat-tile">
          <span className="stat-label">Nodos activos</span>
          <span className="stat-valor">{nodosCluster.filter((n) => n.estado === 'activo').length} / {nodosCluster.length}</span>
        </div>
      </div>

      <ul className="lista-nodos">
        {nodosCluster.map((n) => (
          <li key={n.id} className="nodo">
            <span className={`estado-dot estado-${n.estado}`} aria-hidden="true" />
            <span className="nodo-id">{n.id}</span>
            <span className="nodo-estado">{n.estado === 'activo' ? 'activo' : 'inactivo'}</span>
            <div className="barra-contenedor" aria-label={`CPU ${n.cpu}%`}>
              <div className="barra-fill" style={{ width: `${n.cpu}%` }} />
            </div>
            <span className="nodo-valor">{n.cpu}% CPU</span>
            <span className="nodo-valor">{n.ram} MB</span>
          </li>
        ))}
      </ul>

      <div className="perdida-epoca">
        <h4>Pérdida por época</h4>
        <svg viewBox={`0 0 ${ANCHO_MINI} ${ALTO_MINI}`} role="img" aria-label="Curva de pérdida por época del entrenamiento">
          <path d={rutaSparkline(perdidaPorEpocaMock)} className="sparkline" />
        </svg>
        <span className="perdida-valor">{perdidaPorEpocaMock[perdidaPorEpocaMock.length - 1].toFixed(3)}</span>
      </div>
    </figure>
  )
}
