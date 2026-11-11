// Panel "Las tecnologías en C": resultados reales de los experimentos 5 a 8
// (Pthreads, OpenMP, MPI y CUDA frente a su versión en Go o a la línea base
// secuencial). Lee public/datos/experimento5..8.json, que escriben
// scripts/nativo.sh y experimentos/cmd/{analisis,gpu}; la página los vuelve a
// pedir sola, así que una corrida que termina aparece sin recargar.
// No hay datos de ejemplo: sin archivo, la pestaña dice cómo generarlo.
import { useState } from 'react'
import { useResultadosNativos } from '../data/resultadosNativos'
import { esGPU, useCluster } from '../data/cluster'
import './PanelTecnologias.css'

const TECNOLOGIAS = [
  { clave: 'experimento5', titulo: 'Pthreads', unidad: 'hilos', comando: 'sh scripts/nativo.sh exp5', tema: 'Sem. 5-6 · Cargador: cola productor-consumidor, mutex' },
  { clave: 'experimento6', titulo: 'OpenMP', unidad: 'hilos', comando: 'sh scripts/nativo.sh exp6', tema: 'Sem. 7, 9-10 · Reducción y schedule' },
  { clave: 'experimento7', titulo: 'MPI', unidad: 'procesos', comando: 'sh scripts/nativo.sh exp7', tema: 'Sem. 3-4 · Scatter, reduce y paso de mensajes' },
  { clave: 'experimento8', titulo: 'CUDA', unidad: null, comando: 'sh scripts/nativo.sh exp8', tema: 'Sem. 11-13 · Kernels en la GPU' },
]

// Etiqueta y color fijos por modo (el color sigue al modo, no a su posición).
const MODOS = {
  concurrente: { etiqueta: 'Go, 1 agregadora', color: 'var(--serie-3)' },
  concurrente_particionado: { etiqueta: 'Go (goroutines)', color: 'var(--serie-3)' },
  pthreads: { etiqueta: 'Pthreads, 1 agregadora', color: 'var(--serie-1)' },
  pthreads_particionado: { etiqueta: 'Pthreads (C)', color: 'var(--serie-1)' },
  openmp_static: { etiqueta: 'OpenMP static', color: 'var(--serie-1)' },
  openmp_dynamic: { etiqueta: 'OpenMP dynamic', color: 'var(--serie-2)' },
  openmp_guided: { etiqueta: 'OpenMP guided', color: 'var(--serie-4)' },
  mpi_colectivo: { etiqueta: 'MPI Allreduce', color: 'var(--serie-1)' },
  mpi_p2p: { etiqueta: 'MPI Send/Recv + Bcast', color: 'var(--serie-2)' },
  secuencial_c: { etiqueta: 'Secuencial (C)', color: 'var(--text-secondary)' },
  cuda_atomic: { etiqueta: 'CUDA atomic', color: 'var(--serie-2)' },
  cuda_shared: { etiqueta: 'CUDA shared', color: 'var(--serie-3)' },
  cuda_shuffle: { etiqueta: 'CUDA shuffle', color: 'var(--serie-4)' },
}
const ORDEN_GPU = ['secuencial_c', 'openmp_static', 'cuda_atomic', 'cuda_shared', 'cuda_shuffle']

const fmt = (v, d = 2) => (Number.isFinite(v) ? v.toLocaleString('es-PE', { minimumFractionDigits: d, maximumFractionDigits: d }) : '—')
const nombreModo = (m) => MODOS[m]?.etiqueta ?? m
const colorModo = (m) => MODOS[m]?.color ?? 'var(--serie-5)'

export default function PanelTecnologias() {
  const { cargando, datos } = useResultadosNativos()
  const [clave, setClave] = useState('experimento6')
  const actual = TECNOLOGIAS.find((t) => t.clave === clave)
  const exp = datos[clave]
  const { inst } = useCluster()

  return (
    <figure className="panel panel-tecnologias">
      <figcaption>
        <h3>Las tecnologías en C</h3>
        <p>
          Resultados reales de las versiones en C frente a Go o a la línea base secuencial. Solo se muestran mediciones hechas en este equipo; la
          página se actualiza sola cuando termina una corrida.
        </p>
      </figcaption>

      <div className="pestanas" role="tablist">
        {TECNOLOGIAS.map((t) => (
          <button key={t.clave} role="tab" aria-selected={clave === t.clave} className="pestana" onClick={() => setClave(t.clave)}>
            {t.titulo}
            {datos[t.clave] && <span className="punto-datos" title="hay mediciones" />}
          </button>
        ))}
        <span className="tema-silabo pestana-tema">{actual.tema}</span>
      </div>

      {cargando ? (
        <p className="tec-vacio">Buscando resultados…</p>
      ) : actual.clave === 'experimento8' ? (
        <>
          <CudaEnCluster inst={inst} />
          {exp ? <ResultadosGPU exp={exp} /> : <SinDatos tecnologia={actual} detalle="la comparación de los tres kernels (atomic, shared y shuffle)" />}
        </>
      ) : !exp ? (
        <SinDatos tecnologia={actual} />
      ) : (
        <ResultadosEscalabilidad exp={exp} unidad={actual.unidad} />
      )}
    </figure>
  )
}

// CUDA ya trabaja dentro del clúster: los nodos con GPU calculan con el motor
// CUDA. Esto lista, con lo que el maestro midió de verdad en esta sesión, las
// corridas con GPU frente a las que fueron solo de CPU.
function CudaEnCluster({ inst }) {
  const historial = (inst?.historial ?? []).filter((h) => h.estado === 'terminado' && h.t_epoca_ms > 0)
  const gpus = (inst?.sala ?? []).filter((n) => esGPU(n.motor))
  const conGPU = (h) => (h.motores ?? []).some(esGPU)
  return (
    <div className="tec-cuda-cluster">
      <h4>CUDA dentro del clúster (corridas reales de esta sesión)</h4>
      {gpus.length > 0 && (
        <p className="tec-resumen">
          GPU conectadas: {gpus.map((n) => `${n.id} (${n.hardware?.gpu ?? n.dispositivo})`).join(' · ')}.
        </p>
      )}
      {historial.length === 0 ? (
        <p className="tec-vacio">Todavía no hay corridas terminadas. Lanza una desde «Clúster en vivo» con nodos con GPU y aparecerá aquí.</p>
      ) : (
        <div className="tec-tabla-caja">
          <table className="tec-tabla">
            <thead>
              <tr>
                <th>#</th>
                <th>nodos</th>
                <th>motor</th>
                <th>ms/época</th>
                <th>cálculo</th>
                <th>red</th>
                <th>tiempo</th>
              </tr>
            </thead>
            <tbody>
              {historial.map((h) => (
                <tr key={h.id}>
                  <td>{h.id}</td>
                  <td>{(h.nodos ?? []).join(' + ')}</td>
                  <td>{conGPU(h) ? 'GPU (CUDA)' : 'CPU (Go)'}</td>
                  <td>{fmt(h.t_epoca_ms, 1)}</td>
                  <td>{fmt(h.t_calculo_ms, 1)} ms</td>
                  <td>{fmt(h.t_comunicacion_ms, 1)} ms</td>
                  <td>{fmt(h.t_total_s, 2)} s</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function SinDatos({ tecnologia, detalle }) {
  return (
    <div className="tec-vacio">
      <p>
        Todavía no hay mediciones de <b>{tecnologia.titulo}</b> en este equipo{detalle ? `: ${detalle}` : ''}.
      </p>
      <p>
        Corre <code>{tecnologia.comando}</code>: al terminar aparecerán aquí los datos de esa ejecución.
      </p>
    </div>
  )
}

const ANCHO = 520
const ALTO = 240
const PAD = { top: 14, right: 16, bottom: 28, left: 40 }

function ResultadosEscalabilidad({ exp, unidad }) {
  const series = Object.entries(exp.series ?? {}).filter(([modo, ps]) => modo !== exp.modo_base && ps?.length)
  if (!series.length) return <p className="tec-vacio">El archivo no trae series paralelas todavía.</p>
  const unidades = [...new Set(series.flatMap(([, ps]) => ps.map((p) => p.unidades)))].sort((a, b) => a - b)
  const maxY = Math.ceil(Math.max(...unidades, ...series.flatMap(([, ps]) => ps.map((p) => p.speedup))))
  const x = (u) => PAD.left + (unidades.length === 1 ? 0 : (unidades.indexOf(u) / (unidades.length - 1)) * (ANCHO - PAD.left - PAD.right))
  const y = (v) => ALTO - PAD.bottom - (v / maxY) * (ALTO - PAD.top - PAD.bottom)
  const ruta = (pts) => pts.map((p, i) => `${i ? 'L' : 'M'} ${x(p.unidades)} ${y(p.speedup)}`).join(' ')
  const maquina = series.flatMap(([, ps]) => ps).find((p) => p.maquina)?.maquina
  const repeticiones = Math.max(...series.flatMap(([, ps]) => ps.map((p) => p.repeticiones)))

  return (
    <>
      <p className="tec-resumen">
        {exp.experimento}: {exp.descripcion}. T(1) = {fmt(exp.t1_s, exp.t1_s < 10 ? 2 : 1)} s ({nombreModo(exp.modo_base)}), mediana de {repeticiones}{' '}
        {repeticiones === 1 ? 'corrida' : 'corridas'}, {exp.cpus_logicas} CPUs lógicas.
        {maquina && <> Equipo: {maquina}.</>} Medido el {new Date(exp.generado).toLocaleString('es-PE')}.
      </p>

      <ul className="leyenda-series">
        {series.map(([modo]) => (
          <li key={modo}>
            <span className="linea-key" style={{ background: colorModo(modo) }} />
            {nombreModo(modo)}
          </li>
        ))}
        <li>
          <span className="linea-key linea-key-punteada" />
          Ideal S(p)=p
        </li>
      </ul>

      <svg className="tec-grafica" viewBox={`0 0 ${ANCHO} ${ALTO}`} role="img" aria-label={`Speedup según el número de ${unidad}, frente al ideal`}>
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line x1={PAD.left} x2={ANCHO - PAD.right} y1={y(f * maxY)} y2={y(f * maxY)} className="tec-grid" />
            <text x={PAD.left - 6} y={y(f * maxY)} className="tec-tick-y">{`${fmt(f * maxY, 1)}×`}</text>
          </g>
        ))}
        <path d={ruta(unidades.map((u) => ({ unidades: u, speedup: u })).filter((p) => p.speedup <= maxY))} className="tec-ideal" />
        {series.map(([modo, ps]) => (
          <g key={modo}>
            <path d={ruta(ps)} className="tec-linea" style={{ stroke: colorModo(modo) }} />
            {ps.map((p) => (
              <g key={p.unidades}>
                <title>{`${nombreModo(modo)}, ${p.unidades} ${unidad}: ${fmt(p.speedup)}× (${fmt(p.t_total_s, 2)} s)`}</title>
                <circle cx={x(p.unidades)} cy={y(p.speedup)} r="5" style={{ fill: colorModo(modo) }} />
              </g>
            ))}
          </g>
        ))}
        {unidades.map((u) => (
          <text key={u} x={x(u)} y={ALTO - 8} className="tec-tick-x">{u}</text>
        ))}
        <text x={ANCHO - PAD.right} y={ALTO - 8} className="tec-unidad">{unidad}</text>
      </svg>

      <div className="tec-tabla-caja">
        <table className="tec-tabla">
          <thead>
            <tr>
              <th>{unidad}</th>
              {series.map(([modo]) => (
                <th key={modo} colSpan={2}>{nombreModo(modo)}</th>
              ))}
            </tr>
            <tr>
              <th />
              {series.map(([modo]) => (
                <CabeceraSerie key={modo} />
              ))}
            </tr>
          </thead>
          <tbody>
            {unidades.map((u) => (
              <tr key={u}>
                <td>{u}</td>
                {series.map(([modo, ps]) => {
                  const p = ps.find((q) => q.unidades === u)
                  return (
                    <Celdas key={modo} p={p} />
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
}

// Dos columnas por serie: tiempo y speedup.
function CabeceraSerie() {
  return (
    <>
      <th className="tec-sub">T [s]</th>
      <th className="tec-sub">S(p)</th>
    </>
  )
}

function Celdas({ p }) {
  return (
    <>
      <td>{p ? fmt(p.t_total_s, 2) : '—'}</td>
      <td>{p ? `${fmt(p.speedup)}×` : '—'}</td>
    </>
  )
}

function ResultadosGPU({ exp }) {
  return (
    <>
      <p className="tec-resumen">
        {exp.experimento}: {exp.descripcion}. Cada barra es el tiempo por época (mediana de las repeticiones, escala logarítmica); cuanto más corta, mejor.
        Medido el {new Date(exp.generado).toLocaleString('es-PE')}.
      </p>
      {exp.equipos.map((eq) => (
        <EquipoGPU key={eq.nombre} equipo={eq} base={exp.modo_base} />
      ))}
    </>
  )
}

function EquipoGPU({ equipo, base }) {
  const tiempos = equipo.tamanos.flatMap((t) => Object.values(t.modos).map((m) => m.t_epoca_ms)).filter((v) => v > 0)
  const lmin = Math.log10(Math.min(...tiempos))
  const lmax = Math.log10(Math.max(...tiempos))
  const ancho = (v) => (lmax === lmin ? 60 : 6 + (94 * (Math.log10(v) - lmin)) / (lmax - lmin))
  const modosPresentes = ORDEN_GPU.filter((m) => equipo.tamanos.some((t) => t.modos[m]))
  return (
    <div className="tec-equipo">
      <h4>Equipo: {equipo.nombre}</h4>
      <ul className="leyenda-series">
        {modosPresentes.map((m) => (
          <li key={m}>
            <span className="linea-key" style={{ background: colorModo(m) }} />
            {nombreModo(m)}
          </li>
        ))}
      </ul>
      {equipo.tamanos.map((t) => (
        <div key={t.ejemplos} className="tec-tamano">
          <p className="tec-tamano-titulo">{t.ejemplos.toLocaleString('es-PE')} ejemplos</p>
          {modosPresentes.filter((m) => t.modos[m]).map((m) => {
            const med = t.modos[m]
            const ref = t.modos[base]
            return (
              <div key={m} className="tec-barra-fila">
                <span className="tec-barra-nombre">{nombreModo(m)}</span>
                <span className="tec-barra-pista">
                  <span className="tec-barra" style={{ width: `${ancho(med.t_epoca_ms)}%`, background: colorModo(m) }} />
                </span>
                <span className="tec-barra-valor">
                  {fmt(med.t_epoca_ms, med.t_epoca_ms < 10 ? 3 : 1)} ms
                  {ref && m !== base && <small> · {fmt(ref.t_epoca_ms / med.t_epoca_ms, 1)}× el secuencial</small>}
                </span>
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}
