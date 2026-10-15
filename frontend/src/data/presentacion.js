// Contenido de la presentación del avance (modo Presentación de la página):
// títulos, notas del orador, tarjetas de conceptos del curso y las cifras,
// que salen de los JSON de public/datos cuando existen. El guion completo
// está en docs/guion_exposicion.md.

export const integrantes = [
  { id: 'piero', rol: 'Maestro', motor: 'maestro', dispositivo: 'Ryzen 5 3600 · Windows 11' },
  { id: 'augusto-pc', rol: 'Nodo', motor: 'cuda', dispositivo: 'NVIDIA RTX 3060 Ti' },
  { id: 'josue', rol: 'Nodo', motor: 'cuda', dispositivo: 'NVIDIA RTX 4060 Laptop' },
  { id: 'angelo', rol: 'Nodo', motor: 'go', dispositivo: 'CPU Ryzen 7 7735HS · 16 hilos' },
]

const f1 = (v, d = 1) => (Number.isFinite(v) ? v.toLocaleString('es-PE', { minimumFractionDigits: d, maximumFractionDigits: d }) : '—')

function punto(exp, modo, unidades) {
  return exp?.series?.[modo]?.find((p) => p.unidades === unidades)
}

// combinacion busca el punto de una combinación exacta de nodos
// ("tcp_tailscale:id1+id2" en orden alfabético) en los resultados nuevos y,
// si no está, en los anteriores (agrupados solo por cantidad de nodos).
function combinacion(datos, ids) {
  const clave = `tcp_tailscale:${[...ids].sort().join('+')}`
  return (
    datos?.experimento2?.series?.[clave]?.[0] ??
    datos?.experimento2_anterior?.series?.[clave]?.[0] ??
    punto(datos?.experimento2_anterior, 'tcp_tailscale', ids.length)
  )
}

const pctRed = (p) => (p ? (100 * p.fases.t_comunicacion_ms) / (p.fases.t_calculo_ms + p.fases.t_comunicacion_ms) : NaN)

// Cifras clave de los experimentos, con respaldo si falta algún JSON.
export function cifras(datos) {
  const e1 = datos?.experimento1
  const e1b = datos?.experimento1b
  const base2 = (datos?.experimento2 ?? datos?.experimento2_anterior)?.fases_base
  const e2p = datos?.experimento2_prueba
  const particionado = (e1?.series?.concurrente_particionado ?? []).reduce((a, p) => (a && a.speedup >= p.speedup ? a : p), null)
  const reduccion8 = punto(e1b, 'paralelo_reduccion', 8)
  const mutex8 = punto(e1b, 'paralelo_mutex', 8)
  const atomico8 = punto(e1b, 'paralelo_atomico', 8)
  const gpu1 = combinacion(datos, ['augusto-pc'])
  const gpu2 = combinacion(datos, ['augusto-pc', 'josue'])
  const mixto3 = combinacion(datos, ['angelo', 'augusto-pc', 'josue'])
  const prueba1 = punto(e2p, 'tcp_tailscale', 1)
  return {
    filas: datos?.resumen?.filas_crudas ?? 16972287,
    registros: datos?.resumen?.registros_unicos ?? 10838667,
    estaciones: datos?.resumen?.estaciones ?? 1447,
    auc: datos?.resumen?.modelo?.auc ?? 0.995,
    cargadorS: particionado ? f1(particionado.speedup, 2) : '3,77',
    cargadorP: particionado?.unidades ?? 16,
    cargadorF: e1 ? f1(e1.fraccion_serial, 2) : '0,80',
    reduccionS: reduccion8 ? f1(reduccion8.speedup, 2) : '6,11',
    mutexS: mutex8 ? f1(mutex8.speedup, 2) : '0,46',
    atomicoS: atomico8 ? f1(atomico8.speedup, 2) : '0,57',
    gpu1S: gpu1 ? f1(gpu1.speedup) : '18,2',
    gpu1T: gpu1 ? f1(gpu1.t_total_s, 2) : '2,28',
    gpu1Calc: gpu1 ? f1(gpu1.fases.t_calculo_ms) : '3,1',
    gpu2S: gpu2 ? f1(gpu2.speedup) : '9,1',
    mixto3S: mixto3 ? f1(mixto3.speedup) : '6,0',
    mixto3Calc: mixto3 ? f1(mixto3.fases.t_calculo_ms) : '18,1',
    t1: base2 ? f1(base2.t_total_s) : '46,8',
    t1Epoca: base2 ? f1(base2.t_epoca_ms, 0) : '234',
    t1EpocaN: base2?.t_epoca_ms ?? 234,
    t1EpocaPruebaN: e2p?.fases_base?.t_epoca_ms ?? 13.3,
    redN: prueba1?.fases?.t_comunicacion_ms ?? 7.4,
    pctPrueba: prueba1 ? `${Math.round(pctRed(prueba1))} %` : '73 %',
    pctGpu1: gpu1 ? `${Math.round(pctRed(gpu1))} %` : '73 %',
    perdida: base2 ? String(base2.perdida_final) : '0.132576',
  }
}

// Diapositivas: sección, título (una afirmación), etiqueta del sílabo y
// notas del orador (resumen del guion; tecla N para verlas).
export const diapositivas = [
  {
    seccion: 'Portada',
    titulo: 'SACA: alerta temprana de contaminación, entrenada en un clúster heterogéneo',
    notas: 'Presentarse. "SACA predice si el NO₂ superará el umbral de alerta en la próxima hora, y lo entrena en paralelo en tres niveles: hilos, varias PCs y GPU." 20 s.',
  },
  {
    seccion: 'El problema',
    titulo: 'Anticipar el NO₂ exige procesar 17 millones de mediciones',
    tema: 'Motivación del paralelismo',
    notas: 'Señalar el mapa: 1447 estaciones. La pregunta es predictiva (clase 1 si NO₂ > 53 ppb en t+1). El volumen (17 M filas, 10,8 M registros) justifica paralelizar ingesta y entrenamiento. 40 s.',
  },
  {
    seccion: 'Arquitectura',
    titulo: 'Tres niveles de paralelismo en un mismo sistema',
    tema: 'Sem. 2 · Flynn y modelos de memoria',
    notas: 'Recorrer de arriba abajo: memoria distribuida (PCs que solo se envían mensajes por TCP), memoria compartida (goroutines dentro de una PC) y SIMT (miles de hilos CUDA). El clúster es MIMD; cada GPU es SIMT. 50 s.',
  },
  {
    seccion: 'Memoria compartida',
    titulo: 'En memoria compartida, la sincronización puede costar más que el cálculo',
    tema: 'Sem. 5-7 · Mutex, CAS, reducción',
    notas: 'El gradiente se suma en un acumulador. Con mutex o CAS por ejemplo hay contención: más lento que el secuencial. Con una reducción (cada goroutine suma en privado y al final se combinan) el speedup sube a ~6 con 8 hilos. Es el reduction(+) de OpenMP. 60 s.',
  },
  {
    seccion: 'Distribución',
    titulo: 'Repartir el gradiente da exactamente el mismo modelo',
    tema: 'Sem. 3-4 · Paso de mensajes · Sem. 10 · Descomposición',
    notas: 'El gradiente es una suma sobre ejemplos: se reparte por estaciones (scatter), el maestro difunde los 13 pesos (broadcast), cada nodo devuelve su suma parcial y el maestro las suma (reduce) tras una barrera. La pérdida final coincide con la secuencial hasta 1e-15. 60 s.',
  },
  {
    seccion: 'El clúster',
    titulo: 'Un clúster real: 4 PCs en 4 casas, con CPU y GPU',
    tema: 'Sem. 11-12 · CUDA: bloques, memoria compartida, reducción',
    notas: 'Los nodos del diagrama son los conectados ahora. En la GPU, cada hilo procesa ejemplos, cada bloque reduce en memoria compartida con un árbol (__syncthreads entre niveles) y la CPU suma los bloques. Antes de usarla, la GPU se valida contra la CPU. 50 s.',
  },
  {
    seccion: 'Demo',
    titulo: 'Demo en vivo: elijo los nodos y entreno',
    tema: 'Todo el curso en vivo',
    notas: 'Marcar augusto → Entrenar (señalar speedup ~18× y la franja de conceptos). Marcar augusto + josue → más lento: más red. Marcar los tres → la línea de angelo arriba: marca el ritmo. Bajar a "Corridas de esta sesión". 3 min máximo.',
  },
  {
    seccion: 'Resultados',
    titulo: 'Distribuir conviene solo si el cálculo domina a la red',
    tema: 'Sem. 1 y 13 · Amdahl y evaluación empírica',
    notas: 'Modelo: T(p) = T_cálculo/p + T_red. Con la muestra de prueba (~13 ms de cálculo, ~7 ms de red) el techo es ~2×. Con todos los datos (~230 ms de cálculo) el techo sube a ~30×. Amdahl con la red como fracción serial; Gustafson: crecer el problema mejora el speedup. 60 s.',
  },
  {
    seccion: 'Conclusiones',
    titulo: 'El nodo más lento marca el ritmo del clúster',
    tema: 'Sem. 10 · Balanceo de carga',
    notas: 'Tres conclusiones con su cifra. Cerrar con el próximo paso: reparto proporcional al rendimiento, para que la GPU reciba más datos que la CPU. 45 s.',
  },
  {
    seccion: 'Temas del curso',
    titulo: 'Lo que el curso enseña, aplicado y medido en SACA',
    tema: 'Unidades I a IV',
    notas: 'No leer la tabla: señalar que cada tema tiene una evidencia medida. Lo pendiente (MPI, Pthreads, OpenMP nativos, warp shuffles) ya tiene contrato y datos listos. Cerrar: "Gracias, ¿preguntas?" 30 s.',
  },
]

// Tarjetas "Concepto del curso": definición, fórmula y cómo se aplica.
export const conceptos = {
  flynn: {
    titulo: 'Taxonomía de Flynn y modelos de memoria',
    definicion: 'MIMD: varios procesadores ejecutan instrucciones distintas sobre datos distintos. SIMD/SIMT: muchos hilos ejecutan la misma instrucción sobre datos distintos.',
    enSaca: 'El clúster es MIMD de memoria distribuida y heterogéneo; dentro de cada PC hay memoria compartida, y cada GPU es SIMT.',
  },
  speedup: {
    titulo: 'Speedup, eficiencia y sección crítica',
    formula: 'S(p) = T₁ / Tₚ      E(p) = S(p) / p',
    definicion: 'Una condición de carrera ocurre cuando dos hilos leen-modifican-escriben la misma variable sin exclusión mutua. Mutex y CAS la evitan, pero serializan el acceso.',
    enSaca: 'Mismo cálculo, tres estrategias: la reducción con casillas privadas evita la sección crítica y es la única que escala.',
  },
  colectivas: {
    titulo: 'Paso de mensajes y colectivas',
    formula: '∇L = Σᵢ ∇ℓᵢ = Σ(nodo 1) + Σ(nodo 2) + … + Σ(nodo p)',
    definicion: 'Scatter reparte datos, broadcast difunde un valor a todos, reduce combina valores con una operación asociativa. Una barrera espera a que lleguen todos.',
    enSaca: 'Scatter = partición por hash de estación; broadcast = 13 pesos por época; reduce = suma de gradientes en el maestro. Equivale a MPI_Bcast + MPI_Allreduce.',
  },
  cuda: {
    titulo: 'Modelo de programación CUDA',
    formula: 'grid → bloques → warps de 32 hilos → hilos',
    definicion: 'Los hilos de un bloque comparten memoria rápida (__shared__) y se sincronizan con __syncthreads(). La reducción en árbol suma n valores en log₂ n pasos.',
    enSaca: 'Hilos con grid-stride sobre los ejemplos, reducción en memoria compartida por bloque y suma final de bloques en la CPU: resultado determinista sin atomicAdd.',
  },
  amdahl: {
    titulo: 'Ley de Amdahl y de Gustafson',
    formula: 'S(p) = 1 / (f + (1 − f)/p)      con f = T_red / (T_cálculo + T_red)',
    definicion: 'La parte no paralelizable f limita el speedup a 1/f. Gustafson: si el problema crece con los recursos, la fracción serial pesa menos.',
    enSaca: 'En el clúster la parte serial es la comunicación síncrona por época (13 números, ~7 ms por internet). Con más datos, f baja y el clúster gana.',
  },
  balanceo: {
    titulo: 'Balanceo de carga',
    definicion: 'En un algoritmo síncrono, cada paso dura lo que tarda el procesador más lento. El balanceo reparte el trabajo según la capacidad de cada uno.',
    enSaca: 'La partición por hash da partes iguales en datos, pero no en tiempo: la CPU tarda mucho más que la GPU y marca el ritmo.',
  },
}

// Temas del sílabo aplicados: unidad, tema, evidencia medida y estado
// (hecho, go = hecho en Go con su núcleo nativo pendiente, pendiente).
export function temasCurso(datos) {
  const c = cifras(datos)
  return [
    { unidad: 'I', tema: 'Flynn, memoria compartida y distribuida', evidencia: 'Clúster MIMD heterogéneo: 4 PCs, 2 GPU + 1 CPU', estado: 'hecho' },
    { unidad: 'I', tema: 'Speedup, eficiencia, Amdahl, Karp-Flatt', evidencia: `Cargador: f = ${c.cargadorF} medida; S(${c.cargadorP}) = ${c.cargadorS}`, estado: 'hecho' },
    { unidad: 'II', tema: 'Paso de mensajes y colectivas (sem. 3-4)', evidencia: 'Scatter, broadcast y reduce por TCP; misma pérdida que el secuencial', estado: 'go' },
    { unidad: 'II', tema: 'Hilos, mutex, CAS, barreras (sem. 5-6)', evidencia: `Mutex S(8) = ${c.mutexS}, CAS S(8) = ${c.atomicoS}`, estado: 'go' },
    { unidad: 'II', tema: 'Reducciones (sem. 7, OpenMP)', evidencia: `Reducción privada S(8) = ${c.reduccionS}`, estado: 'go' },
    { unidad: 'III', tema: 'Descomposición y balanceo (sem. 9-10)', evidencia: `El nodo CPU frena el clúster: S ${c.gpu1S} → ${c.mixto3S}`, estado: 'hecho' },
    { unidad: 'III', tema: 'CUDA: kernels, memoria compartida (sem. 11-12)', evidencia: `GPU remota S = ${c.gpu1S} (${c.gpu1T} s frente a ${c.t1} s)`, estado: 'hecho' },
    { unidad: 'IV', tema: 'Evaluación empírica (sem. 13)', evidencia: `Red: ${c.pctPrueba} con pocos datos; el clúster gana con todos`, estado: 'hecho' },
    { unidad: '—', tema: 'MPI, Pthreads, OpenMP nativos; warp shuffles', evidencia: 'Contrato binario y datos listos', estado: 'pendiente' },
  ]
}

export const etiquetaEstado = { hecho: 'Hecho', go: 'Hecho en Go · nativo pendiente', pendiente: 'Pendiente' }
