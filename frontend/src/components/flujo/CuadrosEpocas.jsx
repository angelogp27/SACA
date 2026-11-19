// Un cuadro por época: se pinta al completarse, más oscuro cuanto menor la
// pérdida; la época en curso parpadea en amarillo. Al pasar el mouse, los
// datos de esa época.

const fmt = (v, d = 0) => (v == null ? '—' : v.toLocaleString('es-PE', { maximumFractionDigits: d, minimumFractionDigits: d }))

export default function CuadrosEpocas({ corrida, enCurso }) {
  const total = corrida.epocas ?? 0
  const perdidas = corrida.perdidas ?? []
  const min = Math.min(...perdidas), max = Math.max(...perdidas)
  const tono = (p) => {
    // 0 = la pérdida más alta (verde claro), 1 = la más baja (verde oscuro).
    const t = max > min ? (max - p) / (max - min) : 1
    return `hsl(140 55% ${(72 - 44 * t).toFixed(1)}%)`
  }
  return (
    <section className="cuadros-epocas" aria-label={`Épocas: ${perdidas.length} de ${total}`}>
      <span className="rotulo">ÉPOCAS · {perdidas.length} DE {total}</span>
      <div className="cuadros">
        {Array.from({ length: total }, (_, e) => {
          const hecha = e < perdidas.length
          const actual = enCurso && e === perdidas.length
          const titulo = hecha
            ? `Época ${e + 1}: pérdida ${fmt(perdidas[e], 4)} · ${fmt(corrida.t_epoca_ms?.[e])} ms (cálculo ${fmt(corrida.t_calculo_ms?.[e])}, red y espera ${fmt(corrida.t_red_ms?.[e])})${corrida.mb_epoca?.[e] != null ? ` · ${fmt(corrida.mb_epoca[e], 1)} MB` : ''}`
            : `Época ${e + 1}${actual ? ': en curso' : ''}`
          return (
            <span key={e} className={`cuadro ${hecha ? 'hecha' : ''} ${actual ? 'actual' : ''}`}
              style={hecha ? { background: tono(perdidas[e]) } : undefined} title={titulo} />
          )
        })}
      </div>
      <p className="detalle leyenda-cuadros">Más oscuro = menor pérdida. Pasa el mouse sobre un cuadro para ver su época.</p>
    </section>
  )
}
