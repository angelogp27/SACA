import { useMemo, useState } from 'react'
import PanelMapa from './components/PanelMapa'
import PanelLineaTiempo from './components/PanelLineaTiempo'
import PanelCluster from './components/PanelCluster'
import PanelRendimiento from './components/PanelRendimiento'
import PanelTecnologias from './components/PanelTecnologias'
import Protector from './components/Protector'
import FlujoCluster from './components/flujo/FlujoCluster'
import { apiLocal, useLocal } from './data/local'
import { useDatosReales, estacionesParaPaneles, serieDeEstacion } from './data/datosReales'
import './App.css'

const formato = new Intl.NumberFormat('es-PE')

// Arriba a la izquierda: si esta laptop es el maestro (con sus IPs, para que
// los demás se unan) o un nodo, y el botón para salir del clúster.
function RolLaptop({ lanzador }) {
  const local = lanzador.datos
  if (!local?.rol) return <span />
  const esMaestro = local.rol === 'maestro'
  return (
    <div className="rol-laptop">
      <span className={`rol-insignia ${esMaestro ? 'maestro' : 'nodo'}`}>{esMaestro ? 'Maestro' : 'Nodo'}</span>
      <span className="rol-dato">{local.id}</span>
      {esMaestro && <span className="rol-dato">IP {local.ip || 'sin red'}</span>}
      <button
        type="button"
        className="rol-salir"
        onClick={async () => {
          await apiLocal.detener().catch(() => {})
          lanzador.actualizar()
        }}
      >
        {esMaestro ? 'Apagar clúster' : 'Salir del clúster'}
      </button>
    </div>
  )
}

function App() {
  const datos = useDatosReales()
  const estaciones = useMemo(() => estacionesParaPaneles(datos.estaciones), [datos.estaciones])
  const [elegida, setElegida] = useState(null)
  // La página abre en el flujo del clúster; sin el lanzador local
  // (bin/saca), esa pestaña explica cómo iniciarlo.
  const lanzador = useLocal()
  const [vista, setVista] = useState(null)
  const vistaActual = vista ?? 'cluster'

  // Por defecto, la estación de mayor riesgo que tenga serie con predicción.
  const estacionId =
    elegida ??
    (estaciones.lista.find((e) => datos.series?.[e.id]?.probabilidad) ?? estaciones.lista[0])?.id
  const estacion = estaciones.lista.find((e) => e.id === estacionId) ?? estaciones.lista[0]
  const serie = estacion ? serieDeEstacion(datos.series, estacion) : null
  const resumen = datos.resumen

  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="app-barra">
          <RolLaptop lanzador={lanzador} />
          <div className="vistas" role="tablist" aria-label="Vista">
            <button type="button" role="tab" aria-selected={vistaActual === 'cluster'} onClick={() => setVista('cluster')}>Clúster</button>
            <button type="button" role="tab" aria-selected={vistaActual === 'tablero'} onClick={() => setVista('tablero')}>Tablero</button>
          </div>
        </div>
        <h1 className="app-marca">
          <span className="app-sigla">SACA</span>
          <span className="app-nombre">Sistema de Alerta de Calidad del Aire</span>
        </h1>
        {vistaActual === 'tablero' && (
          <p>
            {estaciones.real
              ? 'Datos reales de la red de monitoreo de la EPA (2025), procesados por el cargador concurrente.'
              : 'Maqueta con datos de ejemplo: ejecuta herramientas/cmd/exportar para ver datos reales.'}
          </p>
        )}
        {vistaActual === 'tablero' && resumen && (
          <dl className="resumen">
            <div><dt>Filas crudas</dt><dd>{formato.format(resumen.filas_crudas)}</dd></div>
            <div><dt>Registros (estación, hora)</dt><dd>{formato.format(resumen.registros_unicos)}</dd></div>
            <div><dt>Estaciones</dt><dd>{formato.format(resumen.estaciones)}</dd></div>
            {resumen.modelo && (
              <div><dt>AUC del modelo (oct-dic)</dt><dd>{resumen.modelo.auc.toFixed(3)}</dd></div>
            )}
          </dl>
        )}
      </header>

      {vistaActual === 'cluster' ? (
        <main>
          <FlujoCluster datos={datos} lanzador={lanzador} />
        </main>
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
        <PanelTecnologias />
        <Protector>
          <PanelCluster datos={datos} />
        </Protector>
      </main>
      )}
    </div>
  )
}

export default App
