// Historial de un tipo de proceso (limpieza, entrenamiento o predicción):
// lo guarda el maestro en resultados/historial/. Tocar una fila muestra ese
// proceso con sus datos de entonces.
import { useEffect, useState } from 'react'
import { apiCluster } from '../../data/cluster'

const fmtFecha = (f) => new Date(f).toLocaleString('es-PE', { dateStyle: 'short', timeStyle: 'medium' })
const fmtDuracion = (s) => {
  if (!(s > 0)) return '—'
  if (s < 60) return `${s.toLocaleString('es-PE', { maximumFractionDigits: 1 })} s`
  return `${Math.floor(s / 60)} min ${Math.round(s % 60)} s`
}
const ESTADOS = { terminada: 'terminada', terminado: 'terminado', cancelada: 'parada', cancelado: 'cancelado', error: 'error' }

// recargar cambia cuando termina un proceso: se vuelve a pedir la lista.
export default function Historial({ tipo, recargar, elegido, onElegir }) {
  const [estado, setEstado] = useState({ tipo: '', lista: [], error: '' })
  useEffect(() => {
    let vigente = true
    apiCluster.historial(tipo)
      .then((lista) => vigente && setEstado({ tipo, lista, error: '' }))
      .catch((e) => vigente && setEstado({ tipo, lista: [], error: e.message }))
    return () => {
      vigente = false
    }
  }, [tipo, recargar])
  const lista = estado.tipo === tipo ? estado.lista : []

  return (
    <section className="historial" aria-label="Historial">
      <span className="rotulo">HISTORIAL</span>
      {estado.error && <p className="detalle">No se pudo leer el historial: {estado.error}</p>}
      {!estado.error && lista.length === 0 && <p className="detalle">Todavía no hay procesos de este tipo guardados.</p>}
      {lista.length > 0 && (
        <div className="historial-envoltura">
          <table className="historial-tabla">
            <thead>
              <tr>
                <th scope="col">Fecha y hora</th>
                <th scope="col">Maestro</th>
                <th scope="col">Proceso</th>
                <th scope="col">Estado</th>
                <th scope="col">Duración</th>
              </tr>
            </thead>
            <tbody>
              {lista.map((e) => (
                <tr key={e.id} className={elegido === e.id ? 'elegido' : ''}>
                  <td>
                    <button type="button" className="historial-ver" onClick={() => onElegir(e.id)} aria-pressed={elegido === e.id}>
                      {fmtFecha(e.fecha)}
                    </button>
                  </td>
                  <td>{e.maestro}</td>
                  <td>{e.descripcion}</td>
                  <td className={`historial-estado ${e.estado}`}>{ESTADOS[e.estado] ?? e.estado}</td>
                  <td>{fmtDuracion(e.duracion_s)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
