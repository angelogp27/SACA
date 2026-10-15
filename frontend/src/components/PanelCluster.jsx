// Panel de control del clúster: muestra los nodos conectados al maestro en
// modo servidor, permite elegir cuáles usar y lanzar un entrenamiento, y
// dibuja en vivo cómo va (época, pérdida, cálculo frente a red, speedup,
// el nodo que marca el ritmo) junto con el concepto del curso que se está
// viendo. Sin servidor, muestra los resultados guardados.
import { useState } from 'react'
import { apiCluster, esGPU, promedioFinal, useCluster } from '../data/cluster'
import { BarrasH, GraficaApilada, GraficaLineas } from './Graficas'
import ResultadosCluster from './ResultadosCluster'
import './PanelCluster.css'

const COLORES = ['var(--serie-1)', 'var(--serie-4)', 'var(--serie-5)', 'var(--serie-3)', 'var(--serie-2)']
const CALCULO = 'var(--serie-3)'

// comoLista: una serie que puede faltar (corrida recién empezada) se trata
// como vacía.
const comoLista = (v) => (Array.isArray(v) ? v : [])
const RED = 'var(--serie-2)'

const fmt = (v, d = 1) =>
  Number.isFinite(v) ? v.toLocaleString('es-PE', { minimumFractionDigits: d, maximumFractionDigits: d }) : '—'

function fmtEjemplos(v) {
  if (!Number.isFinite(v) || v <= 0) return '—'
  if (v >= 1e9) return `${fmt(v / 1e9)} mil M`
  if (v >= 1e6) return `${fmt(v / 1e6)} M`
  return `${fmt(v / 1e3, 0)} mil`
}

// Media móvil para que el speedup en vivo no salte época a época.
function mediaMovil(valores, n = 10) {
  let suma = 0
  return valores.map((v, i) => {
    suma += v
    if (i >= n) suma -= valores[i - n]
    return suma / Math.min(i + 1, n)
  })
}

function Icono({ motor }) {
  return <span className={`icono-motor ${esGPU(motor) ? 'gpu' : 'cpu'}`}>{esGPU(motor) ? 'GPU' : 'CPU'}</span>
}

function Kpi({ etiqueta, valor, detalle, tema }) {
  return (
    <div className="kpi">
      <span className="kpi-etiqueta">{etiqueta}</span>
      <span className="kpi-valor">{valor}</span>
      {detalle && <span className="kpi-detalle">{detalle}</span>}
      {tema && <span className="tema-silabo">{tema}</span>}
    </div>
  )
}

// conceptos explica en una frase qué parte del curso está ocurriendo.
function conceptos(inst, corrida, lento) {
  if (!corrida || (corrida.estado !== 'entrenando' && corrida.estado !== 'verificando')) {
    const n = comoLista(inst?.sala).length
    return [
      { tema: 'Sem. 2 · MIMD de memoria distribuida', texto: `Sala de espera: ${n} nodo(s) conectados por la red. No comparten memoria; solo intercambian mensajes con el maestro.` },
    ]
  }
  if (corrida.estado === 'verificando') {
    return [{ tema: 'Correctitud', texto: 'Verificación: el maestro entrena lo mismo en secuencial (1 hilo) para comprobar que el modelo distribuido es idéntico.' }]
  }
  const lista = []
  if (corrida.epoca === 0) {
    lista.push({ tema: 'Sem. 4 · Scatter · Sem. 10 · Descomposición de datos', texto: `Partición enviada: cada uno de los ${corrida.nodos.length} nodos se queda con un grupo distinto de estaciones (hash de estacion_id).` })
  } else {
    lista.push({ tema: 'Sem. 3-4 · Broadcast + reduce', texto: `Época ${corrida.epoca}: el maestro difunde los 13 pesos, cada nodo calcula su gradiente parcial y el maestro los suma.` })
  }
  if (lento && corrida.nodos.length > 1) {
    lista.push({ tema: 'Sem. 6 · Barrera · Sem. 10 · Balanceo', texto: `Barrera síncrona: la época termina cuando llega el más lento (${lento.id}, ${fmt(promedioFinal(lento.calculo_ms))} ms de cálculo).` })
  }
  if (corrida.nodos.some((n) => esGPU(n.motor))) {
    lista.push({ tema: 'Sem. 11-12 · CUDA', texto: 'Nodos GPU: kernel CUDA con reducción en memoria compartida, validado contra la CPU antes de entrenar.' })
  }
  return lista
}

function Sala({ inst, elegidos, setElegidos, ocupado, onError }) {
  const sala = comoLista(inst?.sala)
  const elegido = (id) => elegidos[id] ?? true
  if (!sala.length) {
    return (
      <p className="sala-vacia">
        Ningún nodo conectado todavía. Cada integrante ejecuta el comando de su guía (docs/cluster) apuntando a este maestro;
        aparecerán aquí al conectarse.
      </p>
    )
  }
  return (
    <ul className="sala">
      {sala.map((n) => (
        <li key={n.id} className={`nodo-sala ${n.estado === 'en_corrida' ? 'en-corrida' : ''}`}>
          <label>
            <input
              type="checkbox"
              checked={elegido(n.id)}
              disabled={ocupado}
              onChange={(e) => setElegidos({ ...elegidos, [n.id]: e.target.checked })}
            />
            <Icono motor={n.motor} />
            <span className="nodo-nombre">{n.id}</span>
          </label>
          <span className="nodo-dispositivo" title={n.dispositivo}>{n.dispositivo}</span>
          <span className="nodo-ip">{n.remoto?.split(':')[0]}</span>
          <span className={`nodo-estado ${n.estado}`}>{n.estado === 'en_corrida' ? 'en corrida' : 'libre'}</span>
          <button
            className="boton-mini"
            disabled={n.estado === 'en_corrida'}
            onClick={() => apiCluster.quitar(n.id).catch((e) => onError(e.message))}
            title="Despedir a este nodo (se cierra en su PC)"
          >
            Quitar
          </button>
        </li>
      ))}
    </ul>
  )
}

function Controles({ inst, elegidos, onError }) {
  const [epocas, setEpocas] = useState(200)
  const [verificar, setVerificar] = useState(false)
  const sala = comoLista(inst?.sala)
  const ids = sala.filter((n) => (elegidos[n.id] ?? true) && n.estado === 'libre').map((n) => n.id)
  const ocupado = inst?.ocupado
  return (
    <div className="controles">
      <label>
        Épocas
        <input type="number" min="1" max="100000" value={epocas} onChange={(e) => setEpocas(Number(e.target.value))} />
      </label>
      <label className="check">
        <input type="checkbox" checked={verificar} onChange={(e) => setVerificar(e.target.checked)} />
        Verificar contra el secuencial (+{fmt((inst?.t1_epoca_ms ?? 0) * epocas / 1000, 0)} s)
      </label>
      {!ocupado ? (
        <button className="boton-principal" disabled={!ids.length || epocas < 1} onClick={() => apiCluster.entrenar(ids, epocas, verificar).catch((e) => onError(e.message))}>
          Entrenar con {ids.length} nodo{ids.length === 1 ? '' : 's'}
        </button>
      ) : (
        <button className="boton-peligro" onClick={() => apiCluster.cancelar().catch((e) => onError(e.message))}>
          Cancelar corrida
        </button>
      )}
      <button
        className="boton-mini"
        disabled={!sala.length}
        onClick={() => window.confirm('¿Despedir a todos los nodos? Se cerrarán en sus PCs.') && apiCluster.terminarSesion().catch((e) => onError(e.message))}
      >
        Terminar sesión
      </button>
    </div>
  )
}

// normalizar asegura que todas las series de la corrida sean listas.
function normalizar(c) {
  return {
    ...c,
    perdidas: comoLista(c.perdidas),
    t_epoca_ms: comoLista(c.t_epoca_ms),
    t_calculo_ms: comoLista(c.t_calculo_ms),
    t_red_ms: comoLista(c.t_red_ms),
    nodos: comoLista(c.nodos).map((n) => ({ ...n, calculo_ms: comoLista(n.calculo_ms) })),
  }
}

function Corrida({ inst, corrida: cruda }) {
  const corrida = normalizar(cruda)
  const n = Math.max(corrida.nodos.length, 1)
  const t1 = inst.t1_epoca_ms
  const tEpoca = promedioFinal(corrida.t_epoca_ms)
  const sumaEpoca = corrida.t_epoca_ms.reduce((a, b) => a + b, 0)
  const sumaRed = corrida.t_red_ms.reduce((a, b) => a + b, 0)
  const pctRed = sumaEpoca > 0 ? (100 * sumaRed) / sumaEpoca : 0
  const speedup = tEpoca > 0 && t1 > 0 ? t1 / tEpoca : NaN
  const throughput = tEpoca > 0 ? inst.ejemplos_entrenamiento / (tEpoca / 1000) : NaN
  const lento = corrida.nodos.reduce((a, b) => (a && promedioFinal(a.calculo_ms) >= promedioFinal(b.calculo_ms) ? a : b), null)
  const progreso = corrida.epocas ? (100 * corrida.epoca) / corrida.epocas : 0
  const r = corrida.resultado
  const etiquetaEstado = {
    entrenando: 'Entrenando',
    verificando: 'Verificando contra el secuencial…',
    terminado: 'Terminada',
    abortado: 'Abortada',
    cancelado: 'Cancelada',
  }[corrida.estado]

  return (
    <div className="corrida">
      <div className="corrida-cabecera">
        <strong>Corrida #{corrida.id} · {etiquetaEstado}</strong>
        <span>época {corrida.epoca} / {corrida.epocas}</span>
      </div>
      <div className="progreso" aria-label={`Progreso ${Math.round(progreso)}%`}>
        <div className={`progreso-fill ${corrida.estado}`} style={{ width: `${progreso}%` }} />
      </div>
      {corrida.mensaje && <p className="mensaje-corrida">{corrida.mensaje}</p>}

      <div className="kpis">
        <Kpi etiqueta="Tiempo por época" valor={`${fmt(tEpoca)} ms`} detalle={`${fmt(promedioFinal(corrida.t_calculo_ms))} cálculo + ${fmt(promedioFinal(corrida.t_red_ms))} red`} />
        <Kpi etiqueta="Comunicación" valor={`${fmt(pctRed, 0)} %`} detalle="del tiempo es red y espera" tema="Amdahl" />
        <Kpi etiqueta="Speedup en vivo" valor={`${fmt(speedup)}×`} detalle={`E = ${fmt((100 * speedup) / n, 0)} % · T(1) = ${fmt(t1, 0)} ms/época`} tema="S(p), E(p)" />
        <Kpi etiqueta="Throughput" valor={`${fmtEjemplos(throughput)}/s`} detalle="ejemplos procesados por segundo" />
        <Kpi etiqueta="Pérdida" valor={fmt(corrida.perdidas.at(-1) ?? NaN, 4)} detalle="log-loss ponderada" />
      </div>

      <div className="graficas-vivo">
        <GraficaApilada
          titulo="Tiempo por época: cálculo frente a red"
          tema="Sem. 1 · Amdahl"
          epocas={corrida.epocas}
          capas={[
            { nombre: 'Cálculo del nodo más lento', color: CALCULO, valores: corrida.t_calculo_ms },
            { nombre: 'Red y espera', color: RED, valores: corrida.t_red_ms },
          ]}
        />
        <GraficaLineas
          titulo="Cálculo por época de cada nodo"
          tema="Sem. 10 · Balanceo de carga"
          epocas={corrida.epocas}
          formatoY={(v) => `${v.toFixed(v < 10 ? 1 : 0)} ms`}
          series={corrida.nodos.map((nd, i) => ({ nombre: `${nd.id} (${esGPU(nd.motor) ? 'GPU' : 'CPU'})`, color: COLORES[i % COLORES.length], valores: nd.calculo_ms }))}
        />
        <GraficaLineas
          titulo="Speedup en vivo frente al secuencial"
          tema="Sem. 13 · Evaluación de la aceleración"
          epocas={corrida.epocas}
          formatoY={(v) => `${v.toFixed(v < 10 ? 1 : 0)}×`}
          referencia={{ valor: n, etiqueta: `ideal con ${n} nodo${n === 1 ? '' : 's'}` }}
          series={[{ nombre: 'Speedup', color: 'var(--serie-1)', valores: t1 > 0 ? mediaMovil(corrida.t_epoca_ms).map((t) => t1 / t) : [] }]}
        />
        <GraficaLineas
          titulo="Pérdida por época"
          tema="Descenso de gradiente"
          epocas={corrida.epocas}
          formatoY={(v) => v.toFixed(2)}
          series={[{ nombre: 'Pérdida', color: 'var(--serie-4)', valores: corrida.perdidas }]}
        />
      </div>

      <ul className="conceptos">
        {conceptos(inst, corrida, lento).map((c) => (
          <li key={c.tema}><span className="tema-silabo">{c.tema}</span> {c.texto}</li>
        ))}
      </ul>

      {r && (
        <div className="resultado-final">
          <strong>Resultado:</strong> {fmt(r.t_total_s, 2)} s en total, {fmt(r.t_epoca_ms)} ms por época
          ({fmt(r.t_calculo_ms)} de cálculo + {fmt(r.t_comunicacion_ms)} de red) · speedup {fmt(r.speedup)}× · AUC {fmt(r.auc, 3)}
          {r.verificado && (
            <> · <span className={r.diferencia < 1e-9 ? 'ok' : 'mal'}>
              {r.diferencia < 1e-9 ? 'mismo modelo que el secuencial' : 'distinto al secuencial'} (diferencia {r.diferencia.toExponential(1)})
            </span></>
          )}
        </div>
      )}
    </div>
  )
}

const describirNodos = (h) =>
  comoLista(h.nodos).map((id, i) => `${id}${esGPU(comoLista(h.motores)[i]) ? '·GPU' : '·CPU'}`).join(' + ')

// Historial: barras solo para las corridas terminadas (las canceladas o
// abortadas no tienen tiempos: se listan aparte, sin barra).
function Historial({ historial }) {
  const todas = comoLista(historial)
  const terminadas = todas.filter((h) => h.estado === 'terminado' && h.t_epoca_ms > 0).slice(-8)
  const otras = todas.filter((h) => !(h.estado === 'terminado' && h.t_epoca_ms > 0)).slice(-3)
  if (!terminadas.length && !otras.length) return null
  const filas = terminadas.map((h) => ({
    clave: String(h.id),
    etiqueta: `#${h.id} · ${describirNodos(h)}`,
    valores: { calculo: h.t_calculo_ms, red: h.t_comunicacion_ms },
    extra: `${h.t_total_s.toFixed(2)} s · S = ${fmt(h.speedup)} · E = ${fmt(100 * h.eficiencia, 0)} %${h.verificado ? ` · verificado (dif. ${h.diferencia.toExponential(1)})` : ''}`,
  }))
  const nombreEstado = { cancelado: 'cancelada', abortado: 'abortada' }
  return (
    <div className="historial">
      {filas.length > 0 && (
        <BarrasH
          titulo="Corridas de esta sesión (ms por época)"
          tema="Comparación en vivo"
          filas={filas}
          capas={[
            { clave: 'calculo', nombre: 'Cálculo del más lento', color: CALCULO },
            { clave: 'red', nombre: 'Red y espera', color: RED },
          ]}
        />
      )}
      {otras.length > 0 && (
        <p className="historial-otras">
          Sin completar:{' '}
          {otras.map((h) => `#${h.id} ${nombreEstado[h.estado] ?? h.estado} (${describirNodos(h)})`).join(' · ')}
        </p>
      )}
    </div>
  )
}

export default function PanelCluster({ datos, grande = false }) {
  const { conectado, inst } = useCluster()
  const [elegidos, setElegidos] = useState({})
  const [error, setError] = useState('')
  const corrida = inst?.corrida

  return (
    <figure className={`panel panel-cluster ${grande ? 'grande' : ''}`}>
      <figcaption>
        <div className="titulo-cluster">
          <h3>Clúster en vivo</h3>
          <span className={`estado-servidor ${conectado ? 'en-linea' : ''}`}>{conectado ? 'maestro en línea' : 'maestro sin conexión'}</span>
          <span className="tema-silabo">Sem. 3-4 · Paso de mensajes (maestro-trabajador por TCP)</span>
        </div>
        {conectado && inst && (
          <p>
            {comoLista(inst.sala).length} nodo(s) en la sala · {fmtEjemplos(inst.ejemplos_entrenamiento)} ejemplos de entrenamiento · muestra {(inst.huella ?? '').slice(0, 12)}
          </p>
        )}
      </figcaption>

      {!conectado ? (
        <div className="sin-servidor">
          <p>
            El maestro en modo servidor no está corriendo. En la PC del maestro:{' '}
            <code>.\bin\maestro.exe -web 127.0.0.1:8080 -modo tcp_tailscale -resultado resultados\experimento2.csv</code>
          </p>
          <ResultadosCluster experimento2={datos?.experimento2} experimento2Prueba={datos?.experimento2_prueba} compacto />
        </div>
      ) : (
        <>
          <Sala inst={inst} elegidos={elegidos} setElegidos={setElegidos} ocupado={inst?.ocupado} onError={setError} />
          <Controles inst={inst} elegidos={elegidos} onError={setError} />
          {error && (
            <p className="error-api" role="alert">
              {error} <button className="boton-mini" onClick={() => setError('')}>Cerrar</button>
            </p>
          )}
          {corrida && <Corrida inst={inst} corrida={corrida} />}
          {!corrida && (
            <ul className="conceptos">
              {conceptos(inst, null, null).map((c) => (
                <li key={c.tema}><span className="tema-silabo">{c.tema}</span> {c.texto}</li>
              ))}
            </ul>
          )}
          <Historial historial={inst?.historial ?? []} />
        </>
      )}
    </figure>
  )
}
