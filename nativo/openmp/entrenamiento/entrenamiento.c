/* Entrenamiento de la regresión logística con OpenMP (semanas 7, 9 y 10).
 *
 * Es el mismo cálculo que logistica.Entrenar de Go (descenso de gradiente
 * por lote completo con pesos de clase). El gradiente de cada época se
 * paraleliza con
 *
 *     #pragma omp parallel for reduction(+:g[:d+1], perdida) schedule(runtime)
 *
 * Cada hilo acumula en una copia privada de g (variable privada) y OpenMP
 * las suma al final del bucle (reducción): es lo que en Go hace
 * gradienteReduccion con una casilla por goroutine. El reparto de las
 * iteraciones se elige en la línea de comandos con --schedule para el
 * Experimento 6 (static, dynamic, guided).
 *
 * Compilar:  gcc -O3 -fopenmp -o bin/entrenamiento_openmp nativo/openmp/entrenamiento/entrenamiento.c -lm
 * Ejecutar:  bin/entrenamiento_openmp --datos data/muestra.bin --hilos 8 --schedule static \
 *                --resultado resultados/experimento6.csv
 * Línea base: agregar --secuencial (un hilo, sin OpenMP; modo "secuencial_c").
 * Escala del problema (Experimento 8): --ejemplos N recorta o repite cíclicamente la
 * muestra de entrenamiento hasta tener N ejemplos; no se compara con la referencia de Go.
 *
 * Interfaz común: docs/contrato_formato_binario.md.
 */
#include <omp.h>

#include "../../comun/saca.h"

#define D_MAX 63

int main(int argc, char **argv) {
    const char *ruta = saca_arg(argc, argv, "--datos", "data/muestra.bin");
    const double tasa = atof(saca_arg(argc, argv, "--tasa", "0.1"));
    const int epocas = atoi(saca_arg(argc, argv, "--epocas", "200"));
    int hilos = atoi(saca_arg(argc, argv, "--hilos", "0"));
    const char *schedule = saca_arg(argc, argv, "--schedule", "static");
    const int chunk = atoi(saca_arg(argc, argv, "--chunk", "0"));
    const char *resultado = saca_arg(argc, argv, "--resultado", "");
    const uint64_t ejemplos = strtoull(saca_arg(argc, argv, "--ejemplos", "0"), NULL, 10);
    const int secuencial = saca_flag(argc, argv, "--secuencial");
    char maquina[256];
    saca_maquina_por_defecto(maquina, sizeof maquina);
    const char *m_arg = saca_arg(argc, argv, "--maquina", NULL);
    if (m_arg) snprintf(maquina, sizeof maquina, "%s", m_arg);

    omp_sched_t tipo;
    if (strcmp(schedule, "static") == 0) tipo = omp_sched_static;
    else if (strcmp(schedule, "dynamic") == 0) tipo = omp_sched_dynamic;
    else if (strcmp(schedule, "guided") == 0) tipo = omp_sched_guided;
    else saca_fallar("--schedule debe ser static, dynamic o guided", schedule);
    /* dynamic y guided necesitan un tamaño de bloque razonable; con 1 el
     * costo de repartir cada iteración se comería el cálculo. */
    int bloque = chunk > 0 ? chunk : (tipo == omp_sched_static ? 0 : 4096);
    omp_set_schedule(tipo, bloque);
    if (hilos < 1) hilos = omp_get_max_threads();
    if (secuencial) hilos = 1;
    omp_set_num_threads(hilos);

    Muestra m;
    double t0 = saca_ahora();
    saca_cargar(ruta, &m);
    fprintf(stderr, "datos leídos en %.2f s: %llu ejemplos de entrenamiento, %llu de prueba, d=%u\n", saca_ahora() - t0,
            (unsigned long long)m.n_ent, (unsigned long long)m.n_pru, m.d);
    saca_redimensionar(&m, ejemplos);
    if (ejemplos) {
        char suf[64];
        snprintf(suf, sizeof suf, " · %llu ejemplos", (unsigned long long)m.n_ent);
        strncat(maquina, suf, sizeof maquina - strlen(maquina) - 1);
    }
    const uint32_t d = m.d;
    if (d > D_MAX) saca_fallar("demasiadas variables para este núcleo", NULL);
    const uint64_t n = m.n_ent;

    double pesos[D_MAX + 1] = {0};
    double sesgo = 0.0, perdida_epoca = 0.0;

    double inicio = saca_ahora();
    for (int e = 0; e < epocas; e++) {
        double g[D_MAX + 1] = {0};
        double perdida = 0.0;
        if (secuencial) {
            perdida = saca_gradiente_rango(&m, pesos, sesgo, 0, n, g);
        } else {
            const double *w = pesos;
            const double sb = sesgo;
#pragma omp parallel for reduction(+ : g[ : D_MAX + 1], perdida) schedule(runtime)
            for (long long i = 0; i < (long long)n; i++) {
                const double *xi = m.Xe + (uint64_t)i * d;
                double z = sb;
                for (uint32_t j = 0; j < d; j++) z += w[j] * xi[j];
                double pred = saca_sigmoide(z);
                double pc = (m.ye[i] == 1.0) ? m.pc1 : m.pc0;
                double error = pc * (pred - m.ye[i]);
                for (uint32_t j = 0; j < d; j++) g[j] += error * xi[j];
                g[d] += error;
                perdida += pc * saca_logloss(m.ye[i], pred);
            }
        }
        for (uint32_t j = 0; j < d; j++) pesos[j] -= tasa * g[j] / (double)n;
        sesgo -= tasa * g[d] / (double)n;
        perdida_epoca = perdida / (double)n;
    }
    const double t_total = saca_ahora() - inicio;

    double f1, auc;
    saca_evaluar(&m, pesos, sesgo, &f1, &auc);
    const char *modo_s = secuencial ? "secuencial_c" : NULL;
    char modo[64];
    if (modo_s) snprintf(modo, sizeof modo, "%s", modo_s);
    else snprintf(modo, sizeof modo, "openmp_%s", schedule);

    printf("%s con %d hilo(s): %d épocas en %.3f s (%.2f ms/época)\n", modo, hilos, epocas, t_total, 1000.0 * t_total / epocas);
    printf("F1 %.4f, AUC %.4f\n", f1, auc);
    const int ok = ejemplos ? 1 : saca_verificar(ruta, epocas, tasa, perdida_epoca, stdout);

    char fila[1024];
    snprintf(fila, sizeof fila, "%s,%d,%d,%.6f,%.4f,%.6f,%.4f,%.4f,%s", modo, hilos, epocas, t_total, 1000.0 * t_total / epocas,
             perdida_epoca, f1, auc, maquina);
    saca_csv_agregar(resultado, SACA_CSV_ENTRENAMIENTO, fila);
    saca_liberar(&m);
    return ok ? 0 : 1;
}
