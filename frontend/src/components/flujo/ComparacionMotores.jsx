// Entrenamiento: por nodo y motor (OpenMP, Go), el % de CPU en cada época y
// el tiempo de cálculo; los motores corren uno tras otro sobre el mismo
// entrenamiento, como Pthreads y Go en la limpieza.
import { NOMBRE_MOTOR } from './motores'

const fmt = (v, d = 0) => (v == null || Number.isNaN(v) ? '—' : v.toLocaleString('es-PE', { maximumFractionDigits: d, minimumFractionDigits: d }))
const media = (s) => (s?.length ? s.reduce((a, b) => a + b, 0) / s.length : null)
const suma = (s) => (s ?? []).reduce((a, b) => a + b, 0)

function GraficaCPU({ serie, total, estado }) {
  const puntos = serie ?? []
  const x = (i) => (total > 1 ? (i / (total - 1)) * 100 : 50)
  const camino = puntos.map((y, i) => `${x(i).toFixed(2)},${(40 - (Math.min(y, 100) / 100) * 38).toFixed(2)}`).join(' ')
  return (
    <svg className="cpu-epocas" viewBox="0 0 100 40" preserveAspectRatio="none" role="img" aria-label={`Uso del CPU por época: media ${fmt(media(puntos))} %`}>
      <line x1="0" y1="2" x2="100" y2="2" className="guia" />
      <line x1="0" y1="21" x2="100" y2="21" className="guia" />
      <line x1="0" y1="40" x2="100" y2="40" className="base" />
      {puntos.length > 1 && <polyline points={camino} className={`linea ${estado}`} vectorEffect="non-scaling-stroke" />}
    </svg>
  )
}

function Celda({ corrida, id }) {
  const n = corrida?.nodos?.find((x) => x.id === id)
  if (!n) return <><td className="detalle">en espera</td><td /></>
  const gpu = media(n.gpu)
  return (
    <>
      <td className="celda-grafica">
        <div className="grafica-cpu">
          <GraficaCPU serie={n.cpu} total={corrida.epocas} estado={corrida.estado === 'terminado' ? 'terminada' : 'limpiando'} />
          <span className="grafica-pie">CPU media {fmt(media(n.cpu))} %{gpu > 0 ? ` · GPU ${fmt(gpu * 100)} % de sus filas` : ''}</span>
        </div>
      </td>
      <td className="celda-tiempo">
        {n.calculo_ms?.length ? `${fmt(suma(n.calculo_ms) / 1000, 1)} s` : '—'}
        {n.calculo_ms?.length > 0 && <small className="cpu-media">{fmt(media(n.calculo_ms))} ms por época</small>}
      </td>
    </>
  )
}

export default function ComparacionMotores({ corrida, tarjeta }) {
  if (!corrida.motor_cpu) return null
  const corridas = [...(corrida.anteriores ?? []), corrida]
  const motores = corrida.motores?.length ? corrida.motores : [corrida.motor_cpu]
  const de = (m) => corridas.find((c) => c.motor_cpu === m)
  return (
    <section className="flujo-limpieza" aria-label="Cálculo por motor">
      <div className="tabla-limpieza-envoltura">
        <table className="tabla-limpieza tabla-motores">
          <thead>
            <tr>
              <th scope="col" rowSpan={2} className="cabeza-doble">Nodo</th>
              {motores.map((m) => {
                const usaGPU = de(m)?.nodos?.some((n) => n.motor?.includes('cuda'))
                return <th key={m} scope="colgroup" colSpan={2} className="cabeza-tec">{NOMBRE_MOTOR[m] ?? m}{usaGPU ? ' + CUDA' : ''} <small>% Uso del CPU por época</small></th>
              })}
            </tr>
            <tr>{motores.map((m) => [<th key={`${m}c`} scope="col">Épocas</th>, <th key={`${m}t`} scope="col">Cálculo</th>])}</tr>
          </thead>
          <tbody>
            {(corrida.nodos ?? []).map((n) => (
              <tr key={n.id}>
                <th scope="row">{tarjeta(n)}</th>
                {motores.map((m) => <Celda key={m} corrida={de(m)} id={n.id} />)}
              </tr>
            ))}
            <tr className="fila-total">
              <th scope="row">Clúster</th>
              {motores.map((m) => {
                const c = de(m)
                const r = c?.resultado
                return [
                  <td key={`${m}d`} className="detalle">
                    {c ? `${fmt(media(c.t_epoca_ms))} ms por época · pérdida ${fmt(c.perdidas?.at(-1), 4)}${r?.speedup > 0 ? ` · speedup ${fmt(r.speedup, 2)}×` : ''}` : 'en espera'}
                  </td>,
                  <td key={`${m}s`} className="celda-tiempo">{c?.t_epoca_ms?.length ? `${fmt(suma(c.t_epoca_ms) / 1000, 1)} s` : '—'}</td>,
                ]
              })}
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  )
}
