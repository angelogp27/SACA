// Flujo del clúster antes de entrenar (las 4 pantallas):
//   1. Inicio: Iniciar clúster (esta laptop es el maestro) o Unirse.
//   2. Sala: los nodos conectados; Iniciar limpieza o Saltar limpieza.
//   3. Limpieza: cada nodo limpia sus CSV con Pthreads y luego con Go; se
//      ve el % de CPU de cada uno a lo largo de su avance y su tiempo.
//   4. Antes de entrenar: elegir nodos y comprobar que todos tienen el
//      mismo muestra.bin (misma huella) que el maestro.
// Con el entrenamiento en curso se muestra el panel del clúster en vivo.
// Solo la laptop del maestro tiene los botones; las demás miran.
import { useMemo, useState } from 'react'
import { apiCluster, useCluster } from '../../data/cluster'
import { apiLocal } from '../../data/local'
import PanelCluster from '../PanelCluster'
import Protector from '../Protector'
import logoIntel from '../../assets/marcas/Intel.png'
import logoNvidia from '../../assets/marcas/NVIDIA.png'
import logoRyzen from '../../assets/marcas/Ryzen.svg'
import './FlujoCluster.css'

const corta = (h) => (h ? h.slice(0, 12) : '')
const fmtS = (s) => (Number.isFinite(s) && s > 0 ? `${s.toLocaleString('es-PE', { maximumFractionDigits: 1 })} s` : '—')
const NOMBRE_TEC = { pthreads: 'Pthreads (C)', go: 'Go' }

// ip del nodo: la de su conexión (el nodo local del maestro llega por 127.0.0.1).
function ipDe(nodo) {
  const ip = (nodo.remoto ?? '').replace(/:\d+$/, '').replace(/^\[|\]$/g, '')
  return ip === '127.0.0.1' || ip === '::1' ? 'misma laptop que el maestro' : ip
}

// En Windows el procesador llega como "... AuthenticAMD" o "... GenuineIntel".
function marcaCPU(cpu = '') {
  if (/ryzen|amd/i.test(cpu)) return { clase: 'amd', logo: logoRyzen, nombre: /ryzen/i.test(cpu) ? 'AMD Ryzen' : 'AMD' }
  if (/intel/i.test(cpu)) return { clase: 'intel', logo: logoIntel, nombre: 'Intel' }
  return null
}

function Nodo({ nodo, tipo, nota, onClick, pequeno }) {
  const hw = nodo?.hardware ?? {}
  const cpu = marcaCPU(hw.cpu)
  const Etiqueta = onClick ? 'button' : 'div'
  return (
    <div className={`nodo ${pequeno ? 'pequeno' : ''}`}>
      <Etiqueta
        type={onClick ? 'button' : undefined}
        className={`nodo-caja ${tipo}`}
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
          {!pequeno && <small>{nota ?? ipDe(nodo)}</small>}
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

function Sala({ inst, local, esMaestro, modo, seleccion, setSeleccion, onLimpiar, onSaltar, onEntrenar, onRealizarLimpieza, ocupado, error, avisos }) {
  const sala = inst?.sala ?? []
  const maestroNodo = sala.find((n) => n.id === inst?.nodo_maestro)
  const otros = sala.filter((n) => n !== maestroNodo)
  // Máximo 8 laptops (inst.capacidad: 8 si el maestro participa como nodo,
  // 7 si no, porque su laptop igual cuenta).
  const libres = Math.max(0, (inst?.capacidad ?? 8) - sala.length)
  const enPre = modo === 'pre'

  const tipo = (n, base) => {
    if (enPre && !seleccion.has(n.id)) return 'deseleccionado'
    if (enPre && n.huella !== inst.huella) return 'distinto'
    return base
  }
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
  const nota = (n) => {
    if (enPre && n.huella !== inst.huella) return n.huella ? `otra muestra (${corta(n.huella)})` : 'sin muestra'
    return undefined
  }

  return (
    <section className="flujo-sala" aria-label={enPre ? 'Antes de entrenar' : 'Clúster iniciado'}>
      <div className="sala-centro">
        <span className="rotulo">CLÚSTER INICIADO</span>
        {maestroNodo ? (
          <Nodo nodo={maestroNodo} tipo={tipo(maestroNodo, 'maestro')} nota={nota(maestroNodo) ?? 'maestro'} onClick={alternar(maestroNodo)} />
        ) : (
          <div className="nodo"><div className="nodo-caja maestro" /><span className="nodo-nombre">{local.id}<small>maestro (no participa)</small></span></div>
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
          </div>
        )}
        {!esMaestro && <p className="detalle">El maestro ({local.maestro}) controla el clúster; aquí se ve en vivo.</p>}
      </div>

      <div className="sala-nodos">
        {otros.map((n) => (
          <Nodo key={n.id} nodo={n} tipo={tipo(n, 'trabajador')} nota={nota(n)} onClick={alternar(n)} />
        ))}
        {Array.from({ length: libres }, (_, i) => <Nodo key={`libre${i}`} tipo="libre" />)}
      </div>

      <div className="sala-pie">
        {enPre && <p className="detalle">Toca un nodo para quitarlo o volver a elegirlo (amarillo = no participa). Muestra del maestro: <code>{corta(inst?.huella) || 'ninguna'}</code>.</p>}
        {avisos?.map((a) => <p key={a} className="aviso">{a}</p>)}
        {error && <p className="error" role="alert">{error}</p>}
      </div>
    </section>
  )
}

// ---------- 3. Limpieza ----------

// Gráfica: % de CPU (eje y) a lo largo del avance 0-100 % (eje x): todas
// las líneas miden lo mismo de ancho, tarde lo que tarde cada nodo.
function GraficaCPU({ serie, estado }) {
  const puntos = (serie ?? []).filter((p) => p[0] > 0 || p[1] > 0)
  const camino = puntos.map(([x, y]) => `${(x * 100).toFixed(2)},${(40 - (Math.min(y, 100) / 100) * 38).toFixed(2)}`).join(' ')
  const promedio = puntos.length ? puntos.reduce((a, p) => a + p[1], 0) / puntos.length : 0
  return (
    <div className="grafica-cpu">
      <svg viewBox="0 0 100 40" preserveAspectRatio="none" role="img" aria-label={`CPU promedio ${promedio.toFixed(0)} %`}>
        <line x1="0" y1="2" x2="100" y2="2" className="guia" />
        <line x1="0" y1="21" x2="100" y2="21" className="guia" />
        <line x1="0" y1="40" x2="100" y2="40" className="base" />
        {camino && <polyline points={camino} className={`linea ${estado}`} vectorEffect="non-scaling-stroke" />}
      </svg>
      <span className="grafica-pie">{estado === 'pendiente' ? 'en espera' : `CPU media ${promedio.toFixed(0)} %`}</span>
    </div>
  )
}

function CeldaTec({ t }) {
  if (!t) return <td />
  const texto = {
    pendiente: 'en espera',
    limpiando: `${Math.round(t.avance * 100)} %`,
    terminada: fmtS(t.t_total_s),
    error: 'error',
    cancelada: 'parada',
  }[t.estado]
  return (
    <>
      <td className="celda-grafica"><GraficaCPU serie={t.serie} estado={t.estado} /></td>
      <td className={`celda-tiempo estado-${t.estado}`} title={t.mensaje || (t.huella ? `huella ${corta(t.huella)}` : undefined)}>
        {texto}
        {t.estado === 'error' && <small>{t.mensaje}</small>}
      </td>
    </>
  )
}

function Limpieza({ inst, esMaestro, onContinuar }) {
  const l = inst.limpieza
  const [preguntar, setPreguntar] = useState(false)
  const [error, setError] = useState('')
  const enCurso = l.estado === 'limpiando'
  const enLimpieza = new Set(l.nodos.map((n) => n.id))
  const nuevos = (inst.sala ?? []).filter((n) => !enLimpieza.has(n.id))
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
    <section className="flujo-limpieza" aria-label="Limpieza">
      <header className="limpieza-cabecera">
        <span className="rotulo">CLÚSTER INICIADO</span>
        <span className="rotulo titulo">LIMPIEZA {l.estado === 'limpiando' ? '' : `· ${l.estado}`}</span>
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
              <th scope="col">Nodo</th>
              <th scope="col"># hilos</th>
              {l.tecnologias.map((t) => (
                <th key={t} scope="col" colSpan={2}>{NOMBRE_TEC[t] ?? t} <small>% Uso del CPU</small></th>
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
        {huellas.size > 1 ? <p className="error">Los nodos generaron {huellas.size} muestras distintas (huellas diferentes).</p> : <span />}
        {esMaestro && (
          <div className="sala-botones">
            <button type="button" className="boton-grande" disabled={enCurso} onClick={onContinuar}>Continuar</button>
            {enCurso && <button type="button" className="boton" onClick={() => setPreguntar(true)}>Parar limpieza</button>}
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

export default function FlujoCluster({ datos, lanzador }) {
  const { datos: local, actualizar } = lanzador
  const rol = local?.rol ?? ''
  const { conectado, inst, perdido } = useCluster(rol !== '')
  const esMaestro = rol === 'maestro'
  const [paso, setPaso] = useState('sala') // 'sala' | 'pre' | 'entrenamiento'
  // Nodos que el maestro quitó de la selección (amarillos); los demás de la
  // sala, incluidos los que llegan, entran elegidos.
  const [quitados, setQuitados] = useState(new Set())
  // Limpieza ya vista (botón Continuar): mientras no, se sigue mostrando.
  const [continuada, setContinuada] = useState(0)
  const [ocupado, setOcupado] = useState('')
  const [error, setError] = useState('')
  const [avisos, setAvisos] = useState([])

  const limpieza = inst?.limpieza
  const corrida = inst?.corrida
  const entrenando = corrida && (corrida.estado === 'entrenando' || corrida.estado === 'verificando')
  const seleccion = useMemo(() => new Set((inst?.sala ?? []).map((n) => n.id).filter((id) => !quitados.has(id))), [inst?.sala, quitados])
  const setSeleccion = (f) =>
    setQuitados(() => {
      const elegidos = f(seleccion)
      return new Set((inst?.sala ?? []).map((n) => n.id).filter((id) => !elegidos.has(id)))
    })

  if (!local && lanzador.disponible === false) return <SinLanzador />
  if (!local) return <p className="detalle">Conectando con el lanzador local…</p>

  // El rol de esta laptop y el botón para salir van arriba a la izquierda (App).
  const salir = null

  if (!rol) return <>{salir}<Inicio local={local} actualizar={actualizar} /></>
  if (perdido && !conectado) {
    return (
      <section className="flujo-aviso" role="status">
        <h3>Se ha perdido conexión con el maestro…</h3>
        <p className="detalle">Esperando a que vuelva. {rol === 'nodo' ? 'Este nodo se reconecta solo.' : ''}</p>
      </section>
    )
  }
  if (!inst) return <section className="flujo-aviso" role="status"><p className="detalle">Conectando con el maestro…</p></section>

  const accion = async (nombre, f) => {
    setOcupado(nombre)
    setError('')
    setAvisos([])
    try {
      await f()
    } catch (e) {
      setError(e.message)
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
  })
  const onSaltar = () => accion('saltar', async () => {
    const malos = await distintos((inst.sala ?? []).map((n) => n.id))
    if (malos.length) setAvisos([`No todos tienen el mismo muestra.bin que el maestro: ${describir(malos)}. Puedes continuar sin ellos o realizar la limpieza.`])
    setPaso('pre')
  })
  const onEntrenar = () => accion('entrenar', async () => {
    const ids = (inst.sala ?? []).map((n) => n.id).filter((id) => seleccion.has(id))
    const malos = await distintos(ids)
    if (malos.length) {
      setQuitados((q) => new Set([...q, ...malos.map((m) => m.id)]))
      throw new Error(`Se quitaron de la selección porque su muestra.bin no es la del maestro: ${describir(malos)}.`)
    }
    const config = await fetch('/api/config').then((r) => r.json()).catch(() => ({ epocas: 200, verificar: true }))
    await apiCluster.entrenar(ids, config.epocas ?? 200, config.verificar ?? true)
    setPaso('entrenamiento')
  })

  // Un entrenamiento en curso siempre se muestra; una limpieza también,
  // hasta que el maestro aprieta Continuar.
  // (Las laptops que solo miran no tienen Continuar: ven la limpieza solo mientras corre.)
  const viendoLimpieza = limpieza && (limpieza.estado === 'limpiando' || (esMaestro && limpieza.id !== continuada))
  const pasoActual = entrenando ? 'entrenamiento' : viendoLimpieza ? 'limpieza' : paso

  let vista
  if (pasoActual === 'entrenamiento' && corrida) {
    vista = (
      <>
        <Protector><PanelCluster datos={datos} /></Protector>
        {esMaestro && !entrenando && (
          <div className="sala-botones"><button type="button" className="boton" onClick={() => setPaso('pre')}>Volver a elegir nodos</button></div>
        )}
      </>
    )
  } else if (pasoActual === 'limpieza') {
    vista = <Limpieza inst={inst} esMaestro={esMaestro} onContinuar={() => { setAvisos([]); setContinuada(limpieza.id); setPaso('pre') }} />
  } else {
    vista = (
      <Sala
        inst={inst} local={local} esMaestro={esMaestro} modo={pasoActual === 'pre' ? 'pre' : 'sala'}
        seleccion={seleccion} setSeleccion={setSeleccion}
        onLimpiar={onLimpiar} onSaltar={onSaltar} onEntrenar={onEntrenar}
        onRealizarLimpieza={() => { setAvisos([]); setPaso('sala') }}
        ocupado={ocupado} error={error} avisos={avisos}
      />
    )
  }
  return <>{salir}{vista}</>
}
