// Contenido de la presentación del avance (modo Presentación de la página).
// Las cifras salen de los JSON de public/datos cuando existen; los textos
// resumen docs/conceptos_curso.md y docs/guia_presentacion_cluster.md.

// Integrantes y PCs del clúster (se usan si el maestro no está en línea).
export const integrantes = [
  { id: 'piero', rol: 'Maestro', motor: 'maestro', dispositivo: 'Ryzen 5 3600 · Windows 11' },
  { id: 'augusto-pc', rol: 'Nodo', motor: 'cuda', dispositivo: 'NVIDIA RTX 3060 Ti · Ryzen 7 5700X' },
  { id: 'josue', rol: 'Nodo', motor: 'cuda', dispositivo: 'NVIDIA RTX 4060 Laptop · Ryzen 7 8845HS' },
  { id: 'angelo', rol: 'Nodo', motor: 'go', dispositivo: 'CPU Ryzen 7 7735HS (16 hilos) · Fedora' },
]

const f1 = (v, d = 1) => (Number.isFinite(v) ? v.toLocaleString('es-PE', { minimumFractionDigits: d, maximumFractionDigits: d }) : '—')

function mejor(series) {
  return (series ?? []).reduce((a, p) => (a && a.speedup >= p.speedup ? a : p), null)
}

function punto(exp, modo, unidades) {
  return exp?.series?.[modo]?.find((p) => p.unidades === unidades)
}

// Cifras clave de los experimentos, con respaldo si falta algún JSON.
export function cifras(datos) {
  const e1 = datos?.experimento1
  const e1b = datos?.experimento1b
  const e2 = datos?.experimento2
  const e2p = datos?.experimento2_prueba
  const particionado = mejor(e1?.series?.concurrente_particionado)
  const reduccion8 = punto(e1b, 'paralelo_reduccion', 8)
  const mutex8 = punto(e1b, 'paralelo_mutex', 8)
  const gpu1 = punto(e2, 'tcp_tailscale', 1)
  const gpu2 = punto(e2, 'tcp_tailscale', 2)
  const mixto3 = punto(e2, 'tcp_tailscale', 3)
  const prueba1 = punto(e2p, 'tcp_tailscale', 1)
  const pct = (p) => (p ? Math.round((100 * p.fases.t_comunicacion_ms) / (p.fases.t_calculo_ms + p.fases.t_comunicacion_ms)) : NaN)
  return {
    cargadorS: particionado ? `S(${particionado.unidades}) = ${f1(particionado.speedup, 2)}` : 'S(16) = 3,77',
    cargadorF: e1 ? f1(e1.fraccion_serial, 2) : '0,80',
    reduccionS: reduccion8 ? f1(reduccion8.speedup, 2) : '6,11',
    mutexE: mutex8 ? `${Math.round(mutex8.eficiencia * 100)} %` : '4-7 %',
    gpu1S: gpu1 ? f1(gpu1.speedup) : '18,2',
    gpu1T: gpu1 ? f1(gpu1.t_total_s, 2) : '2,28',
    gpu2S: gpu2 ? f1(gpu2.speedup) : '10,7',
    mixto3S: mixto3 ? f1(mixto3.speedup) : '6,1',
    mixto3Calc: mixto3 ? f1(mixto3.fases.t_calculo_ms) : '18,0',
    t1: e2 ? f1(e2.t1_s) : '41,5',
    t1Epoca: e2 ? f1(e2.fases_base.t_epoca_ms, 0) : '207',
    pctPrueba: prueba1 ? `${pct(prueba1)} %` : '72 %',
    pctGpu1: gpu1 ? `${pct(gpu1)} %` : '73 %',
    pctMixto: mixto3 ? `${pct(mixto3)} %` : '47 %',
    perdida: e2 ? String(e2.fases_base.perdida_final) : '0.132576',
  }
}

// Temas del sílabo aplicados: unidad, tema, dónde está, evidencia y estado
// (hecho, go = hecho en Go con su núcleo nativo pendiente, pendiente).
export function temasCurso(datos) {
  const c = cifras(datos)
  return [
    {
      unidad: 'I · Fundamentos',
      temas: [
        { tema: 'Clasificación de computadores paralelos (sem. 2)', donde: 'El clúster del grupo: multicomputador MIMD de memoria distribuida y heterogéneo (CPU + GPU)', evidencia: '4 PCs en 4 casas, 2 GPUs NVIDIA + 1 CPU, unidas por Tailscale', estado: 'hecho' },
        { tema: 'Métricas: speedup, eficiencia, Amdahl, Karp-Flatt (sem. 1 y 13)', donde: 'experimentos/cmd/analisis, mediana de 3 repeticiones', evidencia: `Cargador: f = ${c.cargadorF} medida → techo de Amdahl; particionado ${c.cargadorS}`, estado: 'hecho' },
        { tema: 'Memoria compartida frente a distribuida', donde: 'Cargador y entrenamiento con goroutines frente al clúster por TCP', evidencia: `Memoria compartida S(8) = ${c.reduccionS}; distribuido 1 GPU S = ${c.gpu1S}`, estado: 'hecho' },
      ],
    },
    {
      unidad: 'II · Memoria compartida y paso de mensajes',
      temas: [
        { tema: 'Paso de mensajes: envío y recepción (sem. 3)', donde: 'Protocolo maestro-trabajador por TCP (6 mensajes JSON)', evidencia: 'Entrenamiento real entre casas; misma pérdida que el secuencial (~1e-15)', estado: 'go' },
        { tema: 'Colectivas: scatter, broadcast, reduce (sem. 4)', donde: 'Partición por estación, difusión de pesos, suma de gradientes', evidencia: 'Cada época: 13 números de ida y 14 de vuelta por nodo', estado: 'go' },
        { tema: 'Hilos, mutex y exclusión mutua (sem. 5)', donde: 'Sala de nodos del maestro (sync.Mutex); acumulador con mutex', evidencia: `Mutex por ejemplo: eficiencia ${c.mutexE} con 8 goroutines`, estado: 'go' },
        { tema: 'Productor-consumidor y barreras (sem. 6)', donde: 'Cargador: channels con buffer; barrera síncrona por época', evidencia: 'La época termina cuando llega el nodo más lento', estado: 'go' },
        { tema: 'Reducciones, variables privadas (sem. 7, OpenMP)', donde: 'Gradiente con casillas privadas por goroutine', evidencia: `Reducción S(8) = ${c.reduccionS} frente a mutex y CAS más lentos que el secuencial`, estado: 'go' },
      ],
    },
    {
      unidad: 'III · Tareas, balanceo y GPU',
      temas: [
        { tema: 'Descomposición de datos y balanceo de carga (sem. 9-10)', donde: 'Partición por hash de estación; nodos heterogéneos', evidencia: `Con un nodo CPU, cada época espera ${c.mixto3Calc} ms al más lento: S baja de ${c.gpu1S} a ${c.mixto3S}`, estado: 'hecho' },
        { tema: 'CUDA: kernels, memoria global y compartida, reducción (sem. 11-12)', donde: 'nativo/cuda/gradiente_servidor.cu, validado contra la CPU', evidencia: `RTX 3060 Ti remota: ${c.gpu1T} s frente a ${c.t1} s del secuencial (S = ${c.gpu1S})`, estado: 'hecho' },
        { tema: 'Warp shuffles, sorting y MapReduce en GPU (sem. 12-13)', donde: 'Experimento 8 (kernels shuffle) y Thrust', evidencia: '—', estado: 'pendiente' },
      ],
    },
    {
      unidad: 'IV · Evaluación y diseño',
      temas: [
        { tema: 'Evaluación empírica de la aceleración (sem. 13)', donde: 'Experimentos 1, 1b y 2 con el mismo análisis', evidencia: `Con pocos datos ${c.pctPrueba} de red; con todos, el clúster gana`, estado: 'hecho' },
        { tema: 'MPI, Pthreads y OpenMP nativos', donde: 'nativo/ (C/C++), con el formato binario común', evidencia: '—', estado: 'pendiente' },
      ],
    },
  ]
}

export const etiquetaEstado = { hecho: 'Hecho', go: 'Hecho en Go · nativo pendiente', pendiente: 'Pendiente' }
