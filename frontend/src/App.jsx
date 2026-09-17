import { useState } from 'react'
import PanelMapa from './components/PanelMapa'
import PanelLineaTiempo from './components/PanelLineaTiempo'
import PanelCluster from './components/PanelCluster'
import PanelRendimiento from './components/PanelRendimiento'
import { estaciones } from './data/mock'
import './App.css'

function App() {
  const [estacionId, setEstacionId] = useState(estaciones[0].id)
  const estacion = estaciones.find((e) => e.id === estacionId)

  return (
    <div className="app-shell">
      <header className="app-header">
        <h1>Sistema de alerta temprana de contaminación del aire</h1>
        <p>Maqueta de la SPA — Parte D. Todos los paneles muestran datos de ejemplo hasta que existan las Partes B y C.</p>
      </header>

      <main className="dashboard">
        <PanelMapa seleccionada={estacionId} onSeleccionar={setEstacionId} />
        <PanelLineaTiempo estacion={estacion} />
        <PanelCluster />
        <PanelRendimiento />
      </main>
    </div>
  )
}

export default App
