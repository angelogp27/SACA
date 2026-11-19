// Barra de arriba del clúster: Nuevo / Resultados y la línea de fases
// (limpieza e ingeniería de features → entrenamiento → predicción
// distribuida → resultados finales), con el estado de cada una en esta
// sesión: verde terminada, amarillo en curso, gris pendiente.

const FASES = [
  { id: 'limpieza', nombre: 'Limpieza e ingeniería de features' },
  { id: 'entrenamiento', nombre: 'Entrenamiento' },
  { id: 'prediccion', nombre: 'Predicción distribuida' },
  { id: 'final', nombre: 'Resultados finales' },
]

const TEXTO_ESTADO = { hecho: 'terminada', curso: 'en curso', pendiente: 'pendiente', parada: 'parada o con error', omitida: 'omitida' }

export default function Fases({ modo, setModo, fase, estados, onFase }) {
  return (
    <nav className="flujo-fases" aria-label="Fases del clúster">
      <div className="modos" role="tablist" aria-label="Modo">
        <button type="button" role="tab" aria-selected={modo === 'nuevo'} onClick={() => setModo('nuevo')}>Nuevo</button>
        <button type="button" role="tab" aria-selected={modo === 'resultados'} onClick={() => setModo('resultados')}>Resultados</button>
      </div>
      <ol className="fases">
        {FASES.map((f) => {
          const estado = estados[f.id] ?? 'pendiente'
          return (
            <li key={f.id} className={`fase ${estado} ${fase === f.id ? 'actual' : ''}`}>
              <button type="button" onClick={() => onFase(f.id)} aria-current={fase === f.id ? 'step' : undefined}
                title={`${f.nombre}: ${TEXTO_ESTADO[estado]}`}>
                <span className="fase-circulo" aria-hidden="true" />
                <span className="fase-nombre">{f.nombre}</span>
                {estado === 'omitida' && <small>omitida</small>}
              </button>
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
