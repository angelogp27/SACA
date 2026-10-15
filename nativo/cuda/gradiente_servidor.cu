// Motor CUDA del entrenamiento distribuido (Parte B, nodos con GPU NVIDIA).
//
// Calcula en la GPU el gradiente parcial de la regresión logística sobre
// la partición de un nodo. No habla por la red: lo lanza el trabajador Go
// (entrenamiento/cmd/trabajador -motor cuda) y se comunican por
// stdin/stdout con el protocolo binario de internal/distribuido/externo.go:
//
//   motor -> Go, al arrancar:  "SACA" | uint32 largo | nombre del dispositivo
//   Go -> motor, una vez:       uint32 d | uint64 n | double pc0, pc1 | X[n*d] por filas | y[n]
//   motor -> Go:                uint32 estado (0 = datos en la GPU)
//   Go -> motor, cada época:    uint32 orden (1) | double pesos[d] | double sesgo
//   motor -> Go:                double grad[d] | grad_sesgo | perdida
//   Go -> motor, otra partición: uint32 orden (2) | los mismos datos que la carga inicial
//   motor -> Go:                uint32 estado
//   Go -> motor, al terminar:   uint32 orden (0)
//
// Los datos se copian a la GPU una vez por corrida; por época solo viajan
// d+1 números de ida y d+2 de vuelta. La orden 2 la usa el maestro en modo
// servidor, que lanza varias corridas y en cada una reparte otras
// particiones.
//
// Kernel: cada hilo acumula en registros el aporte de sus ejemplos
// (grid-stride loop), el bloque reduce en memoria compartida con un árbol
// (semana 12 del sílabo) y cada bloque deja su suma en un arreglo global.
// La suma final de los bloques se hace en la CPU, en orden fijo: así el
// resultado es determinista y no hace falta atomicAdd sobre double (que
// exige sm_60 o superior). Las variantes con atomicAdd y warp shuffles son
// las del Experimento 8.
//
// Compilar (en una PC con GPU NVIDIA y CUDA Toolkit; ver README.md):
//   nvcc -O3 -o bin/gradiente_cuda.exe nativo/cuda/gradiente_servidor.cu      (Windows)
//   nvcc -O3 -o bin/gradiente_cuda     nativo/cuda/gradiente_servidor.cu      (Linux)
// Probar solo:  bin/gradiente_cuda.exe --info

#include <algorithm>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>
#include <cuda_runtime.h>

#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif

#define HILOS_POR_BLOQUE 256

static void fallar(const char *que, cudaError_t err) {
    fprintf(stderr, "gradiente_cuda: %s: %s\n", que, cudaGetErrorString(err));
    exit(2);
}

#define CUDA(llamada)                         \
    do {                                      \
        cudaError_t e_ = (llamada);           \
        if (e_ != cudaSuccess) fallar(#llamada, e_); \
    } while (0)

static bool leer(void *dst, size_t bytes) {
    char *p = static_cast<char *>(dst);
    while (bytes > 0) {
        size_t n = fread(p, 1, bytes, stdin);
        if (n == 0) return false;
        p += n;
        bytes -= n;
    }
    return true;
}

static void escribir(const void *src, size_t bytes) {
    if (fwrite(src, 1, bytes, stdout) != bytes) {
        fprintf(stderr, "gradiente_cuda: no se pudo escribir en stdout\n");
        exit(3);
    }
}

// Igual que logistica.Sigmoide y logLoss en Go.
__device__ double sigmoide(double z) { return 1.0 / (1.0 + exp(-z)); }

__device__ double log_loss(double y, double p) {
    const double eps = 1e-15;
    p = fmin(fmax(p, eps), 1.0 - eps);
    return -(y * log(p) + (1.0 - y) * log(1.0 - p));
}

// Cada bloque escribe en parciales[blockIdx.x * (d+2) + k] la suma de su
// grad[0..d-1], grad_sesgo (k = d) y pérdida (k = d+1).
__global__ void gradiente_bloques(const double *X, const double *y, long long n, int d,
                                  const double *pesos, double sesgo, double pc0, double pc1,
                                  double *parciales) {
    extern __shared__ double compartida[];  // blockDim.x * (d+2)
    const int ancho = d + 2;
    double *mio = compartida + threadIdx.x * ancho;
    for (int k = 0; k < ancho; k++) mio[k] = 0.0;

    for (long long i = blockIdx.x * (long long)blockDim.x + threadIdx.x; i < n;
         i += (long long)gridDim.x * blockDim.x) {
        const double *xi = X + i * d;
        double z = sesgo;
        for (int j = 0; j < d; j++) z += pesos[j] * xi[j];
        double pred = sigmoide(z);
        double pc = (y[i] == 1.0) ? pc1 : pc0;
        double error = pc * (pred - y[i]);
        for (int j = 0; j < d; j++) mio[j] += error * xi[j];
        mio[d] += error;
        mio[d + 1] += pc * log_loss(y[i], pred);
    }
    __syncthreads();

    // Reducción en árbol dentro del bloque (blockDim.x es potencia de 2).
    for (int s = blockDim.x / 2; s > 0; s >>= 1) {
        if ((int)threadIdx.x < s) {
            double *otro = compartida + (threadIdx.x + s) * ancho;
            for (int k = 0; k < ancho; k++) mio[k] += otro[k];
        }
        __syncthreads();
    }
    if (threadIdx.x == 0) {
        for (int k = 0; k < ancho; k++) parciales[blockIdx.x * ancho + k] = compartida[k];
    }
}

// Partición cargada en la GPU y la configuración del kernel para ella.
struct Particion {
    int d = 0;
    long long n = 0;
    double pc[2] = {1, 1};
    int hilos = HILOS_POR_BLOQUE, bloques = 1;
    size_t bytesCompartida = 0;
    double *dX = nullptr, *dy = nullptr, *dPesos = nullptr, *dParciales = nullptr;
};

static void liberar(Particion &p) {
    cudaFree(p.dX);
    cudaFree(p.dy);
    cudaFree(p.dPesos);
    cudaFree(p.dParciales);
    p.dX = p.dy = p.dPesos = p.dParciales = nullptr;
}

// cargar lee una partición de stdin, la copia a la GPU y responde el
// estado (0 = lista). Devuelve false si no se puede seguir.
static bool cargar(Particion &p, const cudaDeviceProp &prop) {
    uint32_t d32;
    uint64_t n64;
    if (!leer(&d32, 4) || !leer(&n64, 8) || !leer(p.pc, sizeof p.pc)) {
        fprintf(stderr, "gradiente_cuda: no llegaron los datos\n");
        return false;
    }
    p.d = (int)d32;
    p.n = (long long)n64;
    const int ancho = p.d + 2;
    std::vector<double> X((size_t)p.n * p.d), y((size_t)p.n);
    if (!leer(X.data(), X.size() * sizeof(double)) || !leer(y.data(), y.size() * sizeof(double))) {
        fprintf(stderr, "gradiente_cuda: datos incompletos\n");
        return false;
    }

    p.hilos = HILOS_POR_BLOQUE;
    while (p.hilos > 32 && (size_t)p.hilos * ancho * sizeof(double) > prop.sharedMemPerBlock) p.hilos /= 2;
    long long necesarios = (p.n + p.hilos - 1) / p.hilos;
    p.bloques = (int)(necesarios < (long long)prop.multiProcessorCount * 8 ? necesarios : prop.multiProcessorCount * 8);
    if (p.bloques < 1) p.bloques = 1;
    p.bytesCompartida = (size_t)p.hilos * ancho * sizeof(double);

    uint32_t estado = 0;
    if (cudaMalloc(&p.dX, X.size() * sizeof(double) + 8) != cudaSuccess ||
        cudaMalloc(&p.dy, y.size() * sizeof(double) + 8) != cudaSuccess ||
        cudaMalloc(&p.dPesos, p.d * sizeof(double) + 8) != cudaSuccess ||
        cudaMalloc(&p.dParciales, (size_t)p.bloques * ancho * sizeof(double)) != cudaSuccess ||
        cudaMemcpy(p.dX, X.data(), X.size() * sizeof(double), cudaMemcpyHostToDevice) != cudaSuccess ||
        cudaMemcpy(p.dy, y.data(), y.size() * sizeof(double), cudaMemcpyHostToDevice) != cudaSuccess) {
        fprintf(stderr, "gradiente_cuda: no se pudieron copiar los datos a la GPU: %s\n",
                cudaGetErrorString(cudaGetLastError()));
        estado = 1;
    }
    escribir(&estado, 4);
    fflush(stdout);
    if (estado != 0) return false;
    fprintf(stderr, "gradiente_cuda: %lld ejemplos x %d variables en la GPU (%d bloques x %d hilos)\n", p.n, p.d, p.bloques, p.hilos);
    return true;
}

int main(int argc, char **argv) {
#ifdef _WIN32
    // Sin esto, Windows traduce \n a \r\n y corrompe el protocolo binario.
    _setmode(_fileno(stdin), _O_BINARY);
    _setmode(_fileno(stdout), _O_BINARY);
#endif
    int dispositivos = 0;
    cudaError_t err = cudaGetDeviceCount(&dispositivos);
    if (err != cudaSuccess || dispositivos == 0) {
        fprintf(stderr, "gradiente_cuda: no hay GPU CUDA disponible (%s)\n",
                err != cudaSuccess ? cudaGetErrorString(err) : "0 dispositivos");
        return 1;
    }
    cudaDeviceProp prop;
    CUDA(cudaGetDeviceProperties(&prop, 0));
    char nombre[512];
    snprintf(nombre, sizeof nombre, "%s, sm_%d%d, %d SM, %.1f GB", prop.name, prop.major, prop.minor,
             prop.multiProcessorCount, prop.totalGlobalMem / 1073741824.0);

    if (argc > 1 && strcmp(argv[1], "--info") == 0) {
        printf("GPU: %s\n", nombre);
        return 0;
    }

    // Presentación.
    uint32_t largo = (uint32_t)strlen(nombre);
    escribir("SACA", 4);
    escribir(&largo, 4);
    escribir(nombre, largo);
    fflush(stdout);

    // Datos de la partición. Se vuelven a leer con la orden 2 (otra corrida).
    Particion part;
    if (!cargar(part, prop)) return 4;
    for (;;) {
        uint32_t orden;
        if (!leer(&orden, 4) || orden == 0) break;
        if (orden == 2) {
            liberar(part);
            if (!cargar(part, prop)) return 4;
            continue;
        }
        const int d = part.d, ancho = d + 2, bloques = part.bloques, hilos = part.hilos;
        const long long n = part.n;
        const size_t bytesCompartida = part.bytesCompartida;
        double *dX = part.dX, *dy = part.dy, *dPesos = part.dPesos, *dParciales = part.dParciales;
        const double *pc = part.pc;
        std::vector<double> params(d + 1), parciales((size_t)bloques * ancho), res(ancho);
        if (!leer(params.data(), params.size() * sizeof(double))) break;

        std::fill(res.begin(), res.end(), 0.0);
        if (n > 0) {
            CUDA(cudaMemcpy(dPesos, params.data(), d * sizeof(double), cudaMemcpyHostToDevice));
            gradiente_bloques<<<bloques, hilos, bytesCompartida>>>(dX, dy, n, d, dPesos, params[d], pc[0], pc[1], dParciales);
            CUDA(cudaGetLastError());
            CUDA(cudaMemcpy(parciales.data(), dParciales, parciales.size() * sizeof(double), cudaMemcpyDeviceToHost));
            for (int b = 0; b < bloques; b++)
                for (int k = 0; k < ancho; k++) res[k] += parciales[(size_t)b * ancho + k];
        }
        escribir(res.data(), res.size() * sizeof(double));
        fflush(stdout);
    }

    liberar(part);
    return 0;
}
