/* Entrenamiento de la regresión logística con MPI (semanas 3 y 4).
 *
 * Modelo SPMD: todos los procesos corren este mismo programa y se
 * distinguen por su rango. Es el equivalente en MPI del maestro y los
 * trabajadores TCP de Go (entrenamiento/cmd): el reparto de la muestra es
 * el scatter, los parámetros son el broadcast y la suma de gradientes es
 * el reduce.
 *
 *   rango 0 lee data/muestra.bin
 *   MPI_Scatterv  reparte las filas de entrenamiento en bloques contiguos
 *   cada época:
 *     --modo colectivo  MPI_Allreduce suma los gradientes parciales y todos
 *                       los procesos actualizan sus parámetros igual
 *     --modo p2p        cada proceso hace MPI_Send de su gradiente al 0; el 0
 *                       los suma con MPI_Recv, actualiza y los reenvía con
 *                       MPI_Bcast (la forma manual del mismo reduce+broadcast)
 *   rango 0 evalúa (F1, AUC), compara con la referencia de Go y escribe el CSV
 *
 * Compilar:  mpicc -O3 -o bin/entrenamiento_mpi nativo/mpi/entrenamiento/entrenamiento.c -lm
 * Ejecutar:  mpiexec -n 4 bin/entrenamiento_mpi --datos data/muestra.bin --modo colectivo \
 *                --resultado resultados/experimento7.csv
 * Varias PCs: mpiexec --hostfile hosts -n 8 ...  (ver docs/guia_cluster.md)
 *
 * El número de procesos lo da mpiexec -n, no --hilos. En el CSV va en la
 * columna "goroutines" (docs/contrato_formato_binario.md).
 */
#include <mpi.h>

#include "../../comun/saca.h"

#define D_MAX 63

static void chequear(int rc, const char *que) {
    if (rc != MPI_SUCCESS) {
        fprintf(stderr, "MPI falló en %s\n", que);
        MPI_Abort(MPI_COMM_WORLD, 2);
    }
}

int main(int argc, char **argv) {
    MPI_Init(&argc, &argv);
    int rango, p;
    MPI_Comm_rank(MPI_COMM_WORLD, &rango);
    MPI_Comm_size(MPI_COMM_WORLD, &p);

    const char *ruta = saca_arg(argc, argv, "--datos", "data/muestra.bin");
    const double tasa = atof(saca_arg(argc, argv, "--tasa", "0.1"));
    const int epocas = atoi(saca_arg(argc, argv, "--epocas", "200"));
    const char *modo_arg = saca_arg(argc, argv, "--modo", "colectivo");
    const char *resultado = saca_arg(argc, argv, "--resultado", "");
    const int colectivo = strcmp(modo_arg, "colectivo") == 0;
    if (!colectivo && strcmp(modo_arg, "p2p") != 0) {
        if (rango == 0) fprintf(stderr, "--modo debe ser colectivo o p2p\n");
        MPI_Abort(MPI_COMM_WORLD, 1);
    }
    char maquina[256];
    saca_maquina_por_defecto(maquina, sizeof maquina);
    const char *m_arg = saca_arg(argc, argv, "--maquina", NULL);
    if (m_arg) snprintf(maquina, sizeof maquina, "%s", m_arg);

    /* El rango 0 lee la muestra; los demás reciben solo las dimensiones. */
    Muestra m;
    memset(&m, 0, sizeof m);
    uint64_t cab[4] = {0};
    double pc[2] = {0};
    if (rango == 0) {
        double t0 = saca_ahora();
        saca_cargar(ruta, &m);
        fprintf(stderr, "rango 0: datos leídos en %.2f s (%llu ejemplos, d=%u), %d procesos\n", saca_ahora() - t0,
                (unsigned long long)m.n_ent, m.d, p);
        cab[0] = m.d;
        cab[1] = m.n_ent;
        pc[0] = m.pc0;
        pc[1] = m.pc1;
    }
    chequear(MPI_Bcast(cab, 2, MPI_UINT64_T, 0, MPI_COMM_WORLD), "Bcast de las dimensiones");
    chequear(MPI_Bcast(pc, 2, MPI_DOUBLE, 0, MPI_COMM_WORLD), "Bcast de los pesos de clase");
    const int d = (int)cab[0];
    const uint64_t n = cab[1];
    if (d > D_MAX) {
        if (rango == 0) fprintf(stderr, "demasiadas variables para este núcleo\n");
        MPI_Abort(MPI_COMM_WORLD, 1);
    }

    /* Scatter: bloques contiguos de filas; los primeros n % p procesos reciben una fila más. */
    int *cuentas = malloc((size_t)p * sizeof(int)), *desp = malloc((size_t)p * sizeof(int));
    int *cuentas_y = malloc((size_t)p * sizeof(int)), *desp_y = malloc((size_t)p * sizeof(int));
    uint64_t fila = 0;
    for (int r = 0; r < p; r++) {
        uint64_t filas = n / p + ((uint64_t)r < n % p ? 1 : 0);
        cuentas[r] = (int)(filas * d);
        desp[r] = (int)(fila * d);
        cuentas_y[r] = (int)filas;
        desp_y[r] = (int)fila;
        fila += filas;
    }
    const uint64_t n_local = (uint64_t)cuentas_y[rango];
    Muestra local;
    memset(&local, 0, sizeof local);
    local.d = (uint32_t)d;
    local.n_ent = n_local;
    local.pc0 = pc[0];
    local.pc1 = pc[1];
    local.Xe = malloc((n_local ? n_local : 1) * d * sizeof(double));
    local.ye = malloc((n_local ? n_local : 1) * sizeof(double));

    MPI_Barrier(MPI_COMM_WORLD);
    double t_scatter = MPI_Wtime();
    chequear(MPI_Scatterv(m.Xe, cuentas, desp, MPI_DOUBLE, local.Xe, cuentas[rango], MPI_DOUBLE, 0, MPI_COMM_WORLD), "Scatterv de X");
    chequear(MPI_Scatterv(m.ye, cuentas_y, desp_y, MPI_DOUBLE, local.ye, cuentas_y[rango], MPI_DOUBLE, 0, MPI_COMM_WORLD), "Scatterv de y");
    t_scatter = MPI_Wtime() - t_scatter;
    if (rango == 0) fprintf(stderr, "Scatterv de la muestra: %.2f s\n", t_scatter);

    double pesos[D_MAX + 1] = {0}; /* pesos[0..d-1] y sesgo en pesos[d]: viajan juntos en el Bcast */
    double perdida_epoca = 0.0;
    double t_comunicacion = 0.0;

    MPI_Barrier(MPI_COMM_WORLD);
    const double inicio = MPI_Wtime();
    for (int e = 0; e < epocas; e++) {
        double g[D_MAX + 2] = {0}; /* g[0..d-1] gradiente, g[d] sesgo, g[d+1] pérdida */
        g[d + 1] = saca_gradiente_rango(&local, pesos, pesos[d], 0, n_local, g);

        double tc = MPI_Wtime();
        if (colectivo) {
            /* Reduce + broadcast en una sola operación colectiva. */
            chequear(MPI_Allreduce(MPI_IN_PLACE, g, d + 2, MPI_DOUBLE, MPI_SUM, MPI_COMM_WORLD), "Allreduce");
        } else {
            /* La misma suma, con mensajes punto a punto hacia el rango 0. */
            if (rango == 0) {
                double otro[D_MAX + 2];
                for (int r = 1; r < p; r++) {
                    chequear(MPI_Recv(otro, d + 2, MPI_DOUBLE, r, 0, MPI_COMM_WORLD, MPI_STATUS_IGNORE), "Recv");
                    for (int k = 0; k < d + 2; k++) g[k] += otro[k];
                }
            } else {
                chequear(MPI_Send(g, d + 2, MPI_DOUBLE, 0, 0, MPI_COMM_WORLD), "Send");
            }
        }
        t_comunicacion += MPI_Wtime() - tc;

        if (colectivo || rango == 0) {
            for (int j = 0; j < d; j++) pesos[j] -= tasa * g[j] / (double)n;
            pesos[d] -= tasa * g[d] / (double)n;
            perdida_epoca = g[d + 1] / (double)n;
        }
        if (!colectivo) {
            tc = MPI_Wtime();
            chequear(MPI_Bcast(pesos, d + 1, MPI_DOUBLE, 0, MPI_COMM_WORLD), "Bcast de los parámetros");
            t_comunicacion += MPI_Wtime() - tc;
        }
    }
    MPI_Barrier(MPI_COMM_WORLD);
    const double t_total = MPI_Wtime() - inicio;

    int ok = 1;
    if (rango == 0) {
        double f1, auc;
        saca_evaluar(&m, pesos, pesos[d], &f1, &auc);
        char modo[32];
        snprintf(modo, sizeof modo, "mpi_%s", colectivo ? "colectivo" : "p2p");
        printf("%s con %d proceso(s): %d épocas en %.3f s (%.2f ms/época, %.1f%% en comunicación del rango 0)\n", modo, p, epocas, t_total,
               1000.0 * t_total / epocas, 100.0 * t_comunicacion / t_total);
        printf("F1 %.4f, AUC %.4f\n", f1, auc);
        ok = saca_verificar(ruta, epocas, tasa, perdida_epoca, stdout);
        char filaCSV[1024];
        snprintf(filaCSV, sizeof filaCSV, "%s,%d,%d,%.6f,%.4f,%.6f,%.4f,%.4f", modo, p, epocas, t_total, 1000.0 * t_total / epocas,
                 perdida_epoca, f1, auc);
        saca_csv_agregar(resultado, SACA_CSV_ENTRENAMIENTO, filaCSV, maquina);
        saca_liberar(&m);
    }
    free(local.Xe);
    free(local.ye);
    free(cuentas); free(desp); free(cuentas_y); free(desp_y);
    MPI_Finalize();
    return ok ? 0 : 1;
}
