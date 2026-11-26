/* Lo común de los motores externos de la red neuronal (OpenMP y CUDA): el
 * protocolo binario con el trabajador Go por stdin/stdout, definido en
 * internal/distribuido/motor_red.go. Todo little-endian, reales float64:
 *
 *   motor -> Go, al arrancar:  "SACR" | u32 largo | nombre del dispositivo
 *   orden 1 (datos):      u32 d | u64 n | f64 pc0, pc1 | X[n*d] | y[n]         -> u32 estado
 *   orden 2 (partición):  u32 cuantos | u32 idx[cuantos]                       -> u32 estado
 *   orden 3 (gradiente):  modelo | u32 ini | u32 fin                -> f64 grad[P] | f64 pérdida
 *   orden 4 (predecir):   modelo | u32 cuantos | u32 idx[cuantos]   -> f64 prob[cuantos]
 *   orden 0 (terminar)
 *   modelo = u32 ncapas | u32 capas[ncapas] | f64 params[P]
 *
 * Los datos viajan una sola vez; la partición (los índices de las filas que
 * le tocan al nodo) cambia con el reparto, y cada mini-lote es un tramo
 * [ini, fin) de ella. Una regresión logística es la red de capas {d, 1}.
 */
#ifndef SACA_MOTOR_RED_H
#define SACA_MOTOR_RED_H
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif

#define MR_MAX_CAPAS 8
enum { MR_TERMINAR = 0, MR_DATOS = 1, MR_PARTICION = 2, MR_GRADIENTE = 3, MR_PREDECIR = 4 };

static void mr_fallar(const char *que) {
    fprintf(stderr, "motor: %s\n", que);
    exit(3);
}
static int mr_leer(void *dst, size_t bytes) { return bytes == 0 || fread(dst, 1, bytes, stdin) == bytes; }
static void mr_exigir(void *dst, size_t bytes) {
    if (!mr_leer(dst, bytes)) mr_fallar("el trabajador cerró la conexión a mitad de un mensaje");
}
static void mr_escribir(const void *src, size_t bytes) {
    if (bytes && fwrite(src, 1, bytes, stdout) != bytes) mr_fallar("no se pudo escribir en stdout");
}
static void mr_estado(uint32_t e) {
    mr_escribir(&e, 4);
    fflush(stdout);
}
static void *mr_reservar(size_t bytes) {
    void *p = malloc(bytes ? bytes : 1);
    if (!p) mr_fallar("sin memoria");
    return p;
}
static void mr_binario(void) {
#ifdef _WIN32
    /* Sin esto, Windows traduce \n a \r\n y corrompe el protocolo. */
    _setmode(_fileno(stdin), _O_BINARY);
    _setmode(_fileno(stdout), _O_BINARY);
#endif
}
static void mr_presentarse(const char *nombre) {
    uint32_t largo = (uint32_t)strlen(nombre);
    mr_escribir("SACR", 4);
    mr_escribir(&largo, 4);
    mr_escribir(nombre, largo);
    fflush(stdout);
}

typedef struct {
    int capas[MR_MAX_CAPAS], n; /* n = número de capas, con la entrada */
    int inicio[MR_MAX_CAPAS];   /* dónde empiezan los pesos de cada capa */
    int activ[MR_MAX_CAPAS];    /* dónde empiezan las activaciones de cada capa */
    int nparams, nactiv;
    double *p;
    size_t reservado;
} MrModelo;

static void mr_leer_modelo(MrModelo *m, uint32_t d) {
    uint32_t n, c;
    mr_exigir(&n, 4);
    if (n < 2 || n > MR_MAX_CAPAS) mr_fallar("número de capas inválido");
    m->n = (int)n;
    m->nparams = m->nactiv = 0;
    for (int l = 0; l < m->n; l++) {
        mr_exigir(&c, 4);
        if (c < 1 || c > 4096) mr_fallar("capa inválida");
        m->capas[l] = (int)c;
        m->activ[l] = m->nactiv;
        m->nactiv += (int)c;
        m->inicio[l] = m->nparams;
        if (l > 0) m->nparams += m->capas[l] * m->capas[l - 1] + m->capas[l];
    }
    if ((uint32_t)m->capas[0] != d || m->capas[m->n - 1] != 1) mr_fallar("la red no corresponde a los datos");
    size_t bytes = (size_t)m->nparams * sizeof(double);
    if (bytes > m->reservado) {
        free(m->p);
        m->p = (double *)mr_reservar(bytes);
        m->reservado = bytes;
    }
    mr_exigir(m->p, bytes);
}

typedef struct {
    uint32_t d;
    uint64_t n;
    double pc[2];
    double *X, *y;
    uint32_t *idx, cuantos;
} MrDatos;

/* Lee la orden 1 (sin responder el estado). */
static void mr_leer_datos(MrDatos *D) {
    mr_exigir(&D->d, 4);
    mr_exigir(&D->n, 8);
    mr_exigir(D->pc, sizeof D->pc);
    free(D->X);
    free(D->y);
    D->X = (double *)mr_reservar((size_t)D->n * D->d * sizeof(double));
    D->y = (double *)mr_reservar((size_t)D->n * sizeof(double));
    mr_exigir(D->X, (size_t)D->n * D->d * sizeof(double));
    mr_exigir(D->y, (size_t)D->n * sizeof(double));
    D->cuantos = 0;
}

static uint32_t *mr_leer_indices(const MrDatos *D, uint32_t *cuantos) {
    mr_exigir(cuantos, 4);
    uint32_t *v = (uint32_t *)mr_reservar((size_t)*cuantos * 4);
    mr_exigir(v, (size_t)*cuantos * 4);
    for (uint32_t i = 0; i < *cuantos; i++)
        if (v[i] >= D->n) mr_fallar("índice de fila fuera de los datos");
    return v;
}
#endif
