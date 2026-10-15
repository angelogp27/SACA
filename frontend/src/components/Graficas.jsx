// Gráficas SVG pequeñas y sin librerías para el clúster en vivo: líneas por
// época, área apilada (cálculo frente a red) y barras horizontales apiladas
// para comparar corridas. El eje X de las dos primeras es la época.
import './Graficas.css'

const ANCHO = 320
const PAD = { top: 10, right: 10, bottom: 20, left: 40 }
const MAX_PUNTOS = 300

// Reduce una serie larga a como mucho MAX_PUNTOS (promedio por tramo), para
// que el SVG siga liviano con miles de épocas.
function reducir(valores) {
  if (valores.length <= MAX_PUNTOS) return valores.map((v, i) => [i, v])
  const paso = valores.length / MAX_PUNTOS
  const res = []
  for (let k = 0; k < MAX_PUNTOS; k++) {
    const ini = Math.floor(k * paso)
    const fin = Math.max(ini + 1, Math.floor((k + 1) * paso))
    const tramo = valores.slice(ini, fin)
    res.push([ini, tramo.reduce((a, b) => a + b, 0) / tramo.length])
  }
  return res
}

function ejes({ alto, maxX, maxY, formatoY }) {
  const x = (i) => PAD.left + (maxX <= 1 ? 0 : (i / (maxX - 1)) * (ANCHO - PAD.left - PAD.right))
  const y = (v) => alto - PAD.bottom - (Math.min(v, maxY) / (maxY || 1)) * (alto - PAD.top - PAD.bottom)
  const grilla = [0, 0.5, 1].map((f) => (
    <g key={f}>
      <line x1={PAD.left} x2={ANCHO - PAD.right} y1={y(f * maxY)} y2={y(f * maxY)} className="g-grid" />
      <text x={PAD.left - 5} y={y(f * maxY)} className="g-tick-y">{formatoY(f * maxY)}</text>
    </g>
  ))
  return { x, y, grilla }
}

function Marco({ titulo, tema, leyenda, children }) {
  return (
    <div className="g-marco">
      <div className="g-cabecera">
        <h4>{titulo}</h4>
        {tema && <span className="tema-silabo">{tema}</span>}
      </div>
      {children}
      {leyenda?.length > 0 && (
        <ul className="g-leyenda">
          {leyenda.map((l) => (
            <li key={l.nombre}><span className="g-muestra" style={{ background: l.color }} />{l.nombre}</li>
          ))}
        </ul>
      )}
    </div>
  )
}

// GraficaLineas: una línea por serie; referencia es una recta horizontal
// opcional (p. ej. el speedup ideal).
export function GraficaLineas({ titulo, tema, series, epocas, formatoY = (v) => v.toFixed(1), alto = 150, referencia, maxY: maxYFijo }) {
  const maxX = Math.max(epocas ?? 0, ...series.map((s) => s.valores.length), 2)
  const todos = series.flatMap((s) => s.valores)
  const maxY = maxYFijo ?? Math.max(referencia?.valor ?? 0, ...todos, 1e-9) * 1.1
  const { x, y, grilla } = ejes({ alto, maxX, maxY, formatoY })
  return (
    <Marco titulo={titulo} tema={tema} leyenda={series.length > 1 ? series : null}>
      <svg viewBox={`0 0 ${ANCHO} ${alto}`} role="img" aria-label={titulo}>
        {grilla}
        {referencia && (
          <g>
            <line x1={PAD.left} x2={ANCHO - PAD.right} y1={y(referencia.valor)} y2={y(referencia.valor)} className="g-referencia" />
            <text x={ANCHO - PAD.right} y={y(referencia.valor) - 4} className="g-ref-texto">{referencia.etiqueta}</text>
          </g>
        )}
        {series.map((s) => {
          const puntos = reducir(s.valores)
          if (!puntos.length) return null
          const d = puntos.map(([i, v], k) => `${k ? 'L' : 'M'} ${x(i)} ${y(v)}`).join(' ')
          const [ui, uv] = puntos[puntos.length - 1]
          return (
            <g key={s.nombre}>
              <path d={d} className="g-linea" style={{ stroke: s.color }} />
              <circle cx={x(ui)} cy={y(uv)} r="3" style={{ fill: s.color }} />
            </g>
          )
        })}
        <text x={PAD.left} y={alto - 4} className="g-tick-x">época 1</text>
        <text x={ANCHO - PAD.right} y={alto - 4} className="g-tick-x g-fin">{maxX}</text>
      </svg>
    </Marco>
  )
}

// GraficaApilada: áreas apiladas por época (abajo la primera capa).
export function GraficaApilada({ titulo, tema, capas, epocas, formatoY = (v) => `${v.toFixed(0)} ms`, alto = 150 }) {
  const n = Math.max(...capas.map((c) => c.valores.length), 0)
  const maxX = Math.max(epocas ?? 0, n, 2)
  const sumas = Array.from({ length: n }, (_, i) => capas.reduce((a, c) => a + (c.valores[i] ?? 0), 0))
  const maxY = Math.max(...sumas, 1e-9) * 1.1
  const { x, y, grilla } = ejes({ alto, maxX, maxY, formatoY })
  // Cada capa va desde la suma de las anteriores (abajo) hasta esa suma
  // más sus valores (arriba).
  const tramos = capas.reduce((acc, c) => {
    const abajo = acc.length ? acc[acc.length - 1].arriba : Array(n).fill(0)
    acc.push({ capa: c, abajo, arriba: abajo.map((b, i) => b + (c.valores[i] ?? 0)) })
    return acc
  }, [])
  const areas = tramos.map(({ capa, abajo, arriba }) => {
    const sup = reducir(arriba)
    const inf = reducir(abajo).reverse()
    const d = n ? `M ${sup.map(([i, v]) => `${x(i)} ${y(v)}`).join(' L ')} L ${inf.map(([i, v]) => `${x(i)} ${y(v)}`).join(' L ')} Z` : ''
    return <path key={capa.nombre} d={d} className="g-area" style={{ fill: capa.color }} />
  })
  return (
    <Marco titulo={titulo} tema={tema} leyenda={capas}>
      <svg viewBox={`0 0 ${ANCHO} ${alto}`} role="img" aria-label={titulo}>
        {grilla}
        {areas}
        <text x={PAD.left} y={alto - 4} className="g-tick-x">época 1</text>
        <text x={ANCHO - PAD.right} y={alto - 4} className="g-tick-x g-fin">{maxX}</text>
      </svg>
    </Marco>
  )
}

// BarrasH: una barra horizontal apilada por fila (p. ej. ms por época de
// cálculo y de red en cada configuración), con un texto a la derecha.
export function BarrasH({ titulo, tema, filas, capas, formato = (v) => `${v.toFixed(1)} ms` }) {
  const max = Math.max(...filas.map((f) => capas.reduce((a, c) => a + (f.valores[c.clave] ?? 0), 0)), 1e-9)
  return (
    <Marco titulo={titulo} tema={tema} leyenda={capas}>
      <ul className="g-barras">
        {filas.map((f) => {
          const total = capas.reduce((a, c) => a + (f.valores[c.clave] ?? 0), 0)
          return (
            <li key={f.clave} className={f.destacada ? 'g-barra-destacada' : ''}>
              <span className="g-barra-etiqueta" title={f.detalle}>{f.etiqueta}</span>
              <span className="g-barra-pista">
                {capas.map((c) => (
                  <span
                    key={c.clave}
                    className="g-barra-tramo"
                    style={{ width: `${((f.valores[c.clave] ?? 0) / max) * 100}%`, background: c.color }}
                    title={`${c.nombre}: ${formato(f.valores[c.clave] ?? 0)}`}
                  />
                ))}
              </span>
              <span className="g-barra-valor">{formato(total)}</span>
              {f.extra && <span className="g-barra-extra">{f.extra}</span>}
            </li>
          )
        })}
      </ul>
    </Marco>
  )
}
