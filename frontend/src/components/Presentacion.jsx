// Modo presentación: la exposición del avance en 10 diapositivas (unos 10
// minutos, demo incluida), con los datos reales y el clúster en vivo.
// Teclas: ← → o espacio para navegar, N para las notas del orador, F para
// pantalla completa y T para reiniciar el cronómetro. La diapositiva
// actual queda en la URL (#p3). El guion completo está en
// docs/guion_exposicion.md.
import { useCallback, useEffect, useState } from 'react'
import { cifras, conceptos, diapositivas, etiquetaEstado, integrantes, temasCurso } from '../data/presentacion'
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
      <p className="portada-curso">Programación Concurrente y Paralela · UNMSM · 2026-2</p>
      <div className="cifras">
        <Cifra valor={fmtN.format(c.filas)} texto="mediciones horarias de la EPA (2025)" />
        <Cifra valor="4 PCs" texto="en 4 casas · 2 GPU NVIDIA + 1 CPU" />
        <Cifra valor={`${c.gpu1S}×`} texto="más rápido con una GPU remota que en secuencial" tono="acento" />
      </div>
      <p className="portada-autor">Avance del proyecto · Expone: Piero</p>
    </div>
  )
}

function Problema({ c, estaciones, estacion, setElegida }) {
  return (
    <div className="dos-columnas">
      <div className="mapa-pequeno">
        <PanelMapa estaciones={estaciones} seleccionada={estacion?.id} onSeleccionar={setElegida} />
      </div>
      <div className="pila">
        <p className="enunciado">
          ¿El NO₂ de esta estación superará <strong>53 ppb</strong> (umbral AQI 51) en la próxima hora?
        </p>
        <div className="cifras compactas">
          <Cifra valor={fmtN.format(c.registros)} texto="registros (estación, hora) tras la ingesta concurrente" />
          <Cifra valor={fmtN.format(c.estaciones)} texto="estaciones de monitoreo" />
          <Cifra valor={c.auc.toFixed(3)} texto="AUC en meses no vistos (oct-dic)" />
        </div>
        <p className="nota">Regresión logística por descenso de gradiente: el modelo es simple a propósito; el foco del curso es cómo se paraleliza.</p>
      </div>
    </div>
  )
}

function Arquitectura() {
  return (
    <div className="dos-columnas ancha-izq">
      <NivelesParalelismo />
      <Concepto id="flynn" />
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
  return (
    <div className="dos-columnas">
      <div className="pila">
        {series.length > 0 && <GraficaSpeedup series={series} />}
        <div className="cifras compactas">
          <Cifra valor={`${c.reduccionS}×`} texto="reducción privada con 8 goroutines" tono="acento" />
          <Cifra valor={`${c.mutexS}×`} texto="mutex por ejemplo: más lento que el secuencial" tono="alerta" />
        </div>
      </div>
      <Concepto id="speedup" />
    </div>
  )
}

function Distribucion({ c }) {
  return (
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
  )
}

function ElCluster() {
  const { conectado, inst } = useCluster()
  const enVivo = conectado && inst?.sala?.length
  const nodos = enVivo ? inst.sala : integrantes.filter((i) => i.rol === 'Nodo')
  return (
    <div className="dos-columnas">
      <div className="pila">
        <ul className="nodos-cluster">
          <li className="nodo-maestro">
            <span className="insignia">MAESTRO</span>
            <strong>piero</strong>
            <span>reparte, suma y sirve esta página</span>
          </li>
          {nodos.map((n) => (
            <li key={n.id} className={esGPU(n.motor) ? 'gpu' : 'cpu'}>
              <span className="insignia">{esGPU(n.motor) ? 'GPU · CUDA' : 'CPU · goroutines'}</span>
              <strong>{n.id}</strong>
              <span>{(n.dispositivo ?? '').split(',')[0]}</span>
            </li>
          ))}
        </ul>
        <p className="nota">{enVivo ? `● ${nodos.length} nodo(s) conectados ahora por Tailscale (red privada sobre internet).` : 'Nodos del grupo, unidos por Tailscale (red privada sobre internet).'}</p>
        <ArbolReduccion />
      </div>
      <Concepto id="cuda" />
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

function Conclusiones({ c }) {
  const puntos = [
    { cifra: `${c.pctPrueba}`, titulo: 'Con pocos datos, la red domina', texto: 'La comunicación síncrona es la fracción serial: no conviene distribuir.' },
    { cifra: `${c.gpu1S}×`, titulo: 'Con todos los datos, el clúster gana', texto: `Una GPU remota entrena ${c.gpu1S} veces más rápido que la CPU local, con el mismo modelo.` },
    { cifra: `${c.gpu1S} → ${c.mixto3S}`, titulo: 'El más lento marca el ritmo', texto: `Sumar un nodo CPU (${c.mixto3Calc} ms por época) frena a las GPUs en cada barrera.` },
  ]
  return (
    <div className="dos-columnas ancha-izq">
      <div className="pila">
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
        <div className="siguientes">
          <span className="concepto-rotulo">Próximos pasos</span>
          <p>Reparto proporcional al rendimiento · MPI_Bcast + MPI_Allreduce frente a TCP · CUDA con warp shuffles · Pthreads y OpenMP nativos · tolerancia a fallos</p>
        </div>
      </div>
      <Concepto id="balanceo" />
    </div>
  )
}

function Temas({ datos }) {
  return (
    <table className="matriz-temas">
      <thead>
        <tr><th>Unidad</th><th>Tema</th><th>Evidencia medida en SACA</th><th>Estado</th></tr>
      </thead>
      <tbody>
        {temasCurso(datos).map((t) => (
          <tr key={t.tema}>
            <td className="unidad">{t.unidad}</td>
            <td>{t.tema}</td>
            <td>{t.evidencia}</td>
            <td><span className={`estado-tema ${t.estado}`}>{etiquetaEstado[t.estado]}</span></td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

// --- Plantilla y controles ---------------------------------------------

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
    6: <ElCluster />,
    7: <Demo datos={datos} />,
    8: <Resultados c={c} datos={datos} />,
    9: <Conclusiones c={c} />,
    10: <Temas datos={datos} />,
  }[indice]

  return (
    <div className="presentacion">
      <section className={`diapositiva ${indice === 1 ? 'es-portada' : ''}`}>
        <header className="diapositiva-cabecera">
          <span className="seccion">{dosDigitos(indice)} · {meta.seccion}</span>
          {meta.tema && <span className="tema-silabo">{meta.tema}</span>}
        </header>
        <h2 className="diapositiva-titulo">{meta.titulo}</h2>
        <div className="diapositiva-cuerpo">
          <Protector key={indice}>{contenido}</Protector>
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
