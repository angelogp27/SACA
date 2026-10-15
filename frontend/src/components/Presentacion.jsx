// Modo presentación: la exposición del avance en 10 diapositivas (unos 10
// minutos, demo incluida), con los datos reales y el clúster en vivo.
// Teclas: ← → o espacio para navegar, N para las notas del orador, F para
// pantalla completa y T para reiniciar el cronómetro. La diapositiva
// actual queda en la URL (#p3). El guion completo está en
// docs/guion_exposicion.md.
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import {
  calculoPorNodo,
  cifras,
  codigo,
  conceptos,
  diapositivas,
  docentes,
  etiquetaEstado,
  grupo,
  integrantes,
  mensajes,
  niveles,
  proximos,
  teoriaMedido,
  temasCurso,
} from '../data/presentacion'
import { esGPU, useCluster } from '../data/cluster'
import { ArbolReduccion, FlujoColectivas, GraficaSpeedup, ModeloCosto, NivelesParalelismo } from './Diagramas'
import { BarrasH } from './Graficas'
import PanelMapa from './PanelMapa'
import PanelCluster from './PanelCluster'
import ResultadosCluster from './ResultadosCluster'
import Protector from './Protector'
import './Presentacion.css'

export const TOTAL_DIAPOSITIVAS = diapositivas.length

const fmtN = new Intl.NumberFormat('es-PE')
const dosDigitos = (n) => String(n).padStart(2, '0')
const fmt = (v, d = 1) => (Number.isFinite(v) ? v.toLocaleString('es-PE', { minimumFractionDigits: d, maximumFractionDigits: d }) : '—')

function Concepto({ id }) {
  const c = conceptos[id]
  return (
    <aside className="concepto">
      <span className="concepto-rotulo">Concepto del curso</span>
      <h3>{c.titulo}</h3>
      {c.formula && <p className="concepto-formula">{c.formula}</p>}
      <p>{c.definicion}</p>
      <p className="concepto-saca"><strong>En SACA:</strong> {c.enSaca}</p>
    </aside>
  )
}

function Cifra({ valor, texto, tono }) {
  return (
    <div className={`cifra ${tono ?? ''}`}>
      <strong>{valor}</strong>
      <span>{texto}</span>
    </div>
  )
}

// --- Diapositivas -------------------------------------------------------

function Portada({ c }) {
  return (
    <div className="portada">
      <p className="portada-curso">Programación Concurrente y Paralela · Escuela Profesional de Ingeniería de Software · UNMSM · 2026-2</p>
      <div className="cifras">
        <Cifra valor={fmtN.format(c.filas)} texto="mediciones horarias de la EPA (2025)" />
        <Cifra valor="4 PCs" texto="en 4 casas · 2 GPU NVIDIA + 1 CPU" />
        <Cifra valor={`${c.gpu1S}×`} texto="más rápido con una GPU remota que en secuencial" tono="acento" />
      </div>
      <div className="portada-pie">
        <div>
          <span className="concepto-rotulo">Integrantes</span>
          <ul className="integrantes">
            {grupo.map((g) => (
              <li key={g.apellidos}><strong>{g.apellidos}</strong>, {g.nombres}</li>
            ))}
          </ul>
        </div>
        <div className="portada-docentes">
          <span className="concepto-rotulo">Docente</span>
          <ul>
            {docentes.map((d) => <li key={d}>{d}</li>)}
          </ul>
          <span className="concepto-rotulo">Avance del proyecto</span>
          <p>Expone: Piero Carlinho Vargas Chota</p>
        </div>
      </div>
    </div>
  )
}

function Tuberia({ c }) {
  const pasos = [
    { titulo: 'CSV de la EPA', dato: `${fmtN.format(c.filas)} filas`, tecnica: '4 contaminantes, 2025' },
    { titulo: 'Cargador concurrente', dato: `S(${c.cargadorP}) = ${c.cargadorS}`, tecnica: 'worker pool, fan-out / fan-in' },
    { titulo: 'MongoDB', dato: `${fmtN.format(c.registros)} registros`, tecnica: 'escritura por lotes en paralelo' },
    { titulo: 'Muestra etiquetada', dato: '3,3 M ejemplos', tecnica: 'NO₂ en t+1 > 53 ppb' },
    { titulo: 'Entrenamiento distribuido', dato: `S = ${c.gpu1S} con 1 GPU`, tecnica: 'maestro-trabajador + CUDA' },
    { titulo: 'Alerta', dato: `AUC ${c.auc.toFixed(3)}`, tecnica: 'probabilidad por estación y hora' },
  ]
  return (
    <ol className="tuberia">
      {pasos.map((p) => (
        <li key={p.titulo}>
          <span className="tuberia-titulo">{p.titulo}</span>
          <strong>{p.dato}</strong>
          <span className="tuberia-tecnica">{p.tecnica}</span>
        </li>
      ))}
    </ol>
  )
}

function Problema({ c, estaciones, estacion, setElegida }) {
  return (
    <div className="llena">
      <div className="dos-columnas">
        <div className="mapa-pequeno">
          <PanelMapa estaciones={estaciones} seleccionada={estacion?.id} onSeleccionar={setElegida} />
        </div>
        <div className="pila">
          <p className="enunciado">
            ¿El NO₂ de esta estación superará <strong>53 ppb</strong> (umbral AQI 51) en la próxima hora?
          </p>
          <div className="cifras compactas">
            <Cifra valor={fmtN.format(c.estaciones)} texto="estaciones de monitoreo" />
            <Cifra valor={c.auc.toFixed(3)} texto="AUC en meses no vistos (oct-dic)" tono="acento" />
          </div>
          <p className="nota">
            Regresión logística por descenso de gradiente: simple a propósito. El foco del curso es cómo se paraleliza: el
            cargador concurrente bajó la ingesta de {c.cargadorT1} s a {c.cargadorTp} s con agregadoras particionadas
            (fracción serial medida f = {c.cargadorF} en el diseño original).
          </p>
        </div>
      </div>
      <Tuberia c={c} />
    </div>
  )
}

function Arquitectura() {
  return (
    <div className="llena">
      <div className="dos-columnas ancha-izq">
        <NivelesParalelismo />
        <Concepto id="flynn" />
      </div>
      <table className="tabla-niveles">
        <thead>
          <tr><th>Nivel</th><th>Tecnología</th><th>Riesgo típico</th><th>Cómo lo resolvemos</th></tr>
        </thead>
        <tbody>
          {niveles.map((n) => (
            <tr key={n.nivel}><td><strong>{n.nivel}</strong></td><td>{n.tecnologia}</td><td>{n.riesgo}</td><td>{n.solucion}</td></tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function MemoriaCompartida({ c, datos }) {
  const e1b = datos.experimento1b?.series ?? {}
  const serie = (clave, nombre, color) => ({
    nombre,
    color,
    puntos: (e1b[clave] ?? []).map((p) => ({ p: p.unidades, s: p.speedup })),
  })
  const series = [serie('paralelo_reduccion', 'Reducción', 'var(--serie-3)'), serie('paralelo_mutex', 'Mutex', 'var(--serie-2)'), serie('paralelo_atomico', 'CAS', 'var(--serie-4)')].filter((s) => s.puntos.length)
  const kf8 = (e1b.paralelo_reduccion ?? []).find((p) => p.unidades === 8)?.karp_flatt
  return (
    <div className="llena">
      <div className="dos-columnas">
        <div className="pila">
          {series.length > 0 && <GraficaSpeedup series={series} />}
          <div className="cifras compactas tres">
            <Cifra valor={`${c.reduccionS}×`} texto="reducción privada, 8 goroutines" tono="acento" />
            <Cifra valor={`${c.mutexS}×`} texto="mutex por ejemplo" tono="alerta" />
            <Cifra valor={fmt(kf8, 3)} texto="Karp-Flatt de la reducción (p = 8)" />
          </div>
        </div>
        <Concepto id="speedup" />
      </div>
      <div className="codigos">
        <figure className="codigo malo">
          <figcaption>Sección crítica por ejemplo (mutex) · contención</figcaption>
          <pre><code>{codigo.mutex}</code></pre>
        </figure>
        <figure className="codigo bueno">
          <figcaption>Reducción: casillas privadas + barrera · escala</figcaption>
          <pre><code>{codigo.reduccion}</code></pre>
        </figure>
      </div>
    </div>
  )
}

function Distribucion({ c }) {
  return (
    <div className="llena">
      <div className="dos-columnas">
        <div className="pila">
          <FlujoColectivas />
          <div className="cifras compactas">
            <Cifra valor="13" texto="números por época, de ida y de vuelta" />
            <Cifra valor="~1e-15" texto={`diferencia con la pérdida secuencial (${c.perdida})`} tono="acento" />
          </div>
        </div>
        <Concepto id="colectivas" />
      </div>
      <table className="tabla-mensajes">
        <thead>
          <tr><th>Mensaje (JSON por TCP)</th><th>Sentido</th><th>Contenido</th><th>Cuándo</th></tr>
        </thead>
        <tbody>
          {mensajes.map((m) => (
            <tr key={m.tipo}><td><code>{m.tipo}</code></td><td>{m.sentido}</td><td>{m.contenido}</td><td>{m.cuando}</td></tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function ElCluster({ datos }) {
  const { conectado, inst } = useCluster()
  const enVivo = conectado && inst?.sala?.length
  const nodos = enVivo ? inst.sala : integrantes.filter((i) => i.rol === 'Nodo')
  const calculo = calculoPorNodo(datos)
  const detalleCalculo = {
    'augusto-pc': 'con los 2,5 M ejemplos',
    josue: 'con los 2,5 M ejemplos',
    angelo: 'con 1/3 de los ejemplos',
  }
  return (
    <div className="llena">
      <div className="dos-columnas">
        <div className="pila">
          <ul className="nodos-cluster">
            <li className="nodo-maestro">
              <span className="insignia">MAESTRO</span>
              <strong>piero</strong>
              <span>reparte, suma y sirve esta página</span>
              <span className="nodo-medida">T(1): {fmt(cifras(datos).t1EpocaN, 0)} ms/época en 1 hilo</span>
            </li>
            {nodos.map((n) => (
              <li key={n.id} className={esGPU(n.motor) ? 'gpu' : 'cpu'}>
                <span className="insignia">{esGPU(n.motor) ? 'GPU · CUDA' : 'CPU · goroutines'}</span>
                <strong>{n.id}</strong>
                <span>{(n.dispositivo ?? '').split(',')[0]}</span>
                {Number.isFinite(calculo[n.id]) && (
                  <span className="nodo-medida">{fmt(calculo[n.id])} ms/época {detalleCalculo[n.id] ?? ''}</span>
                )}
              </li>
            ))}
          </ul>
          <p className="nota">
            {enVivo ? `● ${nodos.length} nodo(s) conectados ahora` : 'Nodos del grupo'} por Tailscale (red privada sobre internet, conexión directa: 5-17 ms de ida y vuelta).
            Antes de entrenar, cada GPU compara su gradiente con el de la CPU (tolerancia 1e-8); si no coincide, el nodo calcula con la CPU.
          </p>
        </div>
        <Concepto id="cuda" />
      </div>
      <div className="arbol-con-texto">
        <ArbolReduccion />
        <ol className="pasos-kernel">
          <li><strong>Copia única:</strong> la partición se copia a la memoria global de la GPU una vez por corrida.</li>
          <li><strong>Grid-stride:</strong> cada hilo recorre sus ejemplos y acumula en registros.</li>
          <li><strong>Árbol en __shared__:</strong> el bloque suma en log₂ n pasos, con __syncthreads entre niveles.</li>
          <li><strong>Resultado:</strong> la CPU suma los bloques en orden fijo: determinista, sin atomicAdd.</li>
        </ol>
      </div>
    </div>
  )
}

function Demo({ datos }) {
  return (
    <Protector>
      <PanelCluster datos={datos} grande />
    </Protector>
  )
}

// Corridas terminadas de esta sesión (las de la demo), en vivo.
function SesionEnVivo() {
  const { conectado, inst } = useCluster()
  const hechas = (inst?.historial ?? []).filter((h) => h.estado === 'terminado' && h.t_epoca_ms > 0).slice(-6)
  if (!conectado || !hechas.length) return null
  return (
    <BarrasH
      titulo="Medido en esta sesión"
      tema="En vivo"
      filas={hechas.map((h) => ({
        clave: String(h.id),
        etiqueta: (h.nodos ?? []).map((id, i) => `${id} (${esGPU(h.motores?.[i]) ? 'GPU' : 'CPU'})`).join(' + '),
        valores: { calculo: h.t_calculo_ms, red: h.t_comunicacion_ms },
        extra: `S = ${h.speedup.toFixed(1)} · ${h.t_total_s.toFixed(2)} s`,
      }))}
      capas={[
        { clave: 'calculo', nombre: 'Cálculo', color: 'var(--serie-3)' },
        { clave: 'red', nombre: 'Red y espera', color: 'var(--serie-2)' },
      ]}
    />
  )
}

function Resultados({ c, datos }) {
  return (
    <div className="dos-columnas">
      <div className="pila">
        <ModeloCosto t1Prueba={c.t1EpocaPruebaN} t1Completa={c.t1EpocaN} tRed={c.redN} />
        <p className="nota">Modelo con nodos iguales: T(p) = T_cálculo/p + T_red, con T_red ≈ {c.redN.toFixed(1)} ms medido por internet.</p>
        <Concepto id="amdahl" />
      </div>
      <div className="pila">
        <ResultadosCluster experimento2={datos.experimento2} experimento2Prueba={datos.experimento2_prueba} compacto />
        <SesionEnVivo />
      </div>
    </div>
  )
}

function Conclusiones({ c, datos }) {
  const puntos = [
    { cifra: `${c.pctPrueba}`, titulo: 'Con pocos datos, la red domina', texto: 'La comunicación síncrona es la fracción serial: no conviene distribuir.' },
    { cifra: `${c.gpu1S}×`, titulo: 'Con todos los datos, el clúster gana', texto: `Una GPU remota entrena ${c.gpu1S} veces más rápido que la CPU local, con el mismo modelo.` },
    { cifra: `${c.gpu1S} → ${c.mixto3S}`, titulo: 'El más lento marca el ritmo', texto: `Sumar un nodo CPU (${c.mixto3Calc} ms por época) frena a las GPUs en cada barrera.` },
  ]
  return (
    <div className="llena">
      <div className="dos-columnas ancha-izq">
        <ol className="conclusiones">
          {puntos.map((p) => (
            <li key={p.titulo}>
              <span className="conclusion-cifra">{p.cifra}</span>
              <div>
                <strong>{p.titulo}</strong>
                <p>{p.texto}</p>
              </div>
            </li>
          ))}
        </ol>
        <div className="pila">
          <Concepto id="balanceo" />
          <table className="tabla-teoria">
            <thead><tr><th>Teoría del curso</th><th>Lo que medimos</th></tr></thead>
            <tbody>
              {teoriaMedido(datos).map((t) => (
                <tr key={t.teoria}><td>{t.teoria}</td><td>{t.medido}</td></tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <div className="proximos">
        {proximos.map((p) => (
          <div key={p.titulo} className="proximo">
            <span className="concepto-rotulo">Próximo · {p.semana}</span>
            <strong>{p.titulo}</strong>
            <span>{p.detalle}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

function Temas({ datos }) {
  const temas = temasCurso(datos)
  const cuenta = (e) => temas.filter((t) => t.estado === e).length
  return (
    <div className="llena">
      <div className="cifras compactas tres">
        <Cifra valor={cuenta('hecho')} texto="temas hechos y medidos" tono="acento" />
        <Cifra valor={cuenta('go')} texto="hechos en Go, núcleo nativo pendiente" />
        <Cifra valor={cuenta('pendiente')} texto="pendientes, con contrato de datos listo" />
      </div>
      <table className="matriz-temas">
        <thead>
          <tr><th>Unidad</th><th>Tema</th><th>Evidencia medida en SACA</th><th>Estado</th></tr>
        </thead>
        <tbody>
          {temas.map((t) => (
            <tr key={t.tema}>
              <td className="unidad">{t.unidad}</td>
              <td>{t.tema}</td>
              <td>{t.evidencia}</td>
              <td><span className={`estado-tema ${t.estado}`}>{etiquetaEstado[t.estado]}</span></td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="cierre">
        <strong>Gracias · ¿Preguntas?</strong>
        <span>{grupo.map((g) => `${g.nombres.split(' ')[0]} ${g.apellidos.split(' ')[0]}`).join(' · ')}</span>
      </div>
    </div>
  )
}

// --- Plantilla y controles ---------------------------------------------

// Ajustado escala su contenido para que quepa entero en el espacio de la
// diapositiva (sin desplazarse hacia abajo en pantalla completa). Si cabe,
// queda a tamaño normal. Se recalcula al cambiar el tamaño de la ventana o
// del contenido (p. ej. cuando llegan datos en vivo).
function Ajustado({ children }) {
  const marco = useRef(null)
  const contenido = useRef(null)
  const [escala, setEscala] = useState(1)
  useLayoutEffect(() => {
    const calcular = () => {
      const m = marco.current
      const c = contenido.current
      if (!m || !c || !c.scrollHeight) return
      // scrollHeight/Width no cambian con transform: no hay ciclo.
      const s = Math.min(1, m.clientHeight / c.scrollHeight, m.clientWidth / c.scrollWidth)
      setEscala(Math.max(0.4, Math.floor(s * 100) / 100))
    }
    calcular()
    if (typeof ResizeObserver === 'undefined') return undefined
    const ro = new ResizeObserver(calcular)
    ro.observe(marco.current)
    ro.observe(contenido.current)
    return () => ro.disconnect()
  }, [])
  return (
    <div ref={marco} className="ajuste-marco">
      <div ref={contenido} className="ajuste-contenido" style={{ transform: escala < 1 ? `scale(${escala})` : undefined }}>
        {children}
      </div>
    </div>
  )
}

function useCronometro() {
  const [inicio, setInicio] = useState(() => Date.now())
  const [ahora, setAhora] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setAhora(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])
  const reiniciar = useCallback(() => {
    setInicio(Date.now())
    setAhora(Date.now())
  }, [])
  const s = Math.max(0, Math.floor((ahora - inicio) / 1000))
  return [`${dosDigitos(Math.floor(s / 60))}:${dosDigitos(s % 60)}`, s, reiniciar]
}

export default function Presentacion({ indice, setIndice, datos, estaciones, estacion, setElegida }) {
  const [notas, setNotas] = useState(false)
  const [tiempo, segundos, reiniciar] = useCronometro()
  const c = cifras(datos)

  useEffect(() => {
    const tecla = (e) => {
      if (['INPUT', 'TEXTAREA', 'SELECT', 'BUTTON'].includes(e.target?.tagName)) return
      if (e.key === 'ArrowRight' || e.key === ' ' || e.key === 'PageDown') {
        e.preventDefault()
        setIndice(Math.min(indice + 1, TOTAL_DIAPOSITIVAS))
      } else if (e.key === 'ArrowLeft' || e.key === 'PageUp') {
        e.preventDefault()
        setIndice(Math.max(indice - 1, 1))
      } else if (e.key === 'n' || e.key === 'N') {
        setNotas((v) => !v)
      } else if (e.key === 'f' || e.key === 'F') {
        if (document.fullscreenElement) document.exitFullscreen?.()
        else document.documentElement.requestFullscreen?.()
      } else if (e.key === 't' || e.key === 'T') {
        reiniciar()
      }
    }
    window.addEventListener('keydown', tecla)
    return () => window.removeEventListener('keydown', tecla)
  }, [indice, setIndice, reiniciar])

  const meta = diapositivas[indice - 1]
  const contenido = {
    1: <Portada c={c} />,
    2: <Problema c={c} estaciones={estaciones} estacion={estacion} setElegida={setElegida} />,
    3: <Arquitectura />,
    4: <MemoriaCompartida c={c} datos={datos} />,
    5: <Distribucion c={c} />,
    6: <ElCluster datos={datos} />,
    7: <Demo datos={datos} />,
    8: <Resultados c={c} datos={datos} />,
    9: <Conclusiones c={c} datos={datos} />,
    10: <Temas datos={datos} />,
  }[indice]

  return (
    <div className="presentacion">
      <section className={`diapositiva ${indice === 1 ? 'es-portada' : ''} ${indice === 7 ? 'es-demo' : ''}`}>
        <header className="diapositiva-cabecera">
          <span className="seccion">{dosDigitos(indice)} · {meta.seccion}</span>
          {meta.tema && <span className="tema-silabo">{meta.tema}</span>}
        </header>
        <h2 className="diapositiva-titulo">{meta.titulo}</h2>
        <div className="diapositiva-cuerpo">
          {/* La demo se desplaza dentro de la diapositiva; el resto se ajusta para caber entero. */}
          {indice === 7 ? (
            <Protector key={indice}>{contenido}</Protector>
          ) : (
            <Ajustado key={indice}>
              <Protector>{contenido}</Protector>
            </Ajustado>
          )}
        </div>
        <footer className="diapositiva-pie">
          <div className="progreso-diapositivas"><div style={{ width: `${(100 * indice) / TOTAL_DIAPOSITIVAS}%` }} /></div>
          <span>SACA · Programación Concurrente y Paralela</span>
          <span className={`cronometro ${segundos > 600 ? 'pasado' : ''}`} title="Tiempo de exposición (T reinicia)">{tiempo}</span>
          <span>{indice} / {TOTAL_DIAPOSITIVAS}</span>
        </footer>
      </section>

      {notas && (
        <aside className="notas-orador" aria-label="Notas del orador">
          <strong>Notas · {meta.seccion}</strong>
          <p>{meta.notas}</p>
        </aside>
      )}

      <nav className="navegacion" aria-label="Navegación de la presentación">
        <button onClick={() => setIndice(Math.max(indice - 1, 1))} disabled={indice === 1}>←</button>
        <span>{indice} / {TOTAL_DIAPOSITIVAS}</span>
        <button onClick={() => setIndice(Math.min(indice + 1, TOTAL_DIAPOSITIVAS))} disabled={indice === TOTAL_DIAPOSITIVAS}>→</button>
        <span className="atajos">N notas · F pantalla completa · T cronómetro</span>
      </nav>
    </div>
  )
}
