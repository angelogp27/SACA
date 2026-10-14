import { useMemo, useState } from 'react'
import PanelMapa from './components/PanelMapa'
import PanelLineaTiempo from './components/PanelLineaTiempo'
import PanelCluster from './components/PanelCluster'
import PanelRendimiento from './components/PanelRendimiento'
import { useDatosReales, estacionesParaPaneles, serieDeEstacion } from './data/datosReales'
import './App.css'

const formato = new Intl.NumberFormat('es-PE')

function App() {
  const datos = useDatosReales()
  const estaciones = useMemo(() => estacionesParaPaneles(datos.estaciones), [datos.estaciones])
  const [elegida, setElegida] = useState(null)

  // Por defecto, la estación de mayor riesgo que tenga serie con predicción.
  const estacionId =
    elegida ??
    (estaciones.lista.find((e) => datos.series?.[e.id]?.probabilidad) ?? estaciones.lista[0])?.id
  const estacion = estaciones.lista.find((e) => e.id === estacionId) ?? estaciones.lista[0]
  const resumen = datos.resumen

  return (
    <div className="app-shell">
      <header className="app-header">
        <h1>SACA — Sistema de Alerta de Calidad del Aire</h1>
        <p>
          {estaciones.real
            ? 'Datos reales de la red de monitoreo de la EPA (2025), procesados por el cargador concurrente. La predicción en vivo llega con la API (Parte C).'
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
      </header>

      <main className="dashboard">
        <PanelMapa estaciones={estaciones} seleccionada={estacion?.id} onSeleccionar={setElegida} />
        {estacion && <PanelLineaTiempo estacion={estacion} serie={serieDeEstacion(datos.series, estacion)} />}
        <PanelRendimiento experimento1={datos.experimento1} experimento1b={datos.experimento1b} />
        <PanelCluster />
      </main>
    </div>
  )
}

export default App
