// Entrenamiento de la regresión logística en la GPU con tres kernels de
// gradiente (semanas 11 y 12; Experimento 8). Es el mismo cálculo que
// logistica.Entrenar de Go: descenso de gradiente por lote completo con
// pesos de clase. Lo único que cambia entre --kernel es cómo se suman los
// aportes de los hilos:
//
//   atomic   Un hilo por ejemplo (grid-stride). Cada hilo suma su aporte
//            directamente al gradiente global con atomicAdd: d+2 sumas
//            atómicas por ejemplo sobre las mismas d+2 direcciones. Es el
//            kernel ingenuo (semana 11): correcto, pero toda la GPU se
//            serializa en esas direcciones.
//   shared   Cada hilo acumula en su propio tramo de memoria compartida y el
//            bloque reduce con un árbol (__shared__ + __syncthreads, semana
//            12). Un solo acceso a memoria global por bloque y por valor.
//   shuffle  Cada hilo acumula en registros y la reducción dentro de cada
//            warp usa __shfl_down_sync, sin memoria compartida ni barreras
//            (semana 12). Los 32 hilos de un warp ya están sincronizados.
//
// En shared y shuffle cada bloque deja su suma parcial en memoria global y la
// CPU las suma en orden fijo (resultado determinista). Los datos se copian a
// la GPU una sola vez; por época solo viajan d+1 números de ida y d+2 por
// bloque de vuelta (uno solo con atomic). t_total mide las épocas, sin la
// lectura del archivo ni la copia inicial.
//
// Antes de medir, el gradiente de la época 0 de la GPU se compara con el de la
// CPU (tolerancia relativa 1e-8); si no coincide, el kernel no se mide.
//
// Compilar (PC con GPU NVIDIA y CUDA Toolkit; ver nativo/cuda/README.md):
//   nvcc -O3 -o bin/gradiente_cuda_exp nativo/cuda/gradiente/gradiente.cu
// Ejecutar:
//   bin/gradiente_cuda_exp --datos data/muestra.bin --kernel shuffle --hilos 256
//       --ejemplos 10000000 --resultado resultados/experimento8.csv
//
// Columnas del CSV: goroutines = tamaño del bloque (hilos por bloque); el
// tamaño del problema queda en "maquina" ("· N ejemplos").

#include <cuda_runtime.h>

#include "../../comun/saca.h"

#define D_REG 16  // d + 2 debe caber: los acumuladores del kernel shuffle viven en registros

#define CUDA(llamada)                                                        \
    do {                                                                     \
        cudaError_t e_ = (llamada);                                          \
        if (e_ != cudaSuccess) {                                             \
            fprintf(stderr, "gradiente_cuda_exp: %s: %s\n", #llamada, cudaGetErrorString(e_)); \
            exit(2);                                                         \
        }                                                                    \
    } while (0)

__device__ double sigmoide(double z) { return 1.0 / (1.0 + exp(-z)); }

__device__ double log_loss(double y, double p) {
    const double eps = 1e-15;
    p = fmin(fmax(p, eps), 1.0 - eps);
    return -(y * log(p) + (1.0 - y) * log(1.0 - p));
}

// atomicAdd sobre double existe desde sm_60; antes se hace con atomicCAS.
__device__ void suma_atomica(double *dir, double val) {
#if !defined(__CUDA_ARCH__) || __CUDA_ARCH__ >= 600
    atomicAdd(dir, val);
#else
    unsigned long long *a = (unsigned long long *)dir;
    unsigned long long viejo = *a, asumido;
    do {
        asumido = viejo;
        viejo = atomicCAS(a, asumido, __double_as_longlong(val + __longlong_as_double(asumido)));
    } while (asumido != viejo);
#endif
}

// Aporte de un ejemplo: error ponderado (de él salen el gradiente y el sesgo) y su pérdida.
__device__ void aporte_ejemplo(const double *xi, double yi, int d, const double *pesos, double sesgo,
                               double pc0, double pc1, double *error, double *perdida) {
    double z = sesgo;
    for (int j = 0; j < d; j++) z += pesos[j] * xi[j];
    double pred = sigmoide(z);
    double pc = (yi == 1.0) ? pc1 : pc0;
    *error = pc * (pred - yi);
    *perdida = pc * log_loss(yi, pred);
}

// ---- kernel 1: atomicAdd directo a memoria global ----
__global__ void k_atomic(const double *X, const double *y, long long n, int d, const double *pesos, double sesgo,
                         double pc0, double pc1, double *grad /* d+2, en cero */) {
    for (long long i = blockIdx.x * (long long)blockDim.x + threadIdx.x; i < n; i += (long long)gridDim.x * blockDim.x) {
        const double *xi = X + i * d;
        double error, perdida;
        aporte_ejemplo(xi, y[i], d, pesos, sesgo, pc0, pc1, &error, &perdida);
        for (int j = 0; j < d; j++) suma_atomica(&grad[j], error * xi[j]);
        suma_atomica(&grad[d], error);
        suma_atomica(&grad[d + 1], perdida);
    }
}

// ---- kernel 2: árbol de reducción en memoria compartida ----
__global__ void k_shared(const double *X, const double *y, long long n, int d, const double *pesos, double sesgo,
                         double pc0, double pc1, double *parciales) {
    extern __shared__ double compartida[];  // blockDim.x * (d+2)
    const int ancho = d + 2;
    double *mio = compartida + threadIdx.x * ancho;
    for (int k = 0; k < ancho; k++) mio[k] = 0.0;

    for (long long i = blockIdx.x * (long long)blockDim.x + threadIdx.x; i < n; i += (long long)gridDim.x * blockDim.x) {
        const double *xi = X + i * d;
        double error, perdida;
        aporte_ejemplo(xi, y[i], d, pesos, sesgo, pc0, pc1, &error, &perdida);
        for (int j = 0; j < d; j++) mio[j] += error * xi[j];
        mio[d] += error;
        mio[d + 1] += perdida;
    }
    __syncthreads();

    for (int s = blockDim.x / 2; s > 0; s >>= 1) {  // blockDim.x es potencia de 2
        if ((int)threadIdx.x < s) {
            double *otro = compartida + (threadIdx.x + s) * ancho;
            for (int k = 0; k < ancho; k++) mio[k] += otro[k];
        }
        __syncthreads();
    }
    if (threadIdx.x == 0)
        for (int k = 0; k < ancho; k++) parciales[blockIdx.x * ancho + k] = compartida[k];
}

// ---- kernel 3: warp shuffles ----
__global__ void k_shuffle(const double *X, const double *y, long long n, int d, const double *pesos, double sesgo,
                          double pc0, double pc1, double *parciales) {
    __shared__ double por_warp[32 * D_REG];  // a lo más 32 warps por bloque
    const int ancho = d + 2;
    double acc[D_REG];
#pragma unroll
    for (int k = 0; k < D_REG; k++) acc[k] = 0.0;

    for (long long i = blockIdx.x * (long long)blockDim.x + threadIdx.x; i < n; i += (long long)gridDim.x * blockDim.x) {
        const double *xi = X + i * d;
        double error, perdida;
        aporte_ejemplo(xi, y[i], d, pesos, sesgo, pc0, pc1, &error, &perdida);
        // Índices constantes (bucle desenrollado) para que acc viva en registros y no en memoria local.
#pragma unroll
        for (int k = 0; k < D_REG; k++) {
            if (k < d) acc[k] += error * xi[k];
            else if (k == d) acc[k] += error;
            else if (k == d + 1) acc[k] += perdida;
        }
    }

    // Reducción dentro del warp: cada paso trae el valor del hilo `desp` lugares más arriba.
    for (int desp = 16; desp > 0; desp >>= 1) {
#pragma unroll
        for (int k = 0; k < D_REG; k++) acc[k] += __shfl_down_sync(0xffffffffu, acc[k], desp);
    }
    const int carril = threadIdx.x & 31, warp = threadIdx.x >> 5;
    if (carril == 0)
        for (int k = 0; k < ancho; k++) por_warp[warp * D_REG + k] = acc[k];
    __syncthreads();

    // El hilo 0 suma los warps en orden fijo.
    if (threadIdx.x == 0) {
        const int warps = (blockDim.x + 31) >> 5;
        for (int k = 0; k < ancho; k++) {
            double s = 0.0;
            for (int w = 0; w < warps; w++) s += por_warp[w * D_REG + k];
            parciales[blockIdx.x * ancho + k] = s;
        }
    }
}

enum Kernel { ATOMIC, SHARED, SHUFFLE };

struct Plan {
    Kernel kernel;
    int d, hilos, bloques;
    long long n;
    double pc0, pc1;
    double *dX, *dy, *dPesos, *dSalida;  // dSalida: d+2 (atomic) o bloques*(d+2)
    size_t bytesCompartida;
    std::vector<double> parciales;
};

// Calcula el gradiente de toda la muestra con el kernel del plan: g[0..d-1], g[d] = sesgo, g[d+1] = pérdida.
static void gradiente_gpu(Plan &p, const double *pesos, double sesgo, double *g) {
    const int ancho = p.d + 2;
    CUDA(cudaMemcpy(p.dPesos, pesos, p.d * sizeof(double), cudaMemcpyHostToDevice));
    if (p.kernel == ATOMIC) {
        CUDA(cudaMemset(p.dSalida, 0, ancho * sizeof(double)));
        k_atomic<<<p.bloques, p.hilos>>>(p.dX, p.dy, p.n, p.d, p.dPesos, sesgo, p.pc0, p.pc1, p.dSalida);
        CUDA(cudaGetLastError());
        CUDA(cudaMemcpy(g, p.dSalida, ancho * sizeof(double), cudaMemcpyDeviceToHost));
        return;
    }
    if (p.kernel == SHARED)
        k_shared<<<p.bloques, p.hilos, p.bytesCompartida>>>(p.dX, p.dy, p.n, p.d, p.dPesos, sesgo, p.pc0, p.pc1, p.dSalida);
    else
        k_shuffle<<<p.bloques, p.hilos>>>(p.dX, p.dy, p.n, p.d, p.dPesos, sesgo, p.pc0, p.pc1, p.dSalida);
    CUDA(cudaGetLastError());
    CUDA(cudaMemcpy(p.parciales.data(), p.dSalida, p.parciales.size() * sizeof(double), cudaMemcpyDeviceToHost));
    for (int k = 0; k < ancho; k++) g[k] = 0.0;
    for (int b = 0; b < p.bloques; b++)
        for (int k = 0; k < ancho; k++) g[k] += p.parciales[(size_t)b * ancho + k];
}

int main(int argc, char **argv) {
    const char *ruta = saca_arg(argc, argv, "--datos", "data/muestra.bin");
    const double tasa = atof(saca_arg(argc, argv, "--tasa", "0.1"));
    const int epocas = atoi(saca_arg(argc, argv, "--epocas", "200"));
    int hilos = atoi(saca_arg(argc, argv, "--hilos", "256"));
    const char *nombre_kernel = saca_arg(argc, argv, "--kernel", "shared");
    const char *resultado = saca_arg(argc, argv, "--resultado", "");
    const unsigned long long ejemplos = strtoull(saca_arg(argc, argv, "--ejemplos", "0"), NULL, 10);

    Plan p;
    if (strcmp(nombre_kernel, "atomic") == 0) p.kernel = ATOMIC;
    else if (strcmp(nombre_kernel, "shared") == 0) p.kernel = SHARED;
    else if (strcmp(nombre_kernel, "shuffle") == 0) p.kernel = SHUFFLE;
    else saca_fallar("--kernel debe ser atomic, shared o shuffle", nombre_kernel);
    if (hilos < 32 || hilos > 1024 || (hilos & (hilos - 1)) != 0) saca_fallar("--hilos debe ser una potencia de 2 entre 32 y 1024", NULL);

    int dispositivos = 0;
    if (cudaGetDeviceCount(&dispositivos) != cudaSuccess || dispositivos == 0) saca_fallar("no hay GPU CUDA disponible", NULL);
    cudaDeviceProp prop;
    CUDA(cudaGetDeviceProperties(&prop, 0));
    char maquina[512];
    const char *m_arg = saca_arg(argc, argv, "--maquina", NULL);
    if (m_arg) snprintf(maquina, sizeof maquina, "%s", m_arg);
    else snprintf(maquina, sizeof maquina, "%s", prop.name);

    Muestra m;
    saca_cargar(ruta, &m);
    saca_redimensionar(&m, ejemplos);
    p.d = (int)m.d;
    p.n = (long long)m.n_ent;
    p.pc0 = m.pc0;
    p.pc1 = m.pc1;
    if (p.d + 2 > D_REG) saca_fallar("este núcleo admite hasta 14 variables (D_REG)", NULL);
    const int ancho = p.d + 2;

    // Configuración: igual que gradiente_servidor.cu, la memoria compartida limita los hilos del kernel shared.
    if (p.kernel == SHARED)
        while (hilos > 32 && (size_t)hilos * ancho * sizeof(double) > prop.sharedMemPerBlock) hilos /= 2;
    p.hilos = hilos;
    long long necesarios = (p.n + hilos - 1) / hilos;
    p.bloques = (int)(necesarios < (long long)prop.multiProcessorCount * 8 ? necesarios : (long long)prop.multiProcessorCount * 8);
    if (p.bloques < 1) p.bloques = 1;
    p.bytesCompartida = (size_t)hilos * ancho * sizeof(double);
    const size_t n_salida = p.kernel == ATOMIC ? (size_t)ancho : (size_t)p.bloques * ancho;
    p.parciales.assign(p.kernel == ATOMIC ? 0 : n_salida, 0.0);

    CUDA(cudaMalloc(&p.dX, (size_t)p.n * p.d * sizeof(double)));
    CUDA(cudaMalloc(&p.dy, (size_t)p.n * sizeof(double)));
    CUDA(cudaMalloc(&p.dPesos, p.d * sizeof(double)));
    CUDA(cudaMalloc(&p.dSalida, n_salida * sizeof(double)));
    CUDA(cudaMemcpy(p.dX, m.Xe, (size_t)p.n * p.d * sizeof(double), cudaMemcpyHostToDevice));
    CUDA(cudaMemcpy(p.dy, m.ye, (size_t)p.n * sizeof(double), cudaMemcpyHostToDevice));

    char etiqueta[64];
    snprintf(etiqueta, sizeof etiqueta, "cuda_%s", nombre_kernel);
    fprintf(stderr, "%s en %s: %lld ejemplos x %d variables (%d bloques x %d hilos)\n", etiqueta, prop.name, p.n, p.d, p.bloques, p.hilos);

    // Corrección: el gradiente de la época 0 (pesos en cero) debe coincidir con el de la CPU. Sirve también de calentamiento.
    double pesos[D_REG] = {0}, sesgo = 0.0;
    double g[D_REG], gcpu[D_REG] = {0};
    gradiente_gpu(p, pesos, sesgo, g);
    gcpu[p.d + 1] = saca_gradiente_rango(&m, pesos, sesgo, 0, m.n_ent, gcpu);
    for (int k = 0; k < ancho; k++) {
        double tol = 1e-8 * fmax(1.0, fabs(gcpu[k]));
        if (fabs(g[k] - gcpu[k]) > tol) {
            fprintf(stderr, "%s: la componente %d del gradiente no coincide con la CPU (GPU %.12g, CPU %.12g): el kernel no se mide\n", etiqueta, k, g[k], gcpu[k]);
            return 1;
        }
    }
    fprintf(stderr, "%s: gradiente de la época 0 validado contra la CPU\n", etiqueta);

    double perdida_epoca = 0.0;
    const double inicio = saca_ahora();
    for (int e = 0; e < epocas; e++) {
        gradiente_gpu(p, pesos, sesgo, g);  // el cudaMemcpy a la CPU espera al kernel
        for (int j = 0; j < p.d; j++) pesos[j] -= tasa * g[j] / (double)p.n;
        sesgo -= tasa * g[p.d] / (double)p.n;
        perdida_epoca = g[p.d + 1] / (double)p.n;
    }
    const double t_total = saca_ahora() - inicio;

    double f1, auc;
    saca_evaluar(&m, pesos, sesgo, &f1, &auc);
    printf("%s con bloques de %d hilos: %d épocas en %.3f s (%.3f ms/época)\n", etiqueta, p.hilos, epocas, t_total, 1000.0 * t_total / epocas);
    printf("F1 %.4f, AUC %.4f\n", f1, auc);
    int ok = 1;
    if (ejemplos == 0) ok = saca_verificar(ruta, epocas, tasa, perdida_epoca, stdout);
    else {
        printf("con --ejemplos no se compara con la referencia de Go\n");
        char suf[64];
        snprintf(suf, sizeof suf, " · %lld ejemplos", p.n);
        strncat(maquina, suf, sizeof maquina - strlen(maquina) - 1);
    }

    char fila[1024];
    snprintf(fila, sizeof fila, "%s,%d,%d,%.6f,%.4f,%.6f,%.4f,%.4f", etiqueta, p.hilos, epocas, t_total, 1000.0 * t_total / epocas, perdida_epoca, f1, auc);
    saca_csv_agregar(resultado, SACA_CSV_ENTRENAMIENTO, fila, maquina);

    cudaFree(p.dX);
    cudaFree(p.dy);
    cudaFree(p.dPesos);
    cudaFree(p.dSalida);
    saca_liberar(&m);
    return ok ? 0 : 1;
}
