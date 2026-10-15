// Emulación mínima de CUDA en la CPU para probar nativo/cuda/gradiente_servidor.cu
// en PCs sin GPU NVIDIA: un std::thread por hilo CUDA, std::barrier como
// __syncthreads() y malloc/memcpy como memoria de la GPU. Sirve para
// comprobar el protocolo con el trabajador Go y la reducción del kernel;
// los tiempos NO significan nada. Lo usa nativo/cuda/emulacion/compilar.ps1.
#pragma once
#include <barrier>
#include <cmath>
#include <cstdlib>
#include <cstring>
#include <thread>
#include <vector>
#define __global__
#define __device__
struct dim3 { unsigned x = 1, y = 1, z = 1; };
inline thread_local dim3 threadIdx, blockIdx;
inline dim3 blockDim, gridDim;
inline thread_local std::barrier<>* barrera_actual;
inline std::vector<char> memoria_compartida_bloque;
inline void __syncthreads() { barrera_actual->arrive_and_wait(); }
typedef int cudaError_t;
enum { cudaSuccess = 0 };
enum cudaMemcpyKind { cudaMemcpyHostToDevice, cudaMemcpyDeviceToHost };
inline const char* cudaGetErrorString(cudaError_t) { return "error emulado"; }
inline cudaError_t cudaGetLastError() { return 0; }
inline cudaError_t cudaGetDeviceCount(int* n) { *n = 1; return 0; }
struct cudaDeviceProp { char name[256]; int major, minor, multiProcessorCount; size_t totalGlobalMem, sharedMemPerBlock; };
inline cudaError_t cudaGetDeviceProperties(cudaDeviceProp* p, int) { strcpy(p->name, "GPU emulada en CPU"); p->major = 8; p->minor = 6; p->multiProcessorCount = 2; p->totalGlobalMem = 1ull << 32; p->sharedMemPerBlock = 48 * 1024; return 0; }
template <class T> cudaError_t cudaMalloc(T** p, size_t b) { *p = (T*)malloc(b); return 0; }
inline cudaError_t cudaFree(void* p) { free(p); return 0; }
inline cudaError_t cudaMemcpy(void* d, const void* s, size_t b, cudaMemcpyKind) { memcpy(d, s, b); return 0; }
// Un hilo de sistema por hilo CUDA; los bloques se ejecutan uno tras otro.
template <class F, class... A> void lanzar(int bloques, int hilos, size_t compartida, F kernel, A... args) {
  blockDim.x = hilos; gridDim.x = bloques;
  for (int b = 0; b < bloques; b++) {
    memoria_compartida_bloque.assign(compartida, 0);
    std::barrier<> barrera(hilos);
    std::vector<std::thread> ts;
    for (int t = 0; t < hilos; t++) ts.emplace_back([&, t, b] { threadIdx.x = t; blockIdx.x = b; barrera_actual = &barrera; kernel(args...); });
    for (auto& th : ts) th.join();
  }
}
