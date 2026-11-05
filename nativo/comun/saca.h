/* Utilidades comunes de los núcleos nativos de SACA (Pthreads, OpenMP, MPI).
 *
 * Lee la muestra binaria de docs/contrato_formato_binario.md (archivo 1),
 * mide tiempos, evalúa el modelo (F1 y AUC igual que internal/metricas),
 * compara contra la referencia de Go y agrega filas a los CSV de resultados.
 * Es un solo encabezado con funciones static: cada programa lo incluye y no
 * hay nada que enlazar. Se compila con -lm.
 */
#ifndef SACA_H
#define SACA_H

#if !defined(_WIN32) && !defined(_POSIX_C_SOURCE)
#define _POSIX_C_SOURCE 200809L
#endif
#include <math.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

/* Las funciones son static y cada programa usa solo algunas. */
#if defined(__GNUC__) || defined(__clang__)
#define SACA_FN static __attribute__((unused))
#else
#define SACA_FN static
#endif

typedef struct {
    uint32_t d;
    uint64_t n_ent, n_pru;
    double pc0, pc1;         /* pesos de clase */
    double *Xe, *ye;         /* entrenamiento: n_ent x d (por filas) y n_ent */
    double *Xp, *yp;         /* prueba */
} Muestra;

#ifdef _WIN32
#include <windows.h>
SACA_FN double saca_ahora(void) {
    LARGE_INTEGER f, c;
    QueryPerformanceFrequency(&f);
    QueryPerformanceCounter(&c);
    return (double)c.QuadPart / (double)f.QuadPart;
}
#else
SACA_FN double saca_ahora(void) {
    struct timespec t;
    clock_gettime(CLOCK_MONOTONIC, &t);
    return t.tv_sec + t.tv_nsec * 1e-9;
}
#endif

SACA_FN void saca_fallar(const char *msg, const char *detalle) {
    fprintf(stderr, "saca: %s%s%s\n", msg, detalle ? ": " : "", detalle ? detalle : "");
    exit(2);
}

SACA_FN void saca_leer(FILE *f, void *dst, size_t n, const char *que) {
    if (n && fread(dst, 8, n, f) != n) saca_fallar("archivo incompleto leyendo", que);
}

/* Carga data/muestra.bin completo (entrenamiento y prueba). */
SACA_FN void saca_cargar(const char *ruta, Muestra *m) {
    FILE *f = fopen(ruta, "rb");
    if (!f) saca_fallar("no se pudo abrir", ruta);
    unsigned char enc[48];
    if (fread(enc, 1, 48, f) != 48 || memcmp(enc, "SACA", 4) != 0) saca_fallar("no es un archivo SACA", ruta);
    uint32_t version, tipo;
    memcpy(&version, enc + 4, 4);
    memcpy(&tipo, enc + 8, 4);
    if (version != 1 || tipo != 1) saca_fallar("versión o tipo no soportados (se esperaba muestra v1)", ruta);
    memcpy(&m->d, enc + 12, 4);
    memcpy(&m->n_ent, enc + 16, 8);
    memcpy(&m->n_pru, enc + 24, 8);
    memcpy(&m->pc0, enc + 32, 8);
    memcpy(&m->pc1, enc + 40, 8);
    m->Xe = (double *)malloc(m->n_ent * m->d * 8);
    m->ye = (double *)malloc(m->n_ent * 8);
    m->Xp = (double *)malloc(m->n_pru * m->d * 8);
    m->yp = (double *)malloc(m->n_pru * 8);
    if (!m->Xe || !m->ye || !m->Xp || !m->yp) saca_fallar("sin memoria para la muestra", NULL);
    saca_leer(f, m->Xe, m->n_ent * m->d, "X_entrenamiento");
    saca_leer(f, m->ye, m->n_ent, "y_entrenamiento");
    saca_leer(f, m->Xp, m->n_pru * m->d, "X_prueba");
    saca_leer(f, m->yp, m->n_pru, "y_prueba");
    fclose(f);
}

SACA_FN void saca_liberar(Muestra *m) {
    free(m->Xe); free(m->ye); free(m->Xp); free(m->yp);
}

/* Cambia el tamaño del entrenamiento a n ejemplos: con n menor se queda con
 * las primeras filas y con n mayor repite la muestra cíclicamente. Es la
 * "escala del problema" de los Experimentos 6 y 8 (10^3 a 10^7 ejemplos). */
SACA_FN void saca_redimensionar(Muestra *m, uint64_t n) {
    if (n == 0 || n == m->n_ent) return;
    double *X = (double *)malloc(n * m->d * 8), *y = (double *)malloc(n * 8);
    if (!X || !y) saca_fallar("sin memoria para redimensionar la muestra", NULL);
    for (uint64_t i = 0; i < n; i++) {
        uint64_t o = i % m->n_ent;
        memcpy(X + i * m->d, m->Xe + o * m->d, m->d * 8);
        y[i] = m->ye[o];
    }
    free(m->Xe);
    free(m->ye);
    m->Xe = X;
    m->ye = y;
    m->n_ent = n;
}

/* Igual que logistica.Sigmoide y logLoss en Go. */
SACA_FN inline double saca_sigmoide(double z) { return 1.0 / (1.0 + exp(-z)); }

SACA_FN inline double saca_logloss(double y, double p) {
    const double eps = 1e-15;
    if (p < eps) p = eps;
    if (p > 1.0 - eps) p = 1.0 - eps;
    return -(y * log(p) + (1.0 - y) * log(1.0 - p));
}

/* Gradiente parcial de las filas [ini, fin) con un solo hilo: suma (no
 * promedio) en g[0..d-1], g[d] = sesgo; devuelve la pérdida ponderada. Es
 * logistica.GradienteParcial de Go. */
SACA_FN double saca_gradiente_rango(const Muestra *m, const double *pesos, double sesgo,
                                   uint64_t ini, uint64_t fin, double *g) {
    const uint32_t d = m->d;
    double perdida = 0.0;
    for (uint64_t i = ini; i < fin; i++) {
        const double *xi = m->Xe + i * d;
        double z = sesgo;
        for (uint32_t j = 0; j < d; j++) z += pesos[j] * xi[j];
        double pred = saca_sigmoide(z);
        double pc = (m->ye[i] == 1.0) ? m->pc1 : m->pc0;
        double error = pc * (pred - m->ye[i]);
        for (uint32_t j = 0; j < d; j++) g[j] += error * xi[j];
        g[d] += error;
        perdida += pc * saca_logloss(m->ye[i], pred);
    }
    return perdida;
}

typedef struct { double p, y; } ParPY;

SACA_FN int saca_cmp_par(const void *a, const void *b) {
    double pa = ((const ParPY *)a)->p, pb = ((const ParPY *)b)->p;
    return (pa > pb) - (pa < pb);
}

/* F1 (umbral 0,5) y AUC-ROC (Mann-Whitney con rangos promedio) sobre la
 * muestra de prueba: lo mismo que internal/metricas.Evaluar. */
SACA_FN void saca_evaluar(const Muestra *m, const double *pesos, double sesgo, double *f1, double *auc) {
    const uint32_t d = m->d;
    const uint64_t n = m->n_pru;
    *f1 = 0.0;
    *auc = 0.0;
    if (n == 0) return;
    ParPY *v = (ParPY *)malloc(n * sizeof *v);
    if (!v) saca_fallar("sin memoria para evaluar", NULL);
    uint64_t vp = 0, fp = 0, fn = 0;
    for (uint64_t i = 0; i < n; i++) {
        const double *xi = m->Xp + i * d;
        double z = sesgo;
        for (uint32_t j = 0; j < d; j++) z += pesos[j] * xi[j];
        v[i].p = saca_sigmoide(z);
        v[i].y = m->yp[i];
        int predicha = v[i].p >= 0.5, real = v[i].y == 1.0;
        if (predicha && real) vp++;
        else if (predicha) fp++;
        else if (real) fn++;
    }
    double prec = vp + fp ? (double)vp / (vp + fp) : 0.0;
    double exh = vp + fn ? (double)vp / (vp + fn) : 0.0;
    if (prec + exh > 0) *f1 = 2 * prec * exh / (prec + exh);

    qsort(v, n, sizeof *v, saca_cmp_par);
    double suma_pos = 0.0;
    uint64_t pos = 0, neg = 0;
    for (uint64_t i = 0; i < n;) {
        uint64_t j = i;
        while (j < n && v[j].p == v[i].p) j++;
        double rango = (double)(i + j + 1) / 2.0;
        for (uint64_t k = i; k < j; k++) {
            if (v[k].y == 1.0) { suma_pos += rango; pos++; } else neg++;
        }
        i = j;
    }
    if (pos && neg) *auc = (suma_pos - (double)pos * (pos + 1) / 2.0) / ((double)pos * neg);
    free(v);
}

/* Número que sigue a "clave": en el JSON de referencia; NAN si no está. */
SACA_FN double saca_json_numero(const char *json, const char *clave) {
    char buscada[64];
    snprintf(buscada, sizeof buscada, "\"%s\"", clave);
    const char *p = strstr(json, buscada);
    if (!p) return NAN;
    p = strchr(p, ':');
    return p ? atof(p + 1) : NAN;
}

/* Regla de corrección del contrato: la pérdida final debe coincidir con la
 * de la referencia de Go (junto al .bin, "_referencia.json") dentro de 1e-6.
 * La referencia solo vale para las mismas épocas y tasa con que se generó:
 * con otras (p. ej. una prueba corta) no se verifica y se avisa. Devuelve 0
 * solo si se pudo comprobar y NO coincide. */
SACA_FN int saca_verificar(const char *ruta_bin, int epocas, double tasa, double perdida, FILE *salida) {
    char ruta[1024];
    snprintf(ruta, sizeof ruta, "%s", ruta_bin);
    char *punto = strrchr(ruta, '.');
    if (punto) *punto = '\0';
    strncat(ruta, "_referencia.json", sizeof ruta - strlen(ruta) - 1);
    FILE *f = fopen(ruta, "r");
    if (!f) {
        fprintf(salida, "referencia de Go no encontrada (%s): no se verificó la pérdida\n", ruta);
        return 1;
    }
    static char buf[1 << 16];
    size_t n = fread(buf, 1, sizeof buf - 1, f);
    fclose(f);
    buf[n] = '\0';
    double ref = saca_json_numero(buf, "perdida_final");
    double ref_epocas = saca_json_numero(buf, "epocas"), ref_tasa = saca_json_numero(buf, "tasa");
    if (isnan(ref)) {
        fprintf(salida, "la referencia de Go no trae perdida_final: no se verificó\n");
        return 1;
    }
    if ((int)ref_epocas != epocas || fabs(ref_tasa - tasa) > 1e-12) {
        fprintf(salida, "la referencia de Go es de %d épocas y tasa %g, esta corrida usó %d y %g: no se verificó la pérdida\n", (int)ref_epocas,
                ref_tasa, epocas, tasa);
        return 1;
    }
    double dif = fabs(perdida - ref);
    fprintf(salida, "pérdida %.6f, referencia de Go %.6f, diferencia %.2e: %s\n", perdida, ref, dif,
            dif < 1e-6 ? "CORRECTO" : "NO COINCIDE (el núcleo no se mide)");
    return dif < 1e-6;
}

/* Agrega una fila al CSV, escribiendo antes el encabezado si es nuevo. */
SACA_FN void saca_csv_agregar(const char *ruta, const char *encabezado, const char *fila) {
    if (!ruta || !*ruta) return;
    FILE *f = fopen(ruta, "r");
    int nuevo = (f == NULL);
    if (f) fclose(f);
    f = fopen(ruta, "a");
    if (!f) saca_fallar("no se pudo escribir", ruta);
    if (nuevo) fprintf(f, "%s\n", encabezado);
    fprintf(f, "%s\n", fila);
    fclose(f);
}

/* Argumentos "--nombre valor": devuelve el valor o el por defecto. */
SACA_FN const char *saca_arg(int argc, char **argv, const char *nombre, const char *defecto) {
    for (int i = 1; i + 1 < argc; i++)
        if (strcmp(argv[i], nombre) == 0) return argv[i + 1];
    return defecto;
}

SACA_FN int saca_flag(int argc, char **argv, const char *nombre) {
    for (int i = 1; i < argc; i++)
        if (strcmp(argv[i], nombre) == 0) return 1;
    return 0;
}

/* Modelo de la CPU para la columna "maquina" si no se pasó --maquina. */
SACA_FN void saca_maquina_por_defecto(char *dst, size_t n) {
    snprintf(dst, n, "desconocida");
    FILE *f = fopen("/proc/cpuinfo", "r");
    if (!f) return;
    char linea[512];
    while (fgets(linea, sizeof linea, f)) {
        if (strncmp(linea, "model name", 10) == 0) {
            char *p = strchr(linea, ':');
            if (p) {
                p++;
                while (*p == ' ') p++;
                p[strcspn(p, "\n")] = '\0';
                snprintf(dst, n, "%s", p);
            }
            break;
        }
    }
    fclose(f);
}

#define SACA_CSV_ENTRENAMIENTO "modo,goroutines,epocas,t_total_s,t_epoca_ms,perdida_final,f1,auc,maquina"

#endif
