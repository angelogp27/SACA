// Motor CUDA de la red neuronal (semanas 11 y 12): el mismo gradiente que
// internal/red/red.go, en la GPU. Lo lanza el trabajador Go y coopera con el
// motor de CPU (Go u OpenMP): cada mini-lote se parte entre los dos según su
// velocidad. Protocolo: nativo/comun/motor_red.h.
//
// Un mini-lote se procesa en trozos de hasta `trozo` ejemplos y dos kernels:
//   1. pasada:  un hilo por ejemplo hace la pasada hacia adelante y hacia
//      atrás, y deja sus activaciones y deltas en memoria global, guardadas
//      por neurona (los hilos de un warp leen posiciones contiguas);
//   2. reducir: un bloque por parámetro suma delta[o] * a[i] sobre los
//      ejemplos; cada hilo suma los suyos en un registro y el bloque reduce en
//      árbol en memoria compartida. No hace falta atomicAdd y el resultado no
//      depende del orden en que terminan los hilos.
// Se calcula en double para dar lo mismo que la CPU.
//
// Compilar:  nvcc -O3 -o bin/red_cuda     nativo/cuda/red/red.cu   (Linux)
//            nvcc -O3 -o bin/red_cuda.exe nativo/cuda/red/red.cu   (Windows)
#include <cmath>
#include <vector>
#include <cuda_runtime.h>

#include "../../comun/motor_red.h"

#ifndef HILOS
#define HILOS 256  // por bloque; potencia de 2
#endif

#define CUDA(llamada)                                                                  \
    do {                                                                               \
        cudaError_t e_ = (llamada);                                                    \
        if (e_ != cudaSuccess) {                                                       \
            fprintf(stderr, "red_cuda: %s: %s\n", #llamada, cudaGetErrorString(e_));   \
            exit(2);                                                                   \
        }                                                                              \
    } while (0)

// La arquitectura, pasada por valor a los kernels.
struct Arq {
    int n, capas[MR_MAX_CAPAS], inicio[MR_MAX_CAPAS], activ[MR_MAX_CAPAS];
};

// con_gradiente = 0: solo hacia adelante. salida[k] es la pérdida del
// ejemplo k (gradiente) o su probabilidad (predicción).
__global__ void pasada(Arq q, const double *p, const double *X, const double *y, const unsigned *idx, int n, int paso,
                       double pc0, double pc1, double *a, double *delta, double *salida, int con_gradiente) {
    const int L = q.n - 1, d = q.capas[0];
    for (int k = blockIdx.x * blockDim.x + threadIdx.x; k < n; k += gridDim.x * blockDim.x) {
        const size_t f = idx[k];
        for (int i = 0; i < d; i++) a[(size_t)i * paso + k] = X[f * d + i];
        double prob = 0;
        for (int l = 1; l <= L; l++) {
            const int in = q.capas[l - 1], out = q.capas[l];
            const double *w = p + q.inicio[l], *b = w + in * out;
            const double *prev = a + (size_t)q.activ[l - 1] * paso;
            double *act = a + (size_t)q.activ[l] * paso;
            for (int o = 0; o < out; o++) {
                double z = b[o];
                for (int i = 0; i < in; i++) z += w[o * in + i] * prev[(size_t)i * paso + k];
                if (l == L) prob = z = 1.0 / (1.0 + exp(-z));
                else if (!(z > 0)) z = 0;
                act[(size_t)o * paso + k] = z;
            }
        }
        if (!con_gradiente) {
            salida[k] = prob;
            continue;
        }
        const double yk = y[f], peso = yk == 1.0 ? pc1 : pc0;
        const double pr = fmin(fmax(prob, 1e-12), 1 - 1e-12);
        salida[k] = -peso * (yk * log(pr) + (1 - yk) * log(1 - pr));
        delta[(size_t)q.activ[L] * paso + k] = peso * (prob - yk);
        for (int l = L; l > 1; l--) {
            const int in = q.capas[l - 1], out = q.capas[l];
            const double *w = p + q.inicio[l];
            const double *dl = delta + (size_t)q.activ[l] * paso;
            const double *prev = a + (size_t)q.activ[l - 1] * paso;
            double *dp = delta + (size_t)q.activ[l - 1] * paso;
            for (int j = 0; j < in; j++) {
                double s = 0;
                if (prev[(size_t)j * paso + k] > 0)  // derivada de ReLU
                    for (int o = 0; o < out; o++) s += dl[(size_t)o * paso + k] * w[o * in + j];
                dp[(size_t)j * paso + k] = s;
            }
        }
    }
}

// Un bloque por parámetro de la capa: los in*out pesos y luego los out sesgos.
__global__ void reducir(const double *prev, const double *dl, int in, int out, int n, int paso, double *grad) {
    extern __shared__ double compartida[];
    const int j = blockIdx.x, sesgo = j >= in * out;
    const double *pd = dl + (size_t)(sesgo ? j - in * out : j / in) * paso;
    const double *pa = prev + (size_t)(sesgo ? 0 : j % in) * paso;
    double s = 0;
    for (int k = threadIdx.x; k < n; k += blockDim.x) s += sesgo ? pd[k] : pd[k] * pa[k];
    compartida[threadIdx.x] = s;
    __syncthreads();
    for (int m = blockDim.x / 2; m > 0; m >>= 1) {
        if ((int)threadIdx.x < m) compartida[threadIdx.x] += compartida[threadIdx.x + m];
        __syncthreads();
    }
    if (threadIdx.x == 0) grad[j] = compartida[0];
}

// Memoria de la GPU que solo crece.
template <class T>
struct Buf {
    T *p = nullptr;
    size_t n = 0;
    T *pedir(size_t k) {
        if (k > n) {
            cudaFree(p);
            p = nullptr;
            n = 0;
            CUDA(cudaMalloc(&p, k * sizeof(T)));
            n = k;
        }
        return p;
    }
};

int main(int argc, char **argv) {
    mr_binario();
    int dispositivos = 0;
    cudaError_t err = cudaGetDeviceCount(&dispositivos);
    if (err != cudaSuccess || dispositivos == 0) {
        fprintf(stderr, "red_cuda: no hay GPU CUDA disponible\n");
        return 1;
    }
    cudaDeviceProp prop;
    CUDA(cudaGetDeviceProperties(&prop, 0));
    char nombre[512];
    snprintf(nombre, sizeof nombre, "%s, sm_%d%d, %d SM, %.1f GB", prop.name, prop.major, prop.minor,
             prop.multiProcessorCount, prop.totalGlobalMem / 1073741824.0);
    if (argc > 1 && !strcmp(argv[1], "--info")) {
        printf("GPU: %s\n", nombre);
        return 0;
    }
    mr_presentarse(nombre);

    MrDatos D;
    MrModelo m;
    memset(&D, 0, sizeof D);
    memset(&m, 0, sizeof m);
    double *dX = nullptr, *dy = nullptr;
    unsigned *dIdx = nullptr;
    Buf<unsigned> bIdxPred;
    Buf<double> bP, bA, bDelta, bSalida, bGrad;
    uint32_t orden;
    while (mr_leer(&orden, 4) && orden != MR_TERMINAR) {
        if (orden == MR_DATOS) {
            mr_leer_datos(&D);
            cudaFree(dX);
            cudaFree(dy);
            dX = dy = nullptr;
            const size_t bx = (size_t)D.n * D.d * sizeof(double), by = (size_t)D.n * sizeof(double);
            if (cudaMalloc(&dX, bx + 8) != cudaSuccess || cudaMalloc(&dy, by + 8) != cudaSuccess ||
                cudaMemcpy(dX, D.X, bx, cudaMemcpyHostToDevice) != cudaSuccess ||
                cudaMemcpy(dy, D.y, by, cudaMemcpyHostToDevice) != cudaSuccess) {
                fprintf(stderr, "red_cuda: los datos no caben en la GPU\n");
                mr_estado(1);
                return 4;
            }
            free(D.X);  // ya están en la GPU
            free(D.y);
            D.X = D.y = nullptr;
            mr_estado(0);
        } else if (orden == MR_PARTICION) {
            free(D.idx);
            D.idx = mr_leer_indices(&D, &D.cuantos);
            cudaFree(dIdx);
            dIdx = nullptr;
            CUDA(cudaMalloc(&dIdx, (size_t)D.cuantos * 4 + 4));
            CUDA(cudaMemcpy(dIdx, D.idx, (size_t)D.cuantos * 4, cudaMemcpyHostToDevice));
            mr_estado(0);
        } else if (orden == MR_GRADIENTE || orden == MR_PREDECIR) {
            const bool grad = orden == MR_GRADIENTE;
            mr_leer_modelo(&m, D.d);
            uint32_t ini = 0, fin = 0;
            unsigned *indices = dIdx;
            if (grad) {
                mr_exigir(&ini, 4);
                mr_exigir(&fin, 4);
                if (ini > fin || fin > D.cuantos) mr_fallar("mini-lote fuera de la partición");
            } else {
                uint32_t *v = mr_leer_indices(&D, &fin);
                indices = bIdxPred.pedir((size_t)fin + 1);
                CUDA(cudaMemcpy(indices, v, (size_t)fin * 4, cudaMemcpyHostToDevice));
                free(v);
            }
            Arq q;
            q.n = m.n;
            for (int l = 0; l < m.n; l++) q.capas[l] = m.capas[l], q.inicio[l] = m.inicio[l], q.activ[l] = m.activ[l];
            double *dP = bP.pedir(m.nparams);
            CUDA(cudaMemcpy(dP, m.p, (size_t)m.nparams * sizeof(double), cudaMemcpyHostToDevice));
            // Como máximo ~256 MB de activaciones y deltas a la vez.
            size_t tope = (256u << 20) / (2 * sizeof(double) * (size_t)m.nactiv);
            if (tope < HILOS) tope = HILOS;
            if (tope > 65536) tope = 65536;
            const int trozo = (int)tope;
            double *dA = bA.pedir((size_t)m.nactiv * trozo), *dD = bDelta.pedir((size_t)m.nactiv * trozo);
            double *dS = bSalida.pedir(trozo), *dG = bGrad.pedir(m.nparams);
            std::vector<double> total(grad ? m.nparams + 1 : 0, 0.0), parcial(m.nparams), salida(trozo), prob;
            for (uint32_t c = ini; c < fin; c += trozo) {
                const int n = (int)(fin - c < (uint32_t)trozo ? fin - c : (uint32_t)trozo);
                const int bloques = (n + HILOS - 1) / HILOS;
                const unsigned *tramo = indices + c;
                const int hacer_gradiente = grad ? 1 : 0;
                pasada<<<bloques, HILOS>>>(q, dP, dX, dy, tramo, n, trozo, D.pc[0], D.pc[1], dA, dD, dS, hacer_gradiente);
                CUDA(cudaGetLastError());
                CUDA(cudaMemcpy(salida.data(), dS, n * sizeof(double), cudaMemcpyDeviceToHost));
                if (!grad) {
                    prob.insert(prob.end(), salida.begin(), salida.begin() + n);
                    continue;
                }
                for (int k = 0; k < n; k++) total[m.nparams] += salida[k];
                for (int l = 1; l < m.n; l++) {
                    const int in = m.capas[l - 1], out = m.capas[l], parametros = in * out + out;
                    const size_t bytes = HILOS * sizeof(double);
                    const double *prev = dA + (size_t)m.activ[l - 1] * trozo, *dl = dD + (size_t)m.activ[l] * trozo;
                    double *destino = dG + m.inicio[l];
                    reducir<<<parametros, HILOS, bytes>>>(prev, dl, in, out, n, trozo, destino);
                    CUDA(cudaGetLastError());
                }
                CUDA(cudaMemcpy(parcial.data(), dG, (size_t)m.nparams * sizeof(double), cudaMemcpyDeviceToHost));
                for (int j = 0; j < m.nparams; j++) total[j] += parcial[j];
            }
            const std::vector<double> &r = grad ? total : prob;
            mr_escribir(r.data(), r.size() * sizeof(double));
            fflush(stdout);
        } else {
            mr_fallar("orden desconocida");
        }
    }
    return 0;
}
