// Diagramas SVG de la presentación: los niveles de paralelismo del
// sistema, el flujo scatter → broadcast → reduce, la reducción en árbol de
// un bloque CUDA y el modelo de costo (Amdahl) del clúster.
import './Diagramas.css'

// Tres niveles: memoria distribuida (PCs), memoria compartida (hilos en
// una PC) y SIMT (hilos de una GPU).
export function NivelesParalelismo() {
  const nivel = (y, titulo, sub, tecnica, clase, hijos) => (
    <g>
      <rect x="10" y={y} width="620" height="92" rx="12" className={`dg-nivel ${clase}`} />
      <text x="28" y={y + 30} className="dg-titulo">{titulo}</text>
      <text x="28" y={y + 52} className="dg-sub">{sub}</text>
      <text x="28" y={y + 74} className="dg-tecnica">{tecnica}</text>
      {hijos}
    </g>
  )
  const cajitas = (y, n, ancho, etiqueta, clase) =>
    Array.from({ length: n }, (_, i) => (
      <g key={i}>
        <rect x={360 + i * (ancho + 8)} y={y} width={ancho} height="30" rx="5" className={`dg-cajita ${clase}`} />
        <text x={360 + i * (ancho + 8) + ancho / 2} y={y + 20} className="dg-cajita-texto">{typeof etiqueta === 'function' ? etiqueta(i) : etiqueta}</text>
      </g>
    ))
  return (
    <svg viewBox="0 0 640 310" className="diagrama-svg" role="img" aria-label="Tres niveles de paralelismo">
      {nivel(4, 'Memoria distribuida · MIMD', 'PCs en casas distintas, solo mensajes', 'TCP maestro-trabajador (sem. 3-4)', 'distribuida',
        cajitas(36, 4, 58, (i) => ['maestro', 'GPU', 'GPU', 'CPU'][i], 'pc'))}
      {nivel(110, 'Memoria compartida', 'Hilos de una PC sobre las mismas variables', 'goroutines + reducción (sem. 5-7)', 'compartida',
        cajitas(142, 6, 38, (i) => `h${i + 1}`, 'hilo'))}
      {nivel(216, 'SIMT · GPU', 'Miles de hilos, la misma instrucción', 'CUDA: bloques y memoria compartida (sem. 11-12)', 'simt',
        cajitas(248, 8, 26, '', 'warp'))}
    </svg>
  )
}

// Scatter → broadcast → cálculo local → reduce, con la barrera síncrona.
export function FlujoColectivas() {
  const nodos = [90, 230, 370]
  return (
    <svg viewBox="0 0 460 300" className="diagrama-svg" role="img" aria-label="Scatter, broadcast y reduce entre el maestro y tres nodos">
      <rect x="160" y="10" width="140" height="50" rx="10" className="dg-maestro" />
      <text x="230" y="33" className="dg-caja-titulo">Maestro</text>
      <text x="230" y="50" className="dg-caja-sub">w ← w − η·Σ∇/n</text>
      {nodos.map((x, i) => (
        <g key={x}>
          <line x1="230" y1="60" x2={x} y2="200" className="dg-flecha-baja" markerEnd="url(#dg-punta-baja)" />
          <line x1={x + 14} y1="200" x2="244" y2="62" className="dg-flecha-sube" markerEnd="url(#dg-punta-sube)" />
          <rect x={x - 55} y="200" width="110" height="56" rx="10" className="dg-nodo" />
          <text x={x} y="222" className="dg-caja-titulo">Nodo {i + 1}</text>
          <text x={x} y="241" className="dg-caja-sub">Σ ∇ℓ de su parte</text>
        </g>
      ))}
      <text x="78" y="120" className="dg-etiqueta baja">1 · scatter (partición)</text>
      <text x="78" y="138" className="dg-etiqueta baja">2 · broadcast (13 pesos)</text>
      <text x="382" y="120" className="dg-etiqueta sube">3 · reduce (Σ gradientes)</text>
      <line x1="20" y1="276" x2="440" y2="276" className="dg-barrera" />
      <text x="230" y="294" className="dg-caja-sub">barrera síncrona: la época termina cuando llegan todos</text>
      <defs>
        <marker id="dg-punta-baja" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto"><path d="M0,0 L10,5 L0,10 z" className="dg-punta baja" /></marker>
        <marker id="dg-punta-sube" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto"><path d="M0,0 L10,5 L0,10 z" className="dg-punta sube" /></marker>
      </defs>
    </svg>
  )
}

// Reducción en árbol de un bloque de 8 hilos en memoria compartida.
export function ArbolReduccion() {
  const niveles = [8, 4, 2, 1]
  const ancho = 420
  return (
    <svg viewBox={`0 0 ${ancho} 250`} className="diagrama-svg" role="img" aria-label="Reducción en árbol en memoria compartida">
      {niveles.map((n, k) => {
        const y = 20 + k * 58
        const paso = ancho / 8
        return (
          <g key={n}>
            {Array.from({ length: n }, (_, i) => {
              const x = paso * (i * (8 / n)) + (paso * (8 / n)) / 2
              return (
                <g key={i}>
                  {k > 0 && (
                    <>
                      <line x1={x - (paso * (8 / n)) / 4} y1={y - 30} x2={x} y2={y} className="dg-arista" />
                      <line x1={x + (paso * (8 / n)) / 4} y1={y - 30} x2={x} y2={y} className="dg-arista" />
                    </>
                  )}
                  <circle cx={x} cy={y + 8} r="13" className={k === 3 ? 'dg-raiz' : 'dg-hoja'} />
                  <text x={x} y={y + 12} className="dg-nodo-texto">{k === 0 ? `h${i}` : 'Σ'}</text>
                </g>
              )
            })}
            {k < 3 && <text x={ancho - 4} y={y + 40} className="dg-sync">__syncthreads()</text>}
          </g>
        )
      })}
    </svg>
  )
}

// Modelo de costo del clúster síncrono: S(p) = T₁ / (T₁/p + T_red), una
// curva por tamaño de muestra, con el ideal S = p como referencia.
export function ModeloCosto({ t1Prueba, t1Completa, tRed }) {
  const ancho = 520
  const alto = 260
  const pad = { izq: 44, der: 16, arriba: 14, abajo: 34 }
  const maxP = 32
  const ps = Array.from({ length: maxP }, (_, i) => i + 1)
  const s = (t1, p) => t1 / (t1 / p + tRed)
  const maxS = Math.ceil(Math.max(s(t1Completa, maxP), 4) / 4) * 4
  const x = (p) => pad.izq + ((p - 1) / (maxP - 1)) * (ancho - pad.izq - pad.der)
  const y = (v) => alto - pad.abajo - (Math.min(v, maxS) / maxS) * (alto - pad.arriba - pad.abajo)
  const ruta = (f) => ps.map((p, i) => `${i ? 'L' : 'M'} ${x(p)} ${y(f(p))}`).join(' ')
  const techo = (t1) => (t1 + tRed) / tRed
  return (
    <svg viewBox={`0 0 ${ancho} ${alto}`} className="diagrama-svg" role="img" aria-label="Speedup esperado según el tamaño del problema">
      {[0, 0.25, 0.5, 0.75, 1].map((f) => (
        <g key={f}>
          <line x1={pad.izq} x2={ancho - pad.der} y1={y(f * maxS)} y2={y(f * maxS)} className="dg-grilla" />
          <text x={pad.izq - 6} y={y(f * maxS)} className="dg-eje-y">{Math.round(f * maxS)}×</text>
        </g>
      ))}
      {[1, 8, 16, 24, 32].map((p) => (
        <text key={p} x={x(p)} y={alto - 16} className="dg-eje-x">{p}</text>
      ))}
      <text x={(ancho + pad.izq) / 2} y={alto - 2} className="dg-eje-x">nodos (p)</text>
      <path d={ruta((p) => p)} className="dg-ideal" />
      <path d={ruta((p) => s(t1Completa, p))} className="dg-curva completa" />
      <path d={ruta((p) => s(t1Prueba, p))} className="dg-curva prueba" />
      <text x={x(maxP) - 4} y={y(s(t1Completa, maxP)) - 8} className="dg-rotulo completa">todos los datos · techo {techo(t1Completa).toFixed(0)}×</text>
      <text x={x(maxP) - 4} y={y(s(t1Prueba, maxP)) - 8} className="dg-rotulo prueba">muestra de prueba · techo {techo(t1Prueba).toFixed(1)}×</text>
      <text x={x(9)} y={y(9) - 6} className="dg-rotulo ideal">ideal S = p</text>
    </svg>
  )
}

// GraficaSpeedup: S(p) medido de varias estrategias frente al ideal S = p.
export function GraficaSpeedup({ series, maxP }) {
  const ancho = 520
  const alto = 260
  const pad = { izq: 40, der: 16, arriba: 14, abajo: 34 }
  const tope = maxP ?? Math.max(...series.flatMap((s) => s.puntos.map((p) => p.p)), 2)
  const maxS = Math.max(tope, ...series.flatMap((s) => s.puntos.map((p) => p.s)))
  const x = (p) => pad.izq + ((p - 1) / (tope - 1)) * (ancho - pad.izq - pad.der)
  const y = (v) => alto - pad.abajo - (v / maxS) * (alto - pad.arriba - pad.abajo)
  const marcas = [...new Set(series.flatMap((s) => s.puntos.map((p) => p.p)))].sort((a, b) => a - b)
  return (
    <svg viewBox={`0 0 ${ancho} ${alto}`} className="diagrama-svg" role="img" aria-label="Speedup medido por estrategia">
      {[0, 0.5, 1].map((f) => (
        <g key={f}>
          <line x1={pad.izq} x2={ancho - pad.der} y1={y(f * maxS)} y2={y(f * maxS)} className="dg-grilla" />
          <text x={pad.izq - 6} y={y(f * maxS)} className="dg-eje-y">{(f * maxS).toFixed(0)}×</text>
        </g>
      ))}
      {marcas.map((p) => (
        <text key={p} x={x(p)} y={alto - 16} className="dg-eje-x">{p}</text>
      ))}
      <text x={(ancho + pad.izq) / 2} y={alto - 2} className="dg-eje-x">goroutines (p)</text>
      <path d={`M ${x(1)} ${y(1)} L ${x(tope)} ${y(tope)}`} className="dg-ideal" />
      <text x={x(tope) - 4} y={y(tope) + 14} className="dg-rotulo ideal" style={{ textAnchor: 'end' }}>ideal S = p</text>
      {series.map((s) => (
        <g key={s.nombre}>
          <path d={s.puntos.map((p, i) => `${i ? 'L' : 'M'} ${x(p.p)} ${y(p.s)}`).join(' ')} className="dg-curva" style={{ stroke: s.color }} />
          {s.puntos.map((p) => (
            <circle key={p.p} cx={x(p.p)} cy={y(p.s)} r="4" style={{ fill: s.color }} />
          ))}
          <text x={x(s.puntos.at(-1).p) - 6} y={y(s.puntos.at(-1).s) - 9} className="dg-rotulo" style={{ fill: s.color }}>{s.nombre}</text>
        </g>
      ))}
    </svg>
  )
}
