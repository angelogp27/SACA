// Motores de la Fase 6: con qué calcula cada laptop.
export const NOMBRE_MOTOR = { go: 'Go', openmp: 'OpenMP (C)' }

const todos = (nodos, f) => nodos.length > 0 && nodos.every(f)
export const hayOpenMP = (nodos) => todos(nodos, (n) => n.motores?.includes('openmp'))
export const hayCUDA = (nodos) => nodos.some((n) => n.motores?.includes('cuda'))
// MS-MPI y Open MPI no se comunican: todas con MPI y el mismo sistema.
export const hayMPI = (nodos) => todos(nodos, (n) => n.mpi) && new Set(nodos.map((n) => (n.hardware?.so ?? '').split('/')[0])).size === 1

// Lo que de verdad se puede usar con estos nodos, a partir de lo elegido.
export function efectivo(ajustes, nodos) {
  const omp = hayOpenMP(nodos)
  return {
    motores: !omp || ajustes.motores === 'go' ? ['go'] : ajustes.motores === 'openmp' ? ['openmp'] : ['openmp', 'go'],
    cuda: ajustes.cuda && hayCUDA(nodos),
    comunicacion: ajustes.comunicacion === 'mpi' && !hayMPI(nodos) ? 'arbol' : ajustes.comunicacion,
  }
}

// "openmp+cuda" → "OpenMP (C) + CUDA"
export const textoMotor = (m) => (m ?? '').split('+').map((x) => (x === 'cuda' ? 'CUDA' : NOMBRE_MOTOR[x] ?? x)).join(' + ')
