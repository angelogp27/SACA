/* Motor OpenMP de la red neuronal (semanas 7, 9 y 10): el mismo gradiente
 * que internal/red/red.go, para compararlo con Go en el clúster. Lo lanza el
 * trabajador Go; el protocolo está en nativo/comun/motor_red.h.
 *
 *   - parallel + for schedule(runtime): reparte los ejemplos del mini-lote
 *     entre los hilos (OMP_SCHEDULE; static si no se dice otra cosa);
 *   - cada hilo acumula en su propia copia del gradiente y al final un
 *     segundo `for` las suma por componentes: es la cláusula reduction
 *     hecha a mano, porque reduction sobre un arreglo lo pone en la pila
 *     de cada hilo y una red grande no cabe;
 *   - simd en los productos: varias operaciones por instrucción.
 *
 * Compilar:  gcc -O3 -march=native -fopenmp -o bin/red_openmp nativo/openmp/red/red_openmp.c -lm
 * Uso:       bin/red_openmp [--hilos N] [--info]
 */
#include <math.h>
#include <omp.h>

#include "../../comun/motor_red.h"

/* Memoria de un hilo; se conserva entre mini-lotes. */
typedef struct {
    double *g, *a, *delta;
    double perdida;
    size_t np, na;
} Trabajo;

static void preparar(Trabajo *t, const MrModelo *m) {
    if (t->np < (size_t)m->nparams) {
        free(t->g);
        t->g = (double *)mr_reservar((size_t)m->nparams * sizeof(double));
        t->np = (size_t)m->nparams;
    }
    if (t->na < (size_t)m->nactiv) {
        free(t->a);
        free(t->delta);
        t->a = (double *)mr_reservar((size_t)m->nactiv * sizeof(double));
        t->delta = (double *)mr_reservar((size_t)m->nactiv * sizeof(double));
        t->na = (size_t)m->nactiv;
    }
}

static double adelante(const MrModelo *m, const double *x, double *a) {
    memcpy(a, x, (size_t)m->capas[0] * sizeof(double));
    for (int l = 1; l < m->n; l++) {
        const int in = m->capas[l - 1], out = m->capas[l];
        const double *w = m->p + m->inicio[l], *b = w + in * out;
        const double *prev = a + m->activ[l - 1];
        double *act = a + m->activ[l];
        for (int o = 0; o < out; o++) {
            double z = 0;
            const double *fila = w + o * in;
#pragma omp simd reduction(+ : z)
            for (int i = 0; i < in; i++) z += fila[i] * prev[i];
            z += b[o];
            if (l == m->n - 1) act[o] = 1.0 / (1.0 + exp(-z));
            else act[o] = z > 0 ? z : 0;
        }
    }
    return a[m->activ[m->n - 1]];
}

static double atras(const MrModelo *m, double prob, double y, double peso, Trabajo *t) {
    const double pr = fmin(fmax(prob, 1e-12), 1 - 1e-12);
    const int L = m->n - 1;
    t->delta[m->activ[L]] = peso * (prob - y);
    for (int l = L; l >= 1; l--) {
        const int in = m->capas[l - 1], out = m->capas[l];
        const double *w = m->p + m->inicio[l];
        double *restrict gw = t->g + m->inicio[l];
        double *gb = gw + in * out;
        const double *prev = t->a + m->activ[l - 1], *delta = t->delta + m->activ[l];
        double *restrict dprev = t->delta + m->activ[l - 1];
        if (l > 1) memset(dprev, 0, (size_t)in * sizeof(double));
        for (int o = 0; o < out; o++) {
            const double d = delta[o];
            if (d == 0) continue;
            gb[o] += d;
            double *restrict fila = gw + o * in;
#pragma omp simd
            for (int j = 0; j < in; j++) fila[j] += d * prev[j];
            if (l > 1) {
                const double *wf = w + o * in;
#pragma omp simd
                for (int j = 0; j < in; j++) dprev[j] += d * wf[j];
            }
        }
        if (l > 1)
            for (int j = 0; j < in; j++)
                if (prev[j] <= 0) dprev[j] = 0; /* derivada de ReLU */
    }
    return -peso * (y * log(pr) + (1 - y) * log(1 - pr));
}

int main(int argc, char **argv) {
    mr_binario();
    int hilos = 0, info = 0;
    for (int i = 1; i < argc; i++) {
        if (!strcmp(argv[i], "--hilos") && i + 1 < argc) hilos = atoi(argv[++i]);
        else if (!strcmp(argv[i], "--info")) info = 1;
    }
    if (hilos < 1) hilos = omp_get_num_procs();
    omp_set_dynamic(0);
    omp_set_num_threads(hilos);
    if (!getenv("OMP_SCHEDULE")) omp_set_schedule(omp_sched_static, 0);
    char nombre[64];
    snprintf(nombre, sizeof nombre, "OpenMP, %d hilos", hilos);
    if (info) {
        printf("%s\n", nombre);
        return 0;
    }
    mr_presentarse(nombre);

    Trabajo *ts = (Trabajo *)calloc((size_t)hilos, sizeof(Trabajo));
    MrDatos D;
    MrModelo m;
    memset(&D, 0, sizeof D);
    memset(&m, 0, sizeof m);
    double *res = NULL;
    size_t nres = 0;
    uint32_t orden;
    while (mr_leer(&orden, 4) && orden != MR_TERMINAR) {
        if (orden == MR_DATOS) {
            mr_leer_datos(&D);
            mr_estado(0);
        } else if (orden == MR_PARTICION) {
            free(D.idx);
            D.idx = mr_leer_indices(&D, &D.cuantos);
            mr_estado(0);
        } else if (orden == MR_GRADIENTE) {
            uint32_t ini, fin;
            mr_leer_modelo(&m, D.d);
            mr_exigir(&ini, 4);
            mr_exigir(&fin, 4);
            if (ini > fin || fin > D.cuantos) mr_fallar("mini-lote fuera de la partición");
            if (nres < (size_t)m.nparams + 1) {
                free(res);
                nres = (size_t)m.nparams + 1;
                res = (double *)mr_reservar(nres * sizeof(double));
            }
            double perdida = 0;
#pragma omp parallel reduction(+ : perdida)
            {
                const int yo = omp_get_thread_num(), equipo = omp_get_num_threads();
                Trabajo *t = &ts[yo];
                preparar(t, &m);
                memset(t->g, 0, (size_t)m.nparams * sizeof(double));
#pragma omp for schedule(runtime)
                for (int64_t k = ini; k < (int64_t)fin; k++) {
                    const uint64_t f = D.idx[k];
                    const double y = D.y[f];
                    const double prob = adelante(&m, D.X + f * D.d, t->a);
                    perdida += atras(&m, prob, y, y == 1 ? D.pc[1] : D.pc[0], t);
                } /* barrera implícita: todas las copias están listas */
#pragma omp for schedule(static)
                for (int j = 0; j < m.nparams; j++) {
                    double s = 0;
                    for (int h = 0; h < equipo; h++) s += ts[h].g[j];
                    res[j] = s;
                }
            }
            res[m.nparams] = perdida;
            mr_escribir(res, ((size_t)m.nparams + 1) * sizeof(double));
            fflush(stdout);
        } else if (orden == MR_PREDECIR) {
            uint32_t n;
            mr_leer_modelo(&m, D.d);
            uint32_t *idx = mr_leer_indices(&D, &n);
            double *prob = (double *)mr_reservar((size_t)n * sizeof(double));
#pragma omp parallel
            {
                Trabajo *t = &ts[omp_get_thread_num()];
                preparar(t, &m);
#pragma omp for schedule(runtime)
                for (int64_t k = 0; k < (int64_t)n; k++) prob[k] = adelante(&m, D.X + (uint64_t)idx[k] * D.d, t->a);
            }
            mr_escribir(prob, (size_t)n * sizeof(double));
            fflush(stdout);
            free(prob);
            free(idx);
        } else {
            mr_fallar("orden desconocida");
        }
    }
    return 0;
}
