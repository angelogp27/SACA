import { useEffect, useMemo, useState } from 'react'
import PanelMapa from './components/PanelMapa'
import PanelLineaTiempo from './components/PanelLineaTiempo'
import PanelCluster from './components/PanelCluster'
import PanelRendimiento from './components/PanelRendimiento'
import Presentacion, { TOTAL_DIAPOSITIVAS } from './components/Presentacion'
import Protector from './components/Protector'
import { useDatosReales, estacionesParaPaneles, serieDeEstacion } from './data/datosReales'
import './App.css'

const formato = new Intl.NumberFormat('es-PE')

// La vista vive en la URL: #p3 es la diapositiva 3 de la presentación;
// sin hash, el tablero. Así recargar o compartir el enlace no pierde la
// posición.
function leerHash() {
  const m = window.location.hash.match(/^#p(\d+)$/)
  if (!m) return 0
  return Math.min(Math.max(Number(m[1]), 1), TOTAL_DIAPOSITIVAS)
}

function useDiapositiva() {
  const [diapositiva, setDiapositiva] = useState(leerHash)
  useEffect(() => {
    const cambio = () => setDiapositiva(leerHash())
    window.addEventListener('hashchange', cambio)
    return () => window.removeEventListener('hashchange', cambio)
  }, [])
  const ir = (n) => {
    window.history.replaceState(null, '', n ? `#p${n}` : window.location.pathname)
    setDiapositiva(n)
  }
  return [diapositiva, ir]
}

function App() {
  const datos = useDatosReales()
  const estaciones = useMemo(() => estacionesParaPaneles(datos.estaciones), [datos.estaciones])
  const [elegida, setElegida] = useState(null)
  const [diapositiva, irA] = useDiapositiva()

  // Por defecto, la estación de mayor riesgo que tenga serie con predicción.
  const estacionId =
    elegida ??
    (estaciones.lista.find((e) => datos.series?.[e.id]?.probabilidad) ?? estaciones.lista[0])?.id
  const estacion = estaciones.lista.find((e) => e.id === estacionId) ?? estaciones.lista[0]
  const serie = estacion ? serieDeEstacion(datos.series, estacion) : null
  const resumen = datos.resumen
  const presentando = diapositiva > 0

  return (
    <div className={`app-shell ${presentando ? 'presentando' : ''}`}>
      <header className="app-header">
        <div className="app-titulo">
          <h1>SACA — Sistema de Alerta de Calidad del Aire</h1>
          <div className="vista-selector" role="group" aria-label="Vista">
            <button aria-pressed={presentando} onClick={() => irA(presentando ? diapositiva : 1)}>Presentación</button>
            <button aria-pressed={!presentando} onClick={() => irA(0)}>Tablero</button>
          </div>
        </div>
        {!presentando && (
          <>
            <p>
              {estaciones.real
                ? 'Datos reales de la red de monitoreo de la EPA (2025), procesados por el cargador concurrente. El clúster se controla en vivo desde el panel de abajo (maestro en modo servidor).'
                : 'Maqueta con datos de ejemplo: ejecuta herramientas/cmd/exportar para ver datos reales.'}
            </p>
            {resumen && (
              <dl className="resumen">
                <div><dt>Filas crudas</dt><dd>{formato.format(resumen.filas_crudas)}</dd></div>
                <div><dt>Registros (estación, hora)</dt><dd>{formato.format(resumen.registros_unicos)}</dd></div>
                <div><dt>Estaciones</dt><dd>{formato.format(resumen.estaciones)}</dd></div>
                {resumen.modelo && (
                  <div><dt>AUC del modelo (oct-dic)</dt><dd>{resumen.modelo.auc.toFixed(3)}</dd></div>
                )}
              </dl>
            )}
          </>
        )}
      </header>

      {presentando ? (
        <Protector key={`p${diapositiva}`}>
        <Presentacion
          indice={diapositiva}
          setIndice={irA}
          datos={datos}
          estaciones={estaciones}
          estacion={estacion}
          setElegida={setElegida}
          serie={serie}
        />
        </Protector>
      ) : (
        <main className="dashboard">
          <PanelMapa estaciones={estaciones} seleccionada={estacion?.id} onSeleccionar={setElegida} />
          {estacion && <PanelLineaTiempo estacion={estacion} serie={serie} />}
          <PanelRendimiento
            experimento1={datos.experimento1}
            experimento1b={datos.experimento1b}
            experimento2={datos.experimento2}
            experimento2Prueba={datos.experimento2_prueba}
          />
          <Protector>
            <PanelCluster datos={datos} />
          </Protector>
        </main>
      )}
    </div>
  )
}

export default App
