/* Puente MPI (semanas 3 y 4): la comunicación entre laptops con MPI.
 *
 * El maestro lanza con mpiexec un puente por nodo. Cada uno se conecta al
 * trabajador Go de su laptop (127.0.0.1, su puerto de pares) y hace por él
 * las operaciones colectivas con los puentes de las otras:
 *
 *     trabajador Go <-TCP local-> puente <-MPI-> puente <-TCP local-> trabajador Go
 *
 *   orden 1 (Allreduce): u64 n | f64 v[n]      -> f64 suma[n]        (todos)
 *   orden 2 (Gatherv):   u64 bytes | datos     -> rango 0: u64 cuantos[p] | datos de todos
 *   orden 0 o conexión cerrada: terminar.
 *
 * Uso: mpiexec ... puente_mpi <corrida> <puerto del rango 0> <puerto del rango 1> ...
 * Compilar (Linux):   mpicc -O2 -o bin/puente_mpi nativo/mpi/puente/puente.c
 * Compilar (Windows, MSYS2 UCRT64 con el paquete msmpi):
 *   gcc -O2 -o bin/puente_mpi.exe nativo/mpi/puente/puente.c -lmsmpi -lws2_32
 */
#include <mpi.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#ifdef _WIN32
#include <winsock2.h>
#include <ws2tcpip.h>
#define cerrar_socket closesocket
#else
#include <arpa/inet.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <sys/socket.h>
#include <unistd.h>
#define SOCKET int
#define INVALID_SOCKET (-1)
#define cerrar_socket close
#endif

#define RANGO_PUENTE 0xFFFFFFFFu /* el saludo con que Go reconoce al puente */
#define MAX_VALORES 100000000ull

static SOCKET s;

static void abortar(const char *que) {
    fprintf(stderr, "puente_mpi: %s\n", que);
    MPI_Abort(MPI_COMM_WORLD, 2);
}

static int leer(void *dst, size_t n) {
    char *p = (char *)dst;
    while (n > 0) {
        int k = (int)recv(s, p, n > (1 << 20) ? (1 << 20) : (int)n, 0);
        if (k <= 0) return 0;
        p += k;
        n -= (size_t)k;
    }
    return 1;
}

static void escribir(const void *src, size_t n) {
    const char *p = (const char *)src;
    while (n > 0) {
        int k = (int)send(s, p, n > (1 << 20) ? (1 << 20) : (int)n, 0);
        if (k <= 0) abortar("se perdió la conexión con el trabajador");
        p += k;
        n -= (size_t)k;
    }
}

static void *crecer(void *p, size_t *tiene, size_t quiere) {
    if (quiere <= *tiene) return p;
    free(p);
    p = malloc(quiere);
    if (!p) abortar("sin memoria");
    *tiene = quiere;
    return p;
}

int main(int argc, char **argv) {
    MPI_Init(&argc, &argv);
    int rango, n;
    MPI_Comm_rank(MPI_COMM_WORLD, &rango);
    MPI_Comm_size(MPI_COMM_WORLD, &n);
    if (argc != n + 2) abortar("se esperaba: puente_mpi <corrida> <un puerto por rango>");
#ifdef _WIN32
    WSADATA wsa;
    WSAStartup(MAKEWORD(2, 2), &wsa);
#endif
    struct sockaddr_in dir;
    memset(&dir, 0, sizeof dir);
    dir.sin_family = AF_INET;
    dir.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    dir.sin_port = htons((unsigned short)atoi(argv[2 + rango]));
    s = socket(AF_INET, SOCK_STREAM, 0);
    if (s == INVALID_SOCKET || connect(s, (struct sockaddr *)&dir, sizeof dir) != 0)
        abortar("no se pudo conectar con el trabajador de esta laptop");
    int uno = 1;
    setsockopt(s, IPPROTO_TCP, TCP_NODELAY, (const char *)&uno, sizeof uno);
    uint32_t saludo[2] = {(uint32_t)strtoul(argv[1], NULL, 10), RANGO_PUENTE};
    escribir(saludo, 8);

    char *buf = NULL, *todo = NULL;
    size_t nbuf = 0, ntodo = 0;
    int *cuantos = (int *)malloc(sizeof(int) * (size_t)n), *despl = (int *)malloc(sizeof(int) * (size_t)n);
    /* A todos los trabajadores les llega lo mismo en el mismo orden, así que
     * todos los puentes entran a la misma operación colectiva. */
    uint32_t orden = 0;
    while (leer(&orden, 4) && orden != 0) {
        uint64_t k;
        if (!leer(&k, 8) || k > MAX_VALORES) abortar("mensaje inválido del trabajador");
        if (orden == 1) {
            buf = (char *)crecer(buf, &nbuf, (size_t)k * 8 + 8);
            if (!leer(buf, (size_t)k * 8)) abortar("el trabajador cerró a mitad de un mensaje");
            MPI_Allreduce(MPI_IN_PLACE, buf, (int)k, MPI_DOUBLE, MPI_SUM, MPI_COMM_WORLD);
            escribir(buf, (size_t)k * 8);
        } else if (orden == 2) {
            buf = (char *)crecer(buf, &nbuf, (size_t)k + 8);
            if (!leer(buf, (size_t)k)) abortar("el trabajador cerró a mitad de un mensaje");
            int mio = (int)k;
            MPI_Gather(&mio, 1, MPI_INT, cuantos, 1, MPI_INT, 0, MPI_COMM_WORLD);
            int64_t total = 0;
            if (rango == 0) {
                for (int i = 0; i < n; i++) {
                    if (total > INT32_MAX) abortar("demasiados datos para Gatherv");
                    despl[i] = (int)total;
                    total += cuantos[i];
                }
                todo = (char *)crecer(todo, &ntodo, (size_t)total + 8);
            }
            MPI_Gatherv(buf, mio, MPI_BYTE, todo, cuantos, despl, MPI_BYTE, 0, MPI_COMM_WORLD);
            if (rango == 0) {
                for (int i = 0; i < n; i++) {
                    uint64_t c = (uint64_t)cuantos[i];
                    escribir(&c, 8);
                }
                escribir(todo, (size_t)total);
            }
        } else {
            abortar("orden desconocida");
        }
    }
    cerrar_socket(s);
    MPI_Finalize();
    return 0;
}
