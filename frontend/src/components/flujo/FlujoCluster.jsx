// Flujo del clúster antes de entrenar (las 4 pantallas):
//   1. Inicio: Iniciar clúster (esta laptop es el maestro) o Unirse.
//   2. Sala: los nodos conectados; Iniciar limpieza o Saltar limpieza.
//   3. Limpieza: cada nodo limpia sus CSV con Pthreads y luego con Go; se
//      ve el % de CPU de cada uno a lo largo de su avance y su tiempo.
//   4. Antes de entrenar: elegir nodos y comprobar que todos tienen el
//      mismo muestra.bin (misma huella) que el maestro.
// Con el entrenamiento en curso se muestra el panel del clúster en vivo.
// Solo la laptop del maestro tiene los botones; las demás miran.
import { Fragment, useEffect, useMemo, useState } from 'react'
import { apiCluster, useCluster } from '../../data/cluster'
import { apiLocal } from '../../data/local'
import { NOMBRE_MOTOR, efectivo, hayCUDA, hayMPI, hayOpenMP, textoMotor } from './motores'
import ComparacionMotores from './ComparacionMotores'
import CuadrosEpocas from './CuadrosEpocas'
import Fases from './Fases'
import Historial from './Historial'
import { csvEntrenamiento, csvLimpieza, csvPrediccion, descargarCSV } from './csv'
import logoIntel from '../../assets/marcas/Intel.png'
import logoNvidia from '../../assets/marcas/NVIDIA.png'
import logoRyzen from '../../assets/marcas/Ryzen.svg'
import './FlujoCluster.css'

const corta = (h) => (h ? h.slice(0, 12) : '')
const fmtS = (s) => (Number.isFinite(s) && s > 0 ? `${s.toLocaleString('es-PE', { maximumFractionDigits: 1 })} s` : '—')
const NOMBRE_TEC = { pthreads: 'Pthreads (C)', go: 'Go' }

// ip del nodo: la de su conexión. El nodo que corre en la laptop del maestro
// llega por 127.0.0.1: para él se usa la IP del maestro (ipMaestro).
function ipDe(nodo, ipMaestro) {
  const ip = (nodo.remoto ?? '').replace(/:\d+$/, '').replace(/^\[|\]$/g, '')
  return ip === '127.0.0.1' || ip === '::1' ? ipMaestro : ip
}

// En Windows el procesador llega como "... AuthenticAMD" o "... GenuineIntel".
function marcaCPU(cpu = '') {
  if (/ryzen|amd/i.test(cpu)) return { clase: 'amd', logo: logoRyzen, nombre: /ryzen/i.test(cpu) ? 'AMD Ryzen' : 'AMD' }
  if (/intel/i.test(cpu)) return { clase: 'intel', logo: logoIntel, nombre: 'Intel' }
  return null
}

// Tarjeta de un nodo: el color de fondo y del borde salen de la marca del
// procesador (Ryzen naranja, Intel celeste); el maestro y los elegidos para
// entrenar se resaltan más. Debajo: nombre, tipo (Maestro o Nodo) e IP.
function Nodo({ nodo, tipo, rol, ip, nota, extra, onClick, pequeno, elegido }) {
  const hw = nodo?.hardware ?? {}
  const cpu = marcaCPU(hw.cpu)
  const Etiqueta = onClick ? 'button' : 'div'
  const clases = ['nodo-caja', tipo, cpu ? `marca-${cpu.clase}` : 'marca-otra', elegido ? 'elegido' : ''].join(' ')
  return (
    <div className={`nodo ${pequeno ? 'pequeno' : ''}`}>
      <Etiqueta
        type={onClick ? 'button' : undefined}
        className={clases}
        onClick={onClick}
        aria-pressed={onClick ? tipo !== 'deseleccionado' : undefined}
        title={hw.cpu ? `${hw.cpu}${hw.gpu ? ` · ${hw.gpu}` : ''}` : undefined}
      >
        {hw.gpu && <img className="logo logo-nvidia" src={logoNvidia} alt="GPU NVIDIA" />}
        {cpu && <img className={`logo logo-cpu ${cpu.clase}`} src={cpu.logo} alt={cpu.nombre} />}
      </Etiqueta>
      {nodo && (
        <span className="nodo-nombre">
          {nodo.id}
          {!pequeno && (
            <>
              <small className={`nodo-rol ${tipo === 'maestro' || rol === 'Maestro' ? 'maestro' : ''}`}>{rol}</small>
              {ip && <small>{ip}</small>}
              {extra && <small className="nodo-extra">{extra}</small>}
              {nota && <small className="nodo-nota">{nota}</small>}
            </>
          )}
        </span>
      )}
      {!nodo && <span className="nodo-nombre">(Libre)</span>}
    </div>
  )
}

// ---------- 1. Inicio ----------

function Inicio({ local, actualizar }) {
  const [id, setId] = useState('')
  const [maestro, setMaestro] = useState('')
  const [participar, setParticipar] = useState(true)
  const [ocupado, setOcupado] = useState('')
  const [error, setError] = useState('')
  const vistos = local.clusters ?? []
  const listo = local.listo ?? {}
  const nombre = id || local.id

  const hacer = async (que, accion) => {
    setOcupado(que)
    setError('')
    try {
      await accion()
      await actualizar()
    } catch (e) {
      setError(e.message)
    } finally {
      setOcupado('')
    }
  }

  const faltan = [
    !listo.maestro && 'bin/maestro',
    !listo.trabajador && 'bin/trabajador',
    !listo.cargador_pthreads && 'bin/cargador_pthreads (sin él, esta laptop limpia solo con Go)',
    !listo.csv && 'los 4 CSV en data/raw (sin ellos no puede limpiar)',
  ].filter(Boolean)

  return (
    <section className="flujo-inicio" aria-label="Iniciar o unirse al clúster">
      <label className="campo">
        <span>Nombre de esta laptop en el clúster</span>
        <input value={id} placeholder={local.id} onChange={(e) => setId(e.target.value.trim())} />
      </label>

      <div className="inicio-opcion">
        <button
          type="button"
          className="boton-grande"
          disabled={!!ocupado || vistos.length > 0}
          onClick={() => hacer('iniciar', () => apiLocal.iniciar(nombre, participar))}
        >
          {ocupado === 'iniciar' ? 'Iniciando…' : 'INICIAR CLÚSTER'}
        </button>
        <IlustracionCluster />
        <label className="casilla">
          <input
            type="checkbox"
            checked={participar}
            disabled={!!ocupado || vistos.length > 0}
            onChange={(e) => setParticipar(e.target.checked)}
          />
          Esta laptop también participa como nodo
        </label>
        {vistos.length > 0 && (
          <p className="aviso">
            Ya hay un clúster iniciado por <strong>{vistos[0].host}</strong> ({vistos[0].ip}): únete a él.
          </p>
        )}
      </div>

      <div className="inicio-opcion">
        <button
          type="button"
          className="boton-grande"
          disabled={!!ocupado}
          onClick={() => hacer('unirse', () => apiLocal.unirse(nombre, maestro))}
        >
          {ocupado === 'unirse' ? 'Uniéndose…' : 'UNIRSE AL CLÚSTER'}
        </button>
        <IlustracionNodo />
        {vistos.length === 1 && !maestro ? (
          <p className="detalle">
            Maestro detectado: <strong>{vistos[0].host}</strong> ({vistos[0].ip})
          </p>
        ) : (
          <label className="campo">
            <span>{vistos.length > 1 ? 'Hay varios clústeres: IP del maestro' : 'IP del maestro (no se detectó ninguno)'}</span>
            <input value={maestro} placeholder="192.168.50.1" onChange={(e) => setMaestro(e.target.value.trim())} />
          </label>
        )}
      </div>

      {error && <p className="error" role="alert">{error}</p>}
      {local.error && !error && <p className="error">Última sesión: {local.error}</p>}
      {faltan.length > 0 && (
        <p className="detalle faltan">Falta en esta laptop: {faltan.join(' · ')}. Corre scripts/saca.sh o scripts/saca.ps1.</p>
      )}
    </section>
  )
}

function IlustracionCluster() {
  return (
    <svg className="ilustracion" viewBox="0 0 120 90" aria-hidden="true">
      <g fill="none" stroke="currentColor" strokeWidth="3">
        <ellipse cx="60" cy="14" rx="20" ry="7" />
        <path d="M40 14v18c0 4 9 7 20 7s20-3 20-7V14" />
        <path d="M60 39v18M60 48H22v12M60 48h38v12" />
        <rect x="8" y="60" width="28" height="18" rx="2" />
        <rect x="46" y="62" width="28" height="18" rx="2" />
        <rect x="84" y="60" width="28" height="18" rx="2" />
      </g>
    </svg>
  )
}

function IlustracionNodo() {
  return (
    <svg className="ilustracion" viewBox="0 0 120 90" aria-hidden="true">
      <g fill="none" stroke="currentColor" strokeWidth="3">
        <rect x="22" y="8" width="20" height="46" rx="2" />
        <rect x="50" y="8" width="20" height="46" rx="2" />
        <rect x="78" y="8" width="20" height="46" rx="2" />
        <path d="M32 54v10h56V54M60 64v6" />
        <circle cx="60" cy="78" r="9" />
        <path d="M55 78l4 4 7-8" />
      </g>
    </svg>
  )
}

// ---------- 2 y 4. Sala ----------

function Sala({ inst, local, esMaestro, modo, seleccion, setSeleccion, onLimpiar, onSaltar, onEntrenar, onRealizarLimpieza, ocupado, error, avisos, ajustes, setAjustes }) {
  const sala = inst?.sala ?? []
  const maestroNodo = sala.find((n) => n.id === inst?.nodo_maestro)
  const otros = sala.filter((n) => n !== maestroNodo)
  // Máximo 8 laptops (inst.capacidad: 8 si el maestro participa como nodo,
  // 7 si no, porque su laptop igual cuenta).
  const libres = Math.max(0, (inst?.capacidad ?? 8) - sala.length)
  const enPre = modo === 'pre'

  const ipMaestro = esMaestro ? local.ip : (local.maestro ?? '').replace(/:\d+$/, '')
  const tipo = (n, base) => {
    if (enPre && !seleccion.has(n.id)) return 'deseleccionado'
    if (enPre && n.huella !== inst.huella) return `${base} distinto`
    return base
  }
  const elegido = (n) => enPre && seleccion.has(n.id) && n.huella === inst.huella
  const alternar = (n) =>
    enPre && esMaestro
      ? () =>
          setSeleccion((s) => {
            const nueva = new Set(s)
            if (nueva.has(n.id)) nueva.delete(n.id)
            else nueva.add(n.id)
            return nueva
          })
      : undefined
  const sug = inst?.pesos_sugeridos ?? {}
  const porCapacidad = enPre && ajustes?.reparto === 'capacidad' && Object.keys(sug).length > 0
  const nota = (n) => {
    if (enPre && n.huella !== inst.huella) return n.huella ? `otra muestra (${corta(n.huella)})` : 'sin muestra'
    return undefined
  }
  // Peso del nodo en el reparto proporcional (debajo de su IP).
  const peso = (n) => (porCapacidad && seleccion.has(n.id) && sug[n.id] ? `capacidad ×${sug[n.id].toLocaleString('es-PE')}` : undefined)

  return (
    <section className="flujo-sala" aria-label={enPre ? 'Antes de entrenar' : 'Clúster iniciado'}>
      <div className="sala-centro">
        <span className="rotulo">CLÚSTER INICIADO</span>
        {maestroNodo && enPre ? (
          // Antes de entrenar, el maestro se elige como nodo en la figura de
          // la derecha; aquí solo se muestra quién coordina.
          <Nodo nodo={maestroNodo} tipo="maestro coordinador" rol="Maestro" ip={ipMaestro} />
        ) : maestroNodo ? (
          <Nodo nodo={maestroNodo} tipo={tipo(maestroNodo, 'maestro')} rol="Maestro" ip={ipMaestro} nota={nota(maestroNodo)} extra={peso(maestroNodo)}
            elegido={elegido(maestroNodo)} onClick={alternar(maestroNodo)} />
        ) : (
          <Nodo nodo={{ id: esMaestro ? local.id : 'maestro', hardware: esMaestro ? local.hardware : undefined }} tipo="maestro"
            rol="Maestro (no participa)" ip={ipMaestro} />
        )}
        {esMaestro && !enPre && (
          <div className="sala-botones">
            <button type="button" className="boton-grande" disabled={!!ocupado || sala.length === 0} onClick={onLimpiar}>INICIAR LIMPIEZA</button>
            <button type="button" className="boton" disabled={!!ocupado || sala.length === 0} onClick={onSaltar}>
              {ocupado === 'saltar' ? 'Comprobando huellas…' : 'SALTAR LIMPIEZA'}
            </button>
          </div>
        )}
        {esMaestro && enPre && (
          <div className="sala-botones">
            <button type="button" className="boton-grande" disabled={!!ocupado || seleccion.size === 0} onClick={onEntrenar}>
              {ocupado === 'entrenar' ? 'Comprobando huellas…' : 'INICIAR ENTRENAMIENTO'}
            </button>
            <button type="button" className="boton" disabled={!!ocupado} onClick={onRealizarLimpieza}>REALIZAR LIMPIEZA</button>
            <AjustesModelo ajustes={ajustes} setAjustes={setAjustes} nodos={sala.filter((n) => seleccion.has(n.id))} />
            <AjustesMotor ajustes={ajustes} setAjustes={setAjustes} nodos={sala.filter((n) => seleccion.has(n.id))} />
            <AjustesReparto ajustes={ajustes} setAjustes={setAjustes} inst={inst} />
          </div>
        )}
      </div>

      {enPre ? (
        <TopologiaPrevia
          sala={sala} maestroNodo={maestroNodo} seleccion={seleccion} ajustes={ajustes}
          tarjeta={(n) => (
            <Nodo nodo={n} tipo={tipo(n, n === maestroNodo ? 'maestro' : 'trabajador')} nota={nota(n)} extra={peso(n)}
              elegido={elegido(n)} onClick={alternar(n)} />
          )}
        />
      ) : (
      <div className="sala-nodos">
        {otros.map((n) => (
          <Nodo key={n.id} nodo={n} tipo={tipo(n, 'trabajador')} rol="Nodo" ip={ipDe(n, ipMaestro)} nota={nota(n)} extra={peso(n)}
            elegido={elegido(n)} onClick={alternar(n)} />
        ))}
        {Array.from({ length: libres }, (_, i) => <Nodo key={`libre${i}`} tipo="libre" />)}
      </div>
      )}

      <div className="sala-pie">
        {avisos?.map((a) => <p key={a} className="aviso">{a}</p>)}
        {error && <p className="error" role="alert">{error}</p>}
      </div>
    </section>
  )
}

// Durante el entrenamiento: la figura de la comunicación con cada nodo de
// un tamaño según la parte de las filas que calculó en la última época
// (partes iguales: todos iguales; según la capacidad: fijos; dinámico:
// cambian en cada época), con un tope para que ninguno tape a los demás.
function TopologiaEntrenamiento({ corrida, inst }) {
  const sala = new Map((inst.sala ?? []).map((nd) => [nd.id, nd]))
  const maestro = (inst.sala ?? []).find((nd) => nd.id === inst.nodo_maestro)
  const nodos = corrida.nodos ?? []
  const n = Math.max(nodos.length, 1)
  // Parte de cada nodo: la última época medida o, si no hay, los ejemplos.
  const ultima = (corrida.fraccion ?? []).at(-1)
  const ejemplos = nodos.reduce((a, nd) => a + (nd.ejemplos ?? 0), 0)
  const parte = (i) => ultima?.[i] ?? (ejemplos > 0 ? nodos[i].ejemplos / ejemplos : 1 / n)
  const indice = new Map(nodos.map((nd, i) => [nd.id, i]))
  const reparto = { dinamico: 'reparto dinámico', capacidad: 'reparto según la capacidad', iguales: 'partes iguales' }[corrida.reparto]
  const comunicacion = corrida.comunicacion || 'central'
  return (
    <Topologia
      corrida={{ ...corrida, comunicacion }} sala={sala} maestro={maestro} detallado fila={200}
      subtitulo={`${NOMBRE_COMUNICACION[comunicacion]}${reparto ? ` · ${reparto}` : ''}`}
      tarjeta={(nd) => {
        const i = indice.get(nd.id)
        const f = i == null ? null : parte(i)
        // 1 = la parte de un reparto igual; entre 0,7 y 1,6 veces.
        const escala = f == null ? 1 : Math.min(1.6, Math.max(0.7, Math.sqrt(f * n)))
        return (
          <div className="topo-escala" style={{ '--escala': escala.toFixed(3) }}>
            <Nodo nodo={nd} tipo={nd.id === maestro?.id ? 'maestro' : 'nodo'} extra={f != null ? `${(f * 100).toFixed(1)} % de las filas` : undefined} />
          </div>
        )
      }}
    />
  )
}

const fmtS2 = (s) => (s == null ? '—' : `${s.toLocaleString('es-PE', { maximumFractionDigits: s < 10 ? 2 : 1 })} s`)
const fmtNum = (v, d = 0) => (v == null || Number.isNaN(v) ? '—' : v.toLocaleString('es-PE', { maximumFractionDigits: d, minimumFractionDigits: d }))

// Resultados del entrenamiento: los números clave, un cuadro por época y la
// comunicación entre los nodos.
function ResultadoEntrenamiento({ corrida, inst, esMaestro, vivo, onSiguiente }) {
  const [error, setError] = useState('')
  const enCurso = vivo && (corrida.estado === 'entrenando' || corrida.estado === 'verificando')
  const r = corrida.resultado
  const ultimo = (serie) => serie?.at(-1)
  const media = (serie) => (serie?.length ? serie.reduce((a, b) => a + b, 0) / serie.length : null)
  const estado = {
    entrenando: `Entrenando · época ${corrida.epoca} de ${corrida.epocas}`,
    verificando: `Verificando contra el secuencial · época ${corrida.verificacion_epoca}`,
    terminado: 'Terminado',
    cancelado: 'Cancelado',
    error: 'Con error',
  }[corrida.estado] ?? corrida.estado
  const datos = [
    ['Estado', estado],
    ['Modelo', (corrida.modelo ?? '').startsWith('red') ? `Red Neuronal · ${corrida.pasos_por_epoca} mini-lotes` : 'Regresión logística'],
    ...(corrida.motor_cpu ? [['Motor', `${NOMBRE_MOTOR[corrida.motor_cpu]}${corrida.motores?.length > 1 ? ` (${corrida.motores.indexOf(corrida.motor_cpu) + 1} de ${corrida.motores.length})` : ''}${corrida.cuda ? ' + CUDA' : ''}`]] : []),
    ['Tiempo por época', `${fmtNum(media(corrida.t_epoca_ms))} ms`],
    ['Cálculo / red y espera', `${fmtNum(media(corrida.t_calculo_ms))} / ${fmtNum(media(corrida.t_red_ms))} ms`],
    ['Por la red', corrida.mb_epoca?.length ? `${fmtNum(media(corrida.mb_epoca), 1)} MB por época${corrida.comunicacion === 'mpi' ? ' (estimado)' : ''}` : '—'],
    ['Pérdida', fmtNum(ultimo(corrida.perdidas), 4)],
  ]
  if (r) {
    datos.push(['Total', fmtS2(r.t_total_s)])
    if (r.speedup > 0) datos.push(['Speedup', `${fmtNum(r.speedup, 2)}×`])
    datos.push(['AUC / F1', `${fmtNum(r.auc, 3)} / ${fmtNum(r.f1, 3)}`])
  }
  return (
    <>
      <section className="flujo-limpieza resumen-entrenamiento" aria-label="Entrenamiento">
        <dl className="datos-entrenamiento">
          {datos.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
        {corrida.mensaje && <p className="error">{corrida.mensaje}</p>}
        <CuadrosEpocas corrida={corrida} enCurso={enCurso && corrida.estado === 'entrenando'} />
        {esMaestro && vivo && (
          <div className="sala-botones sala-botones-fila">
            {enCurso && (
              <button type="button" className="boton" onClick={() => apiCluster.cancelar().catch((e) => setError(e.message))}>Cancelar entrenamiento</button>
            )}
            {!enCurso && corrida.estado === 'terminado' && inst.modelo_listo && (
              <button type="button" className="boton-grande" onClick={onSiguiente}>Siguiente: predicción distribuida</button>
            )}
          </div>
        )}
        {error && <p className="error" role="alert">{error}</p>}
      </section>
      <ComparacionMotores corrida={corrida} tarjeta={(n) => (
        <Nodo nodo={(inst.sala ?? []).find((x) => x.id === n.id) ?? n} tipo={n.id === inst.nodo_maestro ? 'maestro' : 'trabajador'} pequeno />
      )} />
      <TopologiaEntrenamiento corrida={corrida} inst={inst} />
    </>
  )
}

// Nuevo → Predicción distribuida: con qué nodos y el botón para empezar.
function NuevoPrediccion({ inst, esMaestro, onPredecir, ocupado, error, ajustes, setAjustes }) {
  const sala = new Map((inst.sala ?? []).map((n) => [n.id, n]))
  const maestro = sala.get(inst.nodo_maestro)
  // Los nodos del último entrenamiento que siguen en la sala; si no, todos.
  const deCorrida = (inst.corrida?.nodos ?? []).map((n) => sala.get(n.id)).filter(Boolean)
  const nodos = deCorrida.length ? deCorrida : inst.sala ?? []
  return (
    <section className="flujo-sala" aria-label="Nueva predicción distribuida">
      <div className="sala-centro">
        <span className="rotulo">CLÚSTER INICIADO</span>
        {maestro && <Nodo nodo={maestro} tipo="maestro coordinador" rol="Maestro" />}
        {esMaestro && (
          <div className="sala-botones">
            <button type="button" className="boton-grande" disabled={!!ocupado || !inst.modelo_listo || nodos.length === 0} onClick={() => onPredecir(nodos)}>
              {ocupado === 'predecir' ? 'Iniciando…' : 'INICIAR PREDICCIÓN'}
            </button>
            <AjustesMotor ajustes={ajustes} setAjustes={setAjustes} nodos={nodos} soloUno />
            <label className="ajustes-select">
              Juntar los resultados
              <select value={ajustes.gather === 'mpi' && hayMPI(nodos) ? 'mpi' : 'tcp'} onChange={(e) => setAjustes((a) => ({ ...a, gather: e.target.value }))}>
                <option value="tcp">TCP (cada nodo al maestro)</option>
                <option value="mpi" disabled={!hayMPI(nodos)}>MPI (Gatherv){hayMPI(nodos) ? '' : ': faltan MPI o hay sistemas distintos'}</option>
              </select>
            </label>
          </div>
        )}
        <p className="detalle">
          {inst.modelo_listo
            ? 'Cada nodo predice todas las horas de sus estaciones con el último modelo entrenado y le envía los resultados al maestro.'
            : 'Todavía no hay un modelo entrenado con esta muestra: primero entrena uno.'}
        </p>
      </div>
      <div className="sala-topologia">
        {nodos.length > 0 && (
          <Topologia corrida={{ nodos, comunicacion: 'central' }} maestro={maestro} rotulo="PREDICCIÓN DISTRIBUIDA" subtitulo="Cada nodo envía sus predicciones al maestro" />
        )}
      </div>
      <div className="sala-pie">{error && <p className="error" role="alert">{error}</p>}</div>
    </section>
  )
}

// Antes de entrenar: los nodos elegidos (el maestro incluido, si participa)
// dibujados con la comunicación que se va a usar; cambia en cuanto se cambia
// el modelo o la comunicación. La regresión logística siempre va por el
// maestro. Los que no participan quedan abajo, en gris; tocar una tarjeta
// la quita o la suma.
function TopologiaPrevia({ sala, maestroNodo, seleccion, ajustes, tarjeta }) {
  const elegidos = sala.filter((n) => seleccion.has(n.id))
  const fuera = sala.filter((n) => !seleccion.has(n.id))
  const comunicacion = ajustes.modelo === 'red' ? efectivo(ajustes, elegidos).comunicacion : 'central'
  return (
    <div className="sala-topologia">
      {elegidos.length > 0 ? (
        <Topologia
          corrida={{ nodos: elegidos, comunicacion }}
          maestro={maestroNodo}
          tarjeta={tarjeta}
          detallado
          fila={175}
        />
      ) : (
        <p className="detalle">Elige al menos un nodo.</p>
      )}
      {fuera.length > 0 && (
        <div className="sala-fuera" aria-label="No participan">
          <span className="rotulo">NO PARTICIPAN</span>
          <div className="sala-fuera-nodos">
            {fuera.map((n) => <div key={n.id}>{tarjeta(n)}</div>)}
          </div>
        </div>
      )}
    </div>
  )
}

// Modelo a entrenar: regresión logística (13 o 21 parámetros, una
// actualización por época) o red neuronal 20→64→32→1 por mini-lotes.
// Videos cortos que explican cada modelo (YouTube, no listados).
const VIDEOS_MODELO = {
  red: { id: 'CEuw0KxSIVA', titulo: 'Red Neuronal' },
  logistica: { id: '35_YTw1YemA', titulo: 'Regresión logística' },
}

// Ventana emergente con el video: se cierra con ×, con Esc o tocando fuera.
function VentanaVideo({ video, onCerrar }) {
  useEffect(() => {
    const tecla = (e) => e.key === 'Escape' && onCerrar()
    window.addEventListener('keydown', tecla)
    return () => window.removeEventListener('keydown', tecla)
  }, [onCerrar])
  return (
    <div className="modal-fondo" role="presentation" onClick={onCerrar}>
      <div className="modal modal-video" role="dialog" aria-modal="true" aria-label={video.titulo} onClick={(e) => e.stopPropagation()}>
        <header>
          <h3>{video.titulo}</h3>
          <button type="button" className="cerrar" onClick={onCerrar} aria-label="Cerrar">×</button>
        </header>
        <div className="video-marco">
          <iframe
            src={`https://www.youtube-nocookie.com/embed/${video.id}?autoplay=1&rel=0`}
            title={video.titulo}
            allow="autoplay; encrypted-media; picture-in-picture; fullscreen"
            allowFullScreen
          />
        </div>
        <a className="detalle" href={`https://youtu.be/${video.id}`} target="_blank" rel="noreferrer">Abrir en YouTube</a>
      </div>
    </div>
  )
}

function BotonVideo({ modelo, onVer }) {
  const v = VIDEOS_MODELO[modelo]
  return (
    <button type="button" className="boton-video" onClick={() => onVer(v)} title={`Ver video: ${v.titulo}`} aria-label={`Ver video: ${v.titulo}`}>
      ▶
    </button>
  )
}

function AjustesModelo({ ajustes, setAjustes, nodos }) {
  const [video, setVideo] = useState(null)
  const cambiar = (campo) => (e) => {
    const v = e.target.type === 'number' ? Math.max(1, Number(e.target.value) || 1) : e.target.value
    setAjustes((a) => {
      const nuevo = { ...a, [campo]: v }
      // Cada modelo con sus épocas habituales.
      if (campo === 'modelo') nuevo.epocas = v === 'red' ? 20 : 200
      return nuevo
    })
  }
  return (
    <fieldset className="ajustes-modelo">
      <legend>Modelo</legend>
      <div className="opcion-modelo">
        <label className="casilla">
          <input type="radio" name="modelo" value="red" checked={ajustes.modelo === 'red'} onChange={cambiar('modelo')} />
          Red Neuronal
        </label>
        <BotonVideo modelo="red" onVer={setVideo} />
      </div>
      <div className="opcion-modelo">
        <label className="casilla">
          <input type="radio" name="modelo" value="logistica" checked={ajustes.modelo === 'logistica'} onChange={cambiar('modelo')} />
          Regresión logística
        </label>
        <BotonVideo modelo="logistica" onVer={setVideo} />
      </div>
      {video && <VentanaVideo video={video} onCerrar={() => setVideo(null)} />}
      <div className="ajustes-numeros">
        <label>Épocas <input type="number" min="1" max="100000" value={ajustes.epocas} onChange={cambiar('epocas')} /></label>
        {ajustes.modelo === 'red' && (
          <label>Mini-lotes por época <input type="number" min="1" max="10000" value={ajustes.pasos} onChange={cambiar('pasos')} /></label>
        )}
      </div>
      {ajustes.modelo === 'red' && (
        <label className="ajustes-select">
          Comunicación
          <select value={efectivo(ajustes, nodos).comunicacion} onChange={cambiar('comunicacion')}>
            <option value="arbol">En árbol entre los nodos</option>
            <option value="anillo">En anillo entre los nodos</option>
            <option value="central">Todo por el maestro</option>
            <option value="mpi" disabled={!hayMPI(nodos)}>MPI (Allreduce){hayMPI(nodos) ? '' : ': faltan MPI o hay sistemas distintos'}</option>
          </select>
        </label>
      )}
    </fieldset>
  )
}

// Con qué calcula cada laptop (Fase 6): OpenMP y Go, uno tras otro, para
// compararlos; o uno solo. Con CUDA, las laptops con NVIDIA reparten cada
// mini-lote entre la CPU y la GPU.
function AjustesMotor({ ajustes, setAjustes, nodos, soloUno }) {
  const omp = hayOpenMP(nodos), gpu = hayCUDA(nodos)
  const { motores } = efectivo(ajustes, nodos)
  const elegido = soloUno ? ajustes.motorPrediccion : motores.length > 1 ? 'comparar' : motores[0]
  const campo = soloUno ? 'motorPrediccion' : 'motores'
  const cambiar = (e) => setAjustes((a) => ({ ...a, [campo]: e.target.value }))
  const sinOMP = nodos.filter((n) => !n.motores?.includes('openmp')).map((n) => n.id)
  const opciones = [
    ...(soloUno ? [] : [['comparar', 'Comparar OpenMP y Go']]),
    ['openmp', soloUno ? 'OpenMP' : 'Solo OpenMP'],
    ['go', soloUno ? 'Go' : 'Solo Go'],
  ]
  const valor = !omp ? 'go' : elegido
  return (
    <fieldset className="ajustes-modelo">
      <legend>Cálculo en cada laptop</legend>
      {opciones.map(([v, texto]) => (
        <label key={v} className="casilla">
          <input type="radio" name="motor" value={v} checked={valor === v} disabled={v !== 'go' && !omp} onChange={cambiar} />
          {texto}
        </label>
      ))}
      <label className="casilla">
        <input type="checkbox" checked={ajustes.cuda && gpu} disabled={!gpu} onChange={(e) => setAjustes((a) => ({ ...a, cuda: e.target.checked }))} />
        Cooperar con CUDA (laptops con NVIDIA)
      </label>
      {sinOMP.length > 0 && <p className="detalle">Sin OpenMP: {sinOMP.join(', ')}.</p>}
      {!gpu && <p className="detalle">Ninguna laptop tiene compilado el motor CUDA.</p>}
    </fieldset>
  )
}

// Reparto de los datos entre los nodos (Fase 3): según la capacidad que
// midió el maestro (último entrenamiento, o la limpieza), partes iguales o
// dinámico (solo la red neuronal: se reajusta al final de cada época según
// lo que tardó cada nodo; empieza con la capacidad medida, si la hay).
function AjustesReparto({ ajustes, setAjustes, inst }) {
  const hay = Object.keys(inst?.pesos_sugeridos ?? {}).length > 0
  const esRed = ajustes.modelo === 'red'
  const origen = { entrenamiento: 'medida en el último entrenamiento', limpieza: 'medida en la limpieza' }[inst?.origen_pesos]
  const cambiar = (e) => setAjustes((a) => ({ ...a, reparto: e.target.value }))
  const elegido = ajustes.reparto === 'dinamico' && !esRed ? (hay ? 'capacidad' : 'igual') : ajustes.reparto === 'capacidad' && !hay ? 'igual' : ajustes.reparto
  return (
    <fieldset className="ajustes-modelo">
      <legend>Reparto de los datos</legend>
      <label className="casilla">
        <input type="radio" name="reparto" value="dinamico" checked={elegido === 'dinamico'} disabled={!esRed} onChange={cambiar} />
        Dinámico (se ajusta en cada época)
      </label>
      <label className="casilla">
        <input type="radio" name="reparto" value="capacidad" checked={elegido === 'capacidad'} disabled={!hay} onChange={cambiar} />
        Según la capacidad de cada nodo
      </label>
      <label className="casilla">
        <input type="radio" name="reparto" value="igual" checked={elegido === 'igual'} onChange={cambiar} />
        Partes iguales
      </label>
      <p className="detalle">
        {elegido === 'dinamico'
          ? 'Al final de cada época, cada nodo recibe filas según lo rápido que calculó.'
          : hay ? `Capacidad ${origen}.` : 'Sin mediciones todavía: limpia o entrena una vez para medir la capacidad.'}
      </p>
    </fieldset>
  )
}

// Pantalla del entrenamiento: cómo viajan los gradientes en cada mini-lote
// (Fase 4). Por el maestro, todos le envían a él (un solo puerto); en árbol,
// los pares se suman de a dos por nivel; en anillo, cada uno al siguiente y
// todos a la vez.
// Figura de la comunicación entre nodos: las mismas tarjetas de la sala
// (logo del procesador y nombre) unidas por flechas. Árbol: el rango 0
// arriba y cada nivel debajo (la flecha va del hijo a su padre en la
// reducción; la suma vuelve por el mismo camino). Anillo: en círculo.
// Central: el maestro al centro.
const NOMBRE_COMUNICACION = { central: 'Todo por el maestro', arbol: 'En árbol', anillo: 'En anillo', mpi: 'Con MPI (Allreduce)' }

function Topologia({ corrida, sala, maestro, rotulo = 'COMUNICACIÓN', subtitulo, tarjeta, detallado, fila = 150, children }) {
  const nodos = (corrida.nodos ?? []).map((n) => sala?.get(n.id) ?? n)
  const n = nodos.length
  const tipo = corrida.comunicacion || 'central'
  const W = 680, FILA = fila
  let H, pos
  const flechas = []
  if (tipo === 'arbol') {
    // En el árbol binomial, la profundidad de r es su cantidad de bits en 1
    // y su padre es r sin el bit más bajo.
    const prof = (r) => r.toString(2).replace(/0/g, '').length
    const filas = []
    nodos.forEach((_, r) => (filas[prof(r)] ??= []).push(r))
    H = filas.length * FILA
    pos = []
    filas.forEach((fila, d) => fila.forEach((r, k) => { pos[r] = [((k + 0.5) / fila.length) * W, d * FILA + FILA / 2] }))
    for (let r = 1; r < n; r++) flechas.push([r, r - (r & -r), prof(r) - 1])
  } else if (n <= 2) {
    // Uno o dos nodos: en fila, a los lados del maestro (central) o uno
    // frente al otro (anillo de 2: una flecha de ida arriba y otra de vuelta
    // abajo, separadas para que se vean las dos).
    H = 200
    const xs = tipo === 'central' ? (n === 1 ? [W / 2 - 110] : [W / 2 - 220, W / 2 + 220]) : n === 1 ? [W / 2] : [W / 2 - 180, W / 2 + 180]
    pos = nodos.map((_, i) => [xs[i], H / 2])
    if (tipo === 'central') nodos.forEach((_, i) => flechas.push([i, 'M', 0, 0]))
    else if (n === 2 && tipo === 'mpi') flechas.push([0, 1, 0, 0])
    else if (n === 2) flechas.push([0, 1, 0, -14], [1, 0, 0, -14])
  } else {
    const central = tipo === 'central'
    H = central ? 560 : n > 4 ? 480 : 400
    const radio = [Math.min(260, 100 + 40 * n), H / 2 - 80]
    pos = nodos.map((_, i) => {
      const a = (2 * Math.PI * i) / n - Math.PI / 2
      return [W / 2 + radio[0] * Math.cos(a), H / 2 + radio[1] * Math.sin(a)]
    })
    if (tipo === 'anillo') nodos.forEach((_, i) => flechas.push([i, (i + 1) % n, 0]))
    else if (tipo === 'mpi') nodos.forEach((_, i) => { for (let j = i + 1; j < n; j++) flechas.push([i, j, 0]) }) // el camino lo elige MPI
    else nodos.forEach((_, i) => flechas.push([i, 'M', 0]))
  }
  // El maestro (central) va al centro; con un solo nodo, la pareja queda centrada.
  const centro = [tipo === 'central' && n === 1 ? W / 2 + 110 : W / 2, H / 2]
  const punto = (x) => (x === 'M' ? centro : pos[x])
  const colores = ['var(--serie-1)', 'var(--serie-4)', 'var(--serie-3)']
  return (
    <section className={`flujo-topologia ${detallado ? 'detallado' : ''}`} aria-label="Comunicación entre nodos">
      <header className="topo-titulo">
        <span className="rotulo">{rotulo}</span>
        <span className="topo-subtitulo">{subtitulo ?? NOMBRE_COMUNICACION[tipo]}</span>
      </header>
      <div className="topo-lienzo" style={{ aspectRatio: `${W} / ${H}`, maxWidth: W }}>
        <svg viewBox={`0 0 ${W} ${H}`} aria-hidden="true">
          <defs>
            {colores.map((c, k) => (
              <marker key={k} id={`punta-${k}`} viewBox="0 0 10 10" refX="5" refY="5" markerUnits="userSpaceOnUse" markerWidth="14" markerHeight="14" orient="auto">
                <path d="M0 0L10 5L0 10z" style={{ fill: c }} />
              </marker>
            ))}
          </defs>
          {flechas.map(([a, b, nivel, desvio = 0], i) => {
            // desvio: corre la flecha a un lado (perpendicular a su trazo).
            const [ax, ay] = punto(a), [bx, by] = punto(b)
            const largo = Math.hypot(bx - ax, by - ay) || 1
            const px = (-(by - ay) / largo) * desvio, py = ((bx - ax) / largo) * desvio
            const [x1, y1, x2, y2] = [ax + px, ay + py, bx + px, by + py]
            const k = nivel % colores.length
            // d = distancia de la punta al centro del destino. Debe quedar
            // fuera de las dos tarjetas (~45 px desde su centro) y de los
            // rótulos, que cuelgan ~90 px por debajo de cada tarjeta: si la
            // flecha sube, el rótulo del destino está en el camino; si baja,
            // el del origen.
            const l = Math.hypot(x2 - x1, y2 - y1) || 1
            const sube = y2 < y1 - 20, baja = y2 > y1 + 20
            const minimo = sube ? 115 : 50
            const maximo = baja ? l - 95 : l - 50
            const d = minimo <= maximo ? Math.min(Math.max(70, minimo), maximo) : l / 2
            const mx = x2 - ((x2 - x1) / l) * d, my = y2 - ((y2 - y1) / l) * d
            return <polyline key={i} points={`${x1},${y1} ${mx},${my} ${x2},${y2}`} fill="none"
              style={{ stroke: colores[k] }} strokeWidth="3" markerMid={tipo === 'mpi' ? undefined : `url(#punta-${k})`} />
          })}
        </svg>
        {tipo === 'central' && (
          <div className="topo-pos" style={{ left: `${(centro[0] / W) * 100}%`, top: `${(centro[1] / H) * 100}%` }}>
            <Nodo nodo={{ id: 'Maestro', hardware: maestro?.hardware }} tipo="maestro" />
          </div>
        )}
        {nodos.map((nd, i) => (
          <div key={nd.id} className="topo-pos" style={{ left: `${(pos[i][0] / W) * 100}%`, top: `${(pos[i][1] / H) * 100}%` }}>
            {tarjeta ? tarjeta(nd) : <Nodo nodo={nd} tipo={nd.id === maestro?.id ? 'maestro' : 'nodo'} />}
          </div>
        ))}
      </div>
      {children}
    </section>
  )
}

// Pantalla de la predicción distribuida (Fase 5): cada nodo predice todas
// las horas de sus estaciones con el modelo entrenado y le envía los
// resultados al maestro (gather), que regenera el mapa y la línea de tiempo.
function Prediccion({ inst, onVerFinal }) {
  const p = inst.prediccion
  const sala = new Map((inst.sala ?? []).map((n) => [n.id, n]))
  const fmtN = (v) => (v ?? 0).toLocaleString('es-PE')
  const fmtMB = (v) => `${(v ?? 0).toLocaleString('es-PE', { maximumFractionDigits: 1 })} MB`
  const fmtMs = (v) => (v > 0 ? `${(v / 1000).toLocaleString('es-PE', { maximumFractionDigits: 2 })} s` : '—')
  return (
    <>
      <Topologia
        corrida={{ nodos: p.nodos, comunicacion: p.comunicacion === 'mpi' ? 'mpi' : 'central' }}
        sala={sala}
        maestro={sala.get(inst.nodo_maestro)}
        rotulo="PREDICCIÓN DISTRIBUIDA"
        subtitulo={p.comunicacion === 'mpi' ? 'Las predicciones se juntan con MPI (Gatherv)' : 'Cada nodo envía sus predicciones al maestro'}
      />
      <section className="flujo-limpieza" aria-label="Predicción distribuida">
        <div className="tabla-limpieza-envoltura">
          <table className="tabla-limpieza">
            <thead>
              <tr>
                <th scope="col">Nodo</th>
                <th scope="col">Filas predichas</th>
                <th scope="col">Enviado al maestro</th>
                <th scope="col">Cálculo</th>
                <th scope="col">Hasta el último trozo</th>
              </tr>
            </thead>
            <tbody>
              {p.nodos.map((n) => {
                const vista = sala.get(n.id) ?? { id: n.id, hardware: n.hardware }
                const avance = n.total > 0 ? n.filas / n.total : n.estado === 'terminada' ? 1 : 0
                return (
                  <tr key={n.id}>
                    <th scope="row"><Nodo nodo={vista} tipo={n.id === inst.nodo_maestro ? 'maestro' : 'trabajador'} pequeno /></th>
                    <td>
                      <div className="progreso"><div className={`progreso-fill ${n.estado === 'error' ? 'abortado' : ''}`} style={{ width: `${100 * avance}%` }} /></div>
                      <small className="detalle">{fmtN(n.filas)}{n.total ? ` de ${fmtN(n.total)}` : ''}</small>
                      {n.mensaje && <small className="nodo-nota"> {n.mensaje}</small>}
                    </td>
                    <td className="celda-tiempo">{fmtMB(n.mb)}</td>
                    <td className="celda-tiempo">{fmtMs(n.t_calculo_ms)}{n.motor && <small className="cpu-media">{textoMotor(n.motor)}{n.parte_gpu > 0 ? ` · GPU ${Math.round(n.parte_gpu * 100)} %` : ''}</small>}</td>
                    <td className="celda-tiempo">{fmtMs(n.t_total_ms)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <footer className="limpieza-pie">
          <p className="detalle">
            {p.estado === 'prediciendo' && 'Prediciendo…'}
            {p.estado === 'terminada' && `Listo en ${p.t_total_s.toLocaleString('es-PE', { maximumFractionDigits: 2 })} s: el mapa (Alerta del modelo) y la línea de tiempo ya usan este modelo.`}
            {p.estado === 'error' && p.mensaje}
          </p>
          {p.estado === 'terminada' && onVerFinal && (
            <div className="sala-botones">
              <button type="button" className="boton-grande" onClick={onVerFinal}>Ver resultados finales</button>
            </div>
          )}
        </footer>
      </section>
    </>
  )
}

// ---------- 3. Limpieza ----------

// Gráfica: % de CPU (eje y) a lo largo del avance (eje x): la limpieza de
// datos ocupa la mitad izquierda y la ingeniería de features la derecha,
// con una línea punteada entre las dos. Todas miden lo mismo de ancho,
// tarde lo que tarde cada nodo. Debajo de cada mitad, lo de su fase
// (tiempo o avance), alineado con los títulos de la tabla.
function GraficaCPU({ serie, estado, izquierda, derecha }) {
  const puntos = (serie ?? []).filter((p) => p[0] > 0 || p[1] > 0)
  const camino = puntos.map(([x, y]) => `${(x * 100).toFixed(2)},${(40 - (Math.min(y, 100) / 100) * 38).toFixed(2)}`).join(' ')
  return (
    <div className="grafica-cpu">
      <svg viewBox="0 0 100 40" preserveAspectRatio="none" role="img" aria-label={`Uso del CPU: limpieza ${izquierda}, features ${derecha}`}>
        <line x1="0" y1="2" x2="100" y2="2" className="guia" />
        <line x1="0" y1="21" x2="100" y2="21" className="guia" />
        <line x1="0" y1="40" x2="100" y2="40" className="base" />
        <line x1="50" y1="0" x2="50" y2="40" className="division" vectorEffect="non-scaling-stroke" />
        {camino && <polyline points={camino} className={`linea ${estado}`} vectorEffect="non-scaling-stroke" />}
      </svg>
      <span className="grafica-pie mitades">
        <span>{izquierda}</span>
        <span>{derecha}</span>
      </span>
    </div>
  )
}

// Las dos fases de una tecnología en una sola serie: el avance de la
// limpieza va de 0 a 0,5 y el de la ingeniería de features de 0,5 a 1.
function serieCompleta(t) {
  return [
    ...(t.serie ?? []).map(([x, y]) => [x / 2, y]),
    ...(t.serie_variables ?? []).map(([x, y]) => [0.5 + x / 2, y]),
  ]
}

const pct = (v) => `${Math.round((v ?? 0) * 100)} %`

function CeldaTec({ t }) {
  if (!t) return <td colSpan={2} />
  const enFeatures = t.fase === 'variables'
  const terminada = t.estado === 'terminada'
  // Lo de cada mitad: tiempo si la fase terminó, avance si está en curso.
  const limpiezaLista = terminada || enFeatures
  const izquierda = limpiezaLista ? fmtS(t.t_limpieza_s) : t.estado === 'limpiando' ? pct(t.avance) : '—'
  const derecha = terminada ? fmtS(t.t_variables_s) : enFeatures && t.estado === 'limpiando' ? pct(t.avance_variables) : '—'
  const serie = serieCompleta(t)
  const cpu = serie.length ? serie.reduce((a, p) => a + p[1], 0) / serie.length : 0
  let total
  if (terminada) total = fmtS(t.t_total_s || (t.t_limpieza_s ?? 0) + (t.t_variables_s ?? 0))
  else if (t.estado === 'limpiando') total = pct(enFeatures ? 0.5 + (t.avance_variables ?? 0) / 2 : (t.avance ?? 0) / 2)
  else total = { pendiente: 'en espera', error: 'error', cancelada: 'parada' }[t.estado] ?? t.estado
  return (
    <>
      <td className="celda-grafica"><GraficaCPU serie={serie} estado={t.estado} izquierda={izquierda} derecha={derecha} /></td>
      <td className={`celda-tiempo estado-${t.estado}`} title={t.mensaje || (t.huella ? `huella ${corta(t.huella)}` : undefined)}>
        {total}
        {t.estado === 'error' ? <small>{t.mensaje}</small> : serie.length > 0 && <small className="cpu-media">CPU media {cpu.toFixed(0)} %</small>}
      </td>
    </>
  )
}

// Pantalla 3: limpieza de datos e ingeniería de features (escritura de
// muestra.bin) en una sola tabla; cada tecnología, una gráfica con las dos
// fases seguidas.
function Limpieza({ inst, esMaestro, vivo, onSiguiente }) {
  const l = inst.limpieza
  const [preguntar, setPreguntar] = useState(false)
  const [error, setError] = useState('')
  const enCurso = vivo && l.estado === 'limpiando'
  const enLimpieza = new Set(l.nodos.map((n) => n.id))
  const nuevos = vivo ? (inst.sala ?? []).filter((n) => !enLimpieza.has(n.id)) : []
  const sala = new Map((inst.sala ?? []).map((n) => [n.id, n]))
  const huellas = new Set(l.nodos.flatMap((n) => n.tecnologias.filter((t) => t.huella).map((t) => t.huella)))

  const accion = async (f) => {
    setError('')
    try {
      await f()
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <section className="flujo-limpieza" aria-label="Limpieza de datos e ingeniería de features">
      <header className="limpieza-cabecera">
        <span className="rotulo">CLÚSTER INICIADO</span>
        <span className="rotulo titulo">LIMPIEZA Y FEATURES{l.estado === 'cancelada' ? ' · PARADA' : ''}</span>
        <div className="nuevos" aria-label="Nodos que se unieron después">
          {nuevos.map((n) => (
            <Nodo key={n.id} nodo={n} tipo="nuevo" pequeno onClick={esMaestro && enCurso ? () => accion(() => apiCluster.limpiar([n.id])) : undefined} />
          ))}
          {nuevos.length > 0 && esMaestro && enCurso && <small>tócalos para sumarlos</small>}
        </div>
      </header>

      <div className="tabla-limpieza-envoltura">
        <table className="tabla-limpieza">
          <thead>
            <tr>
              <th scope="col" rowSpan={2} className="cabeza-doble">Nodo</th>
              <th scope="col" rowSpan={2} className="cabeza-doble"># hilos</th>
              {l.tecnologias.map((t) => (
                <th key={t} scope="colgroup" colSpan={2} className="cabeza-tec">{NOMBRE_TEC[t] ?? t} <small>% Uso del CPU</small></th>
              ))}
            </tr>
            <tr>
              {l.tecnologias.map((t) => (
                <Fragment key={t}>
                  <th scope="col" className="cabeza-fases"><span>Limpieza de datos</span><span>Ingeniería de features</span></th>
                  <th scope="col">Total</th>
                </Fragment>
              ))}
            </tr>
          </thead>
          <tbody>
            {l.nodos.map((n) => {
              const vista = sala.get(n.id) ?? { id: n.id, hardware: n.hardware }
              const hilos = Math.max(0, ...n.tecnologias.map((t) => t.hilos || 0))
              return (
                <tr key={n.id}>
                  <th scope="row"><Nodo nodo={vista} tipo={n.id === inst.nodo_maestro ? 'maestro' : 'trabajador'} pequeno /></th>
                  <td className="celda-hilos">{hilos || '—'}</td>
                  {l.tecnologias.map((t) => <CeldaTec key={t} t={n.tecnologias.find((x) => x.tecnologia === t)} />)}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      <footer className="limpieza-pie">
        {/* Solo se avisa si algo salió mal: muestras con huellas distintas. */}
        {!enCurso && huellas.size > 1 ? <p className="error">Los nodos generaron {huellas.size} muestras distintas (huellas diferentes).</p> : <span />}
        {esMaestro && vivo && (
          <div className="sala-botones">
            {enCurso
              ? <button type="button" className="boton" onClick={() => setPreguntar(true)}>Parar limpieza</button>
              : <button type="button" className="boton-grande" onClick={onSiguiente}>Siguiente: entrenamiento</button>}
          </div>
        )}
        {error && <p className="error" role="alert">{error}</p>}
      </footer>

      {preguntar && (
        <div className="modal-fondo" role="presentation">
          <div className="modal" role="dialog" aria-modal="true" aria-labelledby="titulo-parar">
            <h3 id="titulo-parar">¿Quieres guardar los resultados completados?</h3>
            <p className="detalle">Se guardan los tiempos de las tecnologías que ya terminaron (resultados/limpieza.csv).</p>
            <div className="sala-botones">
              <button type="button" className="boton" onClick={() => { setPreguntar(false); accion(() => apiCluster.pararLimpieza(true)) }}>Sí</button>
              <button type="button" className="boton" onClick={() => { setPreguntar(false); accion(() => apiCluster.pararLimpieza(false)) }}>No</button>
            </div>
          </div>
        </div>
      )}
    </section>
  )
}

// Sin bin/saca (p. ej. la página abierta con `npm run dev` sin el
// lanzador): no hay quién inicie el maestro o el nodo en esta laptop.
function SinLanzador() {
  return (
    <section className="flujo-inicio" aria-label="Falta el lanzador">
      <div className="inicio-opcion" style={{ gridColumn: '1 / -1' }}>
        <h3>Falta el lanzador de SACA en esta laptop</h3>
        <p className="detalle">
          Las pantallas del clúster necesitan el lanzador (<code>bin/saca</code>): es el que inicia el maestro o el nodo en esta
          laptop. Desde la carpeta del proyecto, corre:
        </p>
        <pre className="comando">bash scripts/saca.sh{'\n'}powershell -ExecutionPolicy Bypass -File scripts\saca.ps1   (Windows)</pre>
        <p className="detalle">
          y usa la página que abre, <a href="http://127.0.0.1:8090">http://127.0.0.1:8090</a>. Con <code>npm run dev</code> también funciona
          mientras el lanzador esté corriendo (reenvía <code>/local</code> y <code>/api</code> al puerto 8090).
        </p>
      </div>
    </section>
  )
}

// ---------- flujo ----------

const ESTADO_FASE = {
  limpieza: { limpiando: 'curso', terminada: 'hecho', cancelada: 'parada', error: 'parada' },
  entrenamiento: { entrenando: 'curso', verificando: 'curso', terminado: 'hecho', cancelado: 'parada', error: 'parada' },
  prediccion: { prediciendo: 'curso', terminada: 'hecho', error: 'parada' },
}
const NOMBRE_CSV = { limpieza: 'limpieza', entrenamiento: 'entrenamiento', prediccion: 'prediccion' }
const ARMAR_CSV = { limpieza: csvLimpieza, entrenamiento: csvEntrenamiento, prediccion: csvPrediccion }

// Flujo del clúster. Arriba: Nuevo (los paneles para iniciar cada fase;
// las laptops que no son el maestro los ven sin botones) o Resultados (la
// fase en vivo o un proceso del historial, con su CSV), y la línea de fases.
// Cuando el maestro inicia algo, todas las laptops pasan a sus Resultados.
export default function FlujoCluster({ lanzador, onVerFinal }) {
  const { datos: local, actualizar } = lanzador
  const rol = local?.rol ?? ''
  const { conectado, inst, perdido } = useCluster(rol !== '')
  const esMaestro = rol === 'maestro'
  const [modo, setModo] = useState('nuevo') // 'nuevo' | 'resultados'
  const [fase, setFase] = useState('limpieza') // 'limpieza' | 'entrenamiento' | 'prediccion'
  // La limpieza se saltó en esta sesión (Saltar limpieza).
  const [omitida, setOmitida] = useState(false)
  // Proceso del historial que se está viendo: { fase, id, datos, descripcion, fecha }.
  const [historico, setHistorico] = useState(null)
  // Último proceso en curso visto: al aparecer uno nuevo, se pasa a sus Resultados.
  const [ultimoEnCurso, setUltimoEnCurso] = useState('')
  // Nodos que el maestro quitó de la selección (en gris); los demás de la
  // sala, incluidos los que llegan, entran elegidos.
  const [quitados, setQuitados] = useState(new Set())
  // Modelo a entrenar (Nuevo → Entrenamiento).
  const [ajustes, setAjustes] = useState({ modelo: 'red', epocas: 20, pasos: 25, reparto: 'dinamico', comunicacion: 'arbol', motores: 'comparar', cuda: true, motorPrediccion: 'openmp', gather: 'tcp' })
  const [ocupado, setOcupado] = useState('')
  // Error y avisos de una acción, atados a la pantalla en que ocurrieron:
  // al cambiar de pantalla dejan de mostrarse.
  const [mensaje, setMensaje] = useState({ pantalla: '', error: '', avisos: [] })

  const seleccion = useMemo(() => new Set((inst?.sala ?? []).map((n) => n.id).filter((id) => !quitados.has(id))), [inst?.sala, quitados])
  const setSeleccion = (f) =>
    setQuitados(() => {
      const elegidos = f(seleccion)
      return new Set((inst?.sala ?? []).map((n) => n.id).filter((id) => !elegidos.has(id)))
    })

  if (!local && lanzador.disponible === false) return <SinLanzador />
  if (!local) return <p className="detalle">Conectando con el lanzador local…</p>
  if (!rol) return <Inicio local={local} actualizar={actualizar} />
  if (perdido && !conectado) {
    return (
      <section className="flujo-aviso" role="status">
        <h3>Se ha perdido conexión con el maestro…</h3>
        <p className="detalle">Esperando a que vuelva. {rol === 'nodo' ? 'Este nodo se reconecta solo.' : ''}</p>
      </section>
    )
  }
  if (!inst) return <section className="flujo-aviso" role="status"><p className="detalle">Conectando con el maestro…</p></section>

  // Lo actual de cada fase (en vivo): la última limpieza, corrida y predicción.
  const actual = { limpieza: inst.limpieza, entrenamiento: inst.corrida, prediccion: inst.prediccion }
  const estadoDe = (f) => ESTADO_FASE[f][actual[f]?.estado]
  const enCurso = ['limpieza', 'entrenamiento', 'prediccion'].find((f) => estadoDe(f) === 'curso')
  const claveEnCurso = enCurso ? `${enCurso}:${actual[enCurso].id}` : ''
  if (claveEnCurso && claveEnCurso !== ultimoEnCurso) {
    // Empezó un proceso: todas las laptops pasan a sus resultados en vivo.
    setUltimoEnCurso(claveEnCurso)
    setModo('resultados')
    setFase(enCurso)
    setHistorico(null)
  }
  const estados = {
    limpieza: estadoDe('limpieza') ?? (omitida ? 'omitida' : 'pendiente'),
    entrenamiento: estadoDe('entrenamiento') ?? 'pendiente',
    prediccion: estadoDe('prediccion') ?? 'pendiente',
    final: estadoDe('prediccion') === 'hecho' ? 'hecho' : 'pendiente',
  }
  // Cambia cuando termina algo: el historial se vuelve a pedir.
  const recargarHistorial = ['limpieza', 'entrenamiento', 'prediccion'].map((f) => `${actual[f]?.id}:${actual[f]?.estado}`).join('|')

  const pantalla = `${modo}:${fase}`
  const error = mensaje.pantalla === pantalla ? mensaje.error : ''
  const avisos = mensaje.pantalla === pantalla ? mensaje.avisos : []
  const ir = (m, f) => {
    setModo(m)
    setFase(f)
    setHistorico(null)
  }

  const accion = async (nombre, f) => {
    setOcupado(nombre)
    setMensaje({ pantalla: '', error: '', avisos: [] })
    try {
      await f()
    } catch (e) {
      setMensaje({ pantalla, error: e.message, avisos: [] })
    } finally {
      setOcupado('')
    }
  }

  // Comprueba huellas; devuelve los nodos (de ids) cuya muestra no es la del maestro.
  const distintos = async (ids) => {
    const e = await apiCluster.verificarHuellas()
    if (!e.huella) throw new Error('El maestro no tiene muestra.bin: realiza la limpieza (con el nodo del maestro incluido).')
    return (e.sala ?? []).filter((n) => ids.includes(n.id) && n.huella !== e.huella)
  }
  const describir = (lista) => lista.map((n) => `${n.id} (${n.huella ? `otra: ${corta(n.huella)}` : 'sin muestra'})`).join(', ')

  const onLimpiar = () => accion('limpiar', async () => {
    await apiCluster.limpiar((inst.sala ?? []).map((n) => n.id))
    setOmitida(false)
  })
  const onSaltar = () => accion('saltar', async () => {
    const malos = await distintos((inst.sala ?? []).map((n) => n.id))
    setOmitida(true)
    ir('nuevo', 'entrenamiento')
    if (malos.length) setMensaje({ pantalla: 'nuevo:entrenamiento', error: '', avisos: [`No todos tienen el mismo muestra.bin que el maestro: ${describir(malos)}. Puedes continuar sin ellos o realizar la limpieza.`] })
  })
  const onEntrenar = () => accion('entrenar', async () => {
    const ids = (inst.sala ?? []).map((n) => n.id).filter((id) => seleccion.has(id))
    const malos = await distintos(ids)
    if (malos.length) {
      setQuitados((q) => new Set([...q, ...malos.map((m) => m.id)]))
      throw new Error(`Se quitaron de la selección porque su muestra.bin no es la del maestro: ${describir(malos)}.`)
    }
    // Fase 3: reparto según la capacidad medida de cada nodo, si el maestro
    // la conoce para todos los elegidos; si no, partes iguales. El dinámico
    // (solo la red) empieza con la capacidad medida, si la hay.
    const sug = inst.pesos_sugeridos ?? {}
    const dinamico = ajustes.reparto === 'dinamico' && ajustes.modelo === 'red'
    const proporcional = (ajustes.reparto === 'capacidad' || dinamico) && ids.every((id) => sug[id] > 0)
    const pesos = proporcional ? Object.fromEntries(ids.map((id) => [id, sug[id]])) : undefined
    const ef = efectivo(ajustes, (inst.sala ?? []).filter((n) => ids.includes(n.id)))
    await apiCluster.entrenar(ids, ajustes.epocas, true, {
      modelo: ajustes.modelo, pasos: ajustes.pasos, pesos, dinamico, motores: ef.motores, cuda: ef.cuda,
      comunicacion: ajustes.modelo === 'red' ? ef.comunicacion : undefined,
    })
  })
  const onPredecir = (nodos) => accion('predecir', () => apiCluster.predecir(nodos.map((n) => n.id), {
    motor: hayOpenMP(nodos) ? ajustes.motorPrediccion : 'go', cuda: efectivo(ajustes, nodos).cuda,
    comunicacion: ajustes.gather === 'mpi' && hayMPI(nodos) ? 'mpi' : 'tcp',
  }))

  const onFase = (f) => {
    if (f === 'final') onVerFinal()
    else {
      setFase(f)
      setHistorico(null)
    }
  }
  const elegirHistorico = (id) => accion('historial', async () => {
    const e = await apiCluster.historialDe(id)
    setHistorico({ fase, id, datos: e.datos, descripcion: e.descripcion, fecha: e.fecha })
  })

  // ---------- Nuevo ----------
  let vista
  if (modo === 'nuevo') {
    const sala = (m) => (
      <Sala
        inst={inst} local={local} esMaestro={esMaestro} modo={m}
        seleccion={seleccion} setSeleccion={setSeleccion}
        onLimpiar={onLimpiar} onSaltar={onSaltar} onEntrenar={onEntrenar}
        onRealizarLimpieza={() => ir('nuevo', 'limpieza')}
        ajustes={ajustes} setAjustes={setAjustes}
        ocupado={ocupado} error={error} avisos={avisos}
      />
    )
    if (fase === 'prediccion') vista = <NuevoPrediccion inst={inst} esMaestro={esMaestro} onPredecir={onPredecir} ocupado={ocupado} error={error} ajustes={ajustes} setAjustes={setAjustes} />
    else if (fase === 'entrenamiento' && esMaestro) vista = sala('pre')
    else vista = sala('sala')
    if (!esMaestro) {
      vista = (
        <>
          <p className="detalle aviso-nodo">El maestro elige qué hacer en cada fase; cuando empiece, aquí se verán los resultados en vivo.</p>
          {vista}
        </>
      )
    }
  } else {
    // ---------- Resultados ----------
    const viendoHistorico = historico?.fase === fase
    const datosFase = viendoHistorico ? historico.datos : actual[fase]
    const vivo = !viendoHistorico
    let contenido
    if (!datosFase) {
      contenido = <p className="detalle sin-datos">Todavía no hay {{ limpieza: 'una limpieza', entrenamiento: 'un entrenamiento', prediccion: 'una predicción distribuida' }[fase]} en esta sesión. Inícialo en <b>Nuevo</b> o elige uno del historial.</p>
    } else if (fase === 'limpieza') {
      contenido = <Limpieza key={`l${datosFase.id}${vivo}`} inst={{ ...inst, limpieza: datosFase }} esMaestro={esMaestro} vivo={vivo} onSiguiente={() => ir('nuevo', 'entrenamiento')} />
    } else if (fase === 'entrenamiento') {
      contenido = <ResultadoEntrenamiento corrida={datosFase} inst={inst} esMaestro={esMaestro} vivo={vivo} onSiguiente={() => ir('nuevo', 'prediccion')} />
    } else {
      contenido = <Prediccion inst={{ ...inst, prediccion: datosFase }} onVerFinal={onVerFinal} />
    }
    // AAAAMMDDHHMM del proceso (o de ahora, si es el actual).
    const fechaCSV = () => new Date(viendoHistorico ? historico.fecha : Date.now()).toISOString().slice(0, 16).replace(/[-:T]/g, '')
    vista = (
      <>
        <div className="resultados-barra">
          <p className="detalle">
            {viendoHistorico
              ? <>Viendo del historial: <b>{historico.descripcion}</b> ({new Date(historico.fecha).toLocaleString('es-PE')})</>
              : datosFase ? 'En vivo: el proceso actual de esta sesión.' : ''}
          </p>
          <div className="sala-botones sala-botones-fila">
            {datosFase && (
              <button type="button" className="boton" onClick={() => descargarCSV(`saca_${NOMBRE_CSV[fase]}_${fechaCSV()}.csv`, ARMAR_CSV[fase](datosFase))}>
                Descargar CSV
              </button>
            )}
            {viendoHistorico && <button type="button" className="boton" onClick={() => setHistorico(null)}>Volver a lo actual</button>}
          </div>
        </div>
        {contenido}
        {error && <p className="error" role="alert">{error}</p>}
        <Historial tipo={fase} recargar={recargarHistorial} elegido={viendoHistorico ? historico.id : ''} onElegir={elegirHistorico} />
      </>
    )
  }

  return (
    <>
      <Fases modo={modo} setModo={(m) => { setModo(m); setHistorico(null) }} fase={fase} estados={estados} onFase={onFase} />
      {vista}
    </>
  )
}
