// Modo presentación: el recorrido del avance en 10 diapositivas, con los
// datos reales y la demo del clúster en vivo. Se navega con los botones,
// con las flechas ← → o con la barra espaciadora; la diapositiva actual
// queda en la URL (#p3), así una recarga no pierde la posición.
import { useEffect } from 'react'
import { cifras, etiquetaEstado, integrantes, temasCurso } from '../data/presentacion'
import { esGPU, useCluster } from '../data/cluster'
import PanelMapa from './PanelMapa'
import PanelLineaTiempo from './PanelLineaTiempo'
import PanelRendimiento from './PanelRendimiento'
import PanelCluster from './PanelCluster'
import ResultadosCluster from './ResultadosCluster'
import './Presentacion.css'

const fmtN = new Intl.NumberFormat('es-PE')

function Diapositiva({ titulo, tema, children }) {
  return (
    <section className="diapositiva">
      <header>
        <h2>{titulo}</h2>
        {tema && <span className="tema-silabo">{tema}</span>}
      </header>
      <div className="diapositiva-cuerpo">{children}</div>
    </section>
  )
}

function Problema({ datos }) {
  const r = datos.resumen
  return (
    <Diapositiva titulo="SACA: alerta temprana de contaminación del aire">
      <p className="grande">
        ¿En qué estaciones de monitoreo y en qué horas el aire superará el nivel aceptable? Anticiparlo permite suspender
        actividades al aire libre, avisar a pacientes respiratorios o restringir el tráfico <em>antes</em> de la exposición.
      </p>
      <div className="cifras">
        <div><strong>{r ? fmtN.format(r.filas_crudas) : '16 972 287'}</strong><span>mediciones horarias de la EPA (2025), 4 contaminantes</span></div>
        <div><strong>{r ? fmtN.format(r.estaciones) : '1447'}</strong><span>estaciones de monitoreo</span></div>
        <div><strong>NO₂ &gt; 53 ppb</strong><span>en la próxima hora (umbral AQI 51), regresión logística</span></div>
        <div><strong>{r?.modelo ? r.modelo.auc.toFixed(3) : '0,994'}</strong><span>AUC en meses no vistos (oct-dic)</span></div>
      </div>
      <p>El foco del curso no es el modelo sino cómo se paraleliza: ingesta concurrente, entrenamiento en memoria compartida y entrenamiento distribuido en un clúster real.</p>
    </Diapositiva>
  )
}

function Datos({ estaciones, estacion, setElegida, serie }) {
  return (
    <Diapositiva titulo="Los datos: 1447 estaciones, 16,9 M de mediciones" tema="Sem. 5-6 · Cargador concurrente">
      <div className="dos-columnas">
        <PanelMapa estaciones={estaciones} seleccionada={estacion?.id} onSeleccionar={setElegida} />
        {estacion && <PanelLineaTiempo estacion={estacion} serie={serie} />}
      </div>
    </Diapositiva>
  )
}

function PorQueDistribuir() {
  return (
    <Diapositiva titulo="¿Por qué se puede distribuir el entrenamiento?" tema="Sem. 3-4 · Paso de mensajes · Sem. 10 · Descomposición de datos">
      <div className="dos-columnas">
        <div>
          <p className="formula">∇L = Σ<sub>i</sub> ∇ℓ<sub>i</sub> = Σ<sub>nodo 1</sub> + Σ<sub>nodo 2</sub> + Σ<sub>nodo 3</sub></p>
          <p className="grande">
            El gradiente de la regresión logística es una <strong>suma sobre ejemplos independientes</strong>. Cada nodo suma
            sobre sus estaciones y el maestro suma los resultados: el modelo es <strong>idéntico</strong> al secuencial.
          </p>
          <ol className="pasos">
            <li><strong>Scatter</strong>: cada nodo recibe su partición (hash de la estación).</li>
            <li><strong>Broadcast</strong>: en cada época, el maestro envía los 13 pesos.</li>
            <li>Cada nodo calcula su <strong>gradiente parcial</strong> (CPU o GPU).</li>
            <li><strong>Reduce</strong>: el maestro suma, actualiza y repite (<em>parameter server</em> síncrono).</li>
          </ol>
        </div>
        <svg viewBox="0 0 360 250" className="diagrama" role="img" aria-label="Maestro difunde pesos y suma gradientes de tres nodos">
          <rect x="120" y="14" width="120" height="48" rx="8" className="caja-maestro" />
          <text x="180" y="35" className="caja-texto">Maestro</text>
          <text x="180" y="52" className="caja-sub">suma y actualiza</text>
          {[50, 180, 310].map((x, i) => (
            <g key={x}>
              <rect x={x - 48} y="180" width="96" height="48" rx="8" className="caja-nodo" />
              <text x={x} y="201" className="caja-texto">Nodo {i + 1}</text>
              <text x={x} y="218" className="caja-sub">∇ de su parte</text>
              <line x1={180 - 20 + i * 20} y1="62" x2={x - 10} y2="180" className="flecha-baja" markerEnd="url(#punta)" />
              <line x1={x + 10} y1="180" x2={180 - 10 + i * 20} y2="62" className="flecha-sube" markerEnd="url(#punta2)" />
            </g>
          ))}
          <text x="62" y="118" className="etiqueta-flecha">pesos (13)</text>
          <text x="300" y="118" className="etiqueta-flecha">gradientes</text>
          <defs>
            <marker id="punta" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto"><path d="M0,0 L10,5 L0,10 z" className="punta-baja" /></marker>
            <marker id="punta2" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto"><path d="M0,0 L10,5 L0,10 z" className="punta-sube" /></marker>
          </defs>
        </svg>
      </div>
    </Diapositiva>
  )
}

function DeDondeVenimos({ datos }) {
  const c = cifras(datos)
  return (
    <Diapositiva titulo="De dónde venimos: memoria compartida en una PC" tema="Sem. 5-7 · Goroutines, channels, mutex, atómicos, reducción">
      <div className="dos-columnas">
        <PanelRendimiento experimento1={datos.experimento1} experimento1b={datos.experimento1b} experimento2={datos.experimento2} experimento2Prueba={datos.experimento2_prueba} inicial="experimento1b" />
        <ul className="claves">
          <li><strong>Cargador</strong> (worker pool + channels): {c.cargadorS}. Con una sola agregadora el techo de Amdahl era 1,25× (f = {c.cargadorF} medida).</li>
          <li><strong>Entrenamiento</strong> con goroutines: la reducción con casillas privadas logra S(8) = {c.reduccionS}.</li>
          <li><strong>Mutex y CAS por ejemplo</strong> son más lentos que el secuencial (eficiencia {c.mutexE}): la contención en la sección crítica cuesta más que el cálculo.</li>
          <li>El clúster reutiliza la misma idea de reducción, ahora <strong>entre máquinas</strong>.</li>
        </ul>
      </div>
    </Diapositiva>
  )
}

function ElCluster() {
  const { conectado, inst } = useCluster()
  const nodos = conectado && inst?.sala?.length
    ? inst.sala.map((n) => ({ id: n.id, motor: n.motor, dispositivo: n.dispositivo }))
    : integrantes.filter((i) => i.rol === 'Nodo')
  const ancho = 640
  const paso = ancho / (nodos.length + 1)
  return (
    <Diapositiva titulo="El clúster del grupo: 4 PCs en 4 casas" tema="Sem. 2 · MIMD de memoria distribuida, heterogéneo">
      <svg viewBox={`0 0 ${ancho} 300`} className="diagrama ancho" role="img" aria-label="Maestro y nodos unidos por Tailscale">
        <rect x={ancho / 2 - 110} y="12" width="220" height="62" rx="10" className="caja-maestro" />
        <text x={ancho / 2} y="38" className="caja-texto">Maestro · Piero</text>
        <text x={ancho / 2} y="58" className="caja-sub">Ryzen 5 3600 · reparte, suma, sirve la página</text>
        <text x={ancho / 2} y="128" className="etiqueta-nube">Internet · red privada Tailscale (TCP, JSON por línea)</text>
        {nodos.map((n, i) => {
          const x = paso * (i + 1)
          const gpu = esGPU(n.motor)
          return (
            <g key={n.id}>
              <line x1={ancho / 2} y1="74" x2={x} y2="186" className="enlace" />
              <rect x={x - 88} y="186" width="176" height="92" rx="10" className={gpu ? 'caja-gpu' : 'caja-cpu'} />
              <text x={x} y="210" className="caja-texto">{n.id}</text>
              <text x={x} y="230" className="caja-sub">{gpu ? 'GPU · CUDA' : 'CPU · goroutines'}</text>
              <text x={x} y="250" className="caja-sub">{(n.dispositivo ?? '').split(',')[0].slice(0, 30)}</text>
              <text x={x} y="268" className="caja-sub">{conectado && inst?.sala?.length ? '● conectado ahora' : ''}</text>
            </g>
          )
        })}
      </svg>
      <ul className="claves en-fila">
        <li><strong>Sin memoria compartida</strong>: solo mensajes (registro, partición, pesos, gradiente, fin, baja).</li>
        <li><strong>Heterogéneo</strong>: cada nodo calcula con lo que tiene; la GPU se valida contra la CPU antes de usarse.</li>
        <li><strong>Correcto</strong>: el maestro rechaza nodos con otros datos (huella SHA-256).</li>
      </ul>
    </Diapositiva>
  )
}

function Demo({ datos }) {
  return (
    <Diapositiva titulo="Demo en vivo: elige los nodos y entrena" tema="Todo el curso, en vivo">
      <PanelCluster datos={datos} grande />
    </Diapositiva>
  )
}

function Resultados({ datos }) {
  return (
    <Diapositiva titulo="Resultados: cuándo conviene un clúster" tema="Sem. 1 y 13 · Evaluación empírica de la aceleración">
      <ResultadosCluster experimento2={datos.experimento2} experimento2Prueba={datos.experimento2_prueba} />
    </Diapositiva>
  )
}

function Lecciones({ datos }) {
  const c = cifras(datos)
  return (
    <Diapositiva titulo="Lecciones" tema="Amdahl · balanceo de carga · correctitud">
      <ol className="lecciones">
        <li><strong>El tamaño del problema decide (Amdahl).</strong> La red cuesta lo mismo con 146 mil que con 2,5 M ejemplos (13 números por época); lo que crece es el cálculo. Con pocos datos, {c.pctPrueba} del tiempo es red y no conviene distribuir; con todos, una GPU remota es {c.gpu1S} veces más rápida que el secuencial.</li>
        <li><strong>Más nodos no siempre es más rápido.</strong> Con 2 GPUs, S = {c.gpu2S} (menos que con una): partir 3 ms de cálculo ahorra poco y cada nodo suma su espera de red.</li>
        <li><strong>El más lento marca el ritmo.</strong> Con el nodo CPU, cada época espera {c.mixto3Calc} ms aunque una GPU termina en ~1 ms: S = {c.mixto3S}. Es la barrera síncrona; la solución es repartir según el rendimiento.</li>
        <li><strong>Correctitud ante todo.</strong> CPU o GPU, Windows o Linux, 1 o 3 nodos: la pérdida final es {c.perdida} en todas las corridas (diferencia ~1e-15).</li>
      </ol>
    </Diapositiva>
  )
}

function Temas({ datos }) {
  return (
    <Diapositiva titulo="Temas del curso aplicados en SACA" tema="Unidades I a IV">
      <div className="tabla-temas">
        {temasCurso(datos).map((u) => (
          <table key={u.unidad}>
            <caption>{u.unidad}</caption>
            <thead>
              <tr><th>Tema (semana)</th><th>Dónde está</th><th>Evidencia medida</th><th>Estado</th></tr>
            </thead>
            <tbody>
              {u.temas.map((t) => (
                <tr key={t.tema}>
                  <td>{t.tema}</td>
                  <td>{t.donde}</td>
                  <td>{t.evidencia}</td>
                  <td><span className={`estado-tema ${t.estado}`}>{etiquetaEstado[t.estado]}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        ))}
      </div>
    </Diapositiva>
  )
}

function Proximos() {
  return (
    <Diapositiva titulo="Próximos pasos" tema="Semanas 9-13">
      <ul className="lecciones">
        <li><strong>Reparto proporcional al rendimiento (Experimento 9):</strong> cada nodo recibe ejemplos según su velocidad medida; el cálculo por época con 3 nodos bajaría de ~18 ms a ~3,5 ms.</li>
        <li><strong>MPI (Experimentos 7 y 7a):</strong> MPI_Bcast + MPI_Allreduce frente a este TCP en Go; ping-pong de latencia y ancho de banda.</li>
        <li><strong>CUDA (Experimento 8):</strong> kernels con atomicAdd y warp shuffles; punto de cruce CPU frente a GPU según el tamaño.</li>
        <li><strong>Pthreads y OpenMP nativos:</strong> productor-consumidor y reducción en C con el mismo formato de datos.</li>
        <li><strong>Tolerancia a fallos:</strong> redistribuir la partición de un nodo caído o modo asíncrono (Experimento 4).</li>
      </ul>
    </Diapositiva>
  )
}

export const TOTAL_DIAPOSITIVAS = 10

export default function Presentacion({ indice, setIndice, datos, estaciones, estacion, setElegida, serie }) {
  useEffect(() => {
    const tecla = (e) => {
      const etiqueta = e.target?.tagName
      if (['INPUT', 'TEXTAREA', 'SELECT', 'BUTTON'].includes(etiqueta)) return
      if (e.key === 'ArrowRight' || e.key === ' ' || e.key === 'PageDown') {
        e.preventDefault()
        setIndice(Math.min(indice + 1, TOTAL_DIAPOSITIVAS))
      } else if (e.key === 'ArrowLeft' || e.key === 'PageUp') {
        e.preventDefault()
        setIndice(Math.max(indice - 1, 1))
      }
    }
    window.addEventListener('keydown', tecla)
    return () => window.removeEventListener('keydown', tecla)
  }, [indice, setIndice])

  const diapositivas = [
    <Problema key={1} datos={datos} />,
    <Datos key={2} estaciones={estaciones} estacion={estacion} setElegida={setElegida} serie={serie} />,
    <PorQueDistribuir key={3} />,
    <DeDondeVenimos key={4} datos={datos} />,
    <ElCluster key={5} />,
    <Demo key={6} datos={datos} />,
    <Resultados key={7} datos={datos} />,
    <Lecciones key={8} datos={datos} />,
    <Temas key={9} datos={datos} />,
    <Proximos key={10} />,
  ]

  return (
    <div className="presentacion">
      {diapositivas[indice - 1]}
      <nav className="navegacion" aria-label="Navegación de la presentación">
        <button onClick={() => setIndice(Math.max(indice - 1, 1))} disabled={indice === 1}>← Anterior</button>
        <span>{indice} / {TOTAL_DIAPOSITIVAS}</span>
        <button onClick={() => setIndice(Math.min(indice + 1, TOTAL_DIAPOSITIVAS))} disabled={indice === TOTAL_DIAPOSITIVAS}>Siguiente →</button>
      </nav>
    </div>
  )
}
