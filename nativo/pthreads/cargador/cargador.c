/* Cargador de los CSV horarios de la EPA con Pthreads (semanas 5 y 6).
 *
 * Es el mismo diseño que cargador/cmd/concurrente de Go, con las
 * herramientas del curso en lugar de goroutines y channels:
 *
 *   despachador (hilo principal)
 *        │  cola de bloques (mutex + variables de condición)
 *        ▼
 *   N hilos trabajadores: leen un bloque de bytes, parten las líneas CSV,
 *        │  validan cada fila y la envían, en lotes, a la agregadora dueña
 *        │  de su estación (hash de la estación)
 *        ▼  K colas acotadas productor-consumidor (mutex + 2 condvars)
 *   K hilos agregadores: cada uno es el único dueño de su tabla hash, así
 *        que insertar no necesita ningún lock (diseño por propiedad).
 *
 * Sincronización:
 *   - mutex + variable de condición en las colas (productor-consumidor);
 *   - un mutex protege el conteo total de filas, tomado una vez por bloque
 *     y no por fila, para que la contención sea despreciable;
 *   - pthread_join es la barrera: los agregadores solo terminan cuando
 *     todos los trabajadores terminaron y las colas se cerraron.
 *
 * Reglas de limpieza: las de internal/aqs (filas sin medición, qualifier R?,
 * unidades ppm↔ppb, formato) y, ante monitores repetidos del mismo gas en
 * la misma (estación, hora), gana el de POC más bajo.
 *
 * Desempates deterministas (los mismos que internal/aqs.Indice.Agregar):
 * con el mismo POC gana el valor más bajo, y las coordenadas salen del
 * valor ganador del gas de mayor prioridad (NO2, O3, SO2, CO). Así el
 * resultado no depende del orden de llegada ni del número de hilos.
 *
 * No escribe en MongoDB: se compara con
 *     go run ./cargador/cmd/concurrente -sin-mongo -goroutines N -agregadoras K
 *
 * Con --bin ruta escribe además la muestra de entrenamiento (muestra.bin
 * v2, docs/contratos/contrato_formato_binario.md) byte a byte igual a la que escribe
 * el cargador de Go con -bin: misma huella SHA-256.
 *
 * Compilar (Linux):   gcc -O3 -ffp-contract=off -pthread -o bin/cargador_pthreads nativo/pthreads/cargador/cargador.c -lm
 * Compilar (Windows): x86_64-w64-mingw32-gcc -O3 -ffp-contract=off -o bin/cargador_pthreads.exe nativo/pthreads/cargador/cargador.c -static -lpthread -lm
 * -ffp-contract=off es obligatorio: sin él gcc puede fusionar a*b+c en una
 * FMA y la normalización cambiaría en el último bit (otra huella).
 * Ejecutar:  bin/cargador_pthreads --datos data/raw --hilos 8 --agregadoras 4 --bin data/muestra.bin --resultado resultados/experimento5.csv
 */
#ifndef _WIN32
#define _FILE_OFFSET_BITS 64
#endif
#include <errno.h>
#include <pthread.h>

#include "../../comun/saca.h"
#include "../../comun/tabla_ciclica.h"

#ifdef _WIN32
#include <psapi.h>
#define saca_fseek(f, off) _fseeki64((f), (off), SEEK_SET)
#else
#include <sys/resource.h>
#include <unistd.h>
#define saca_fseek(f, off) fseeko((f), (off_t)(off), SEEK_SET)
#endif

/* Mueve tmp a ruta, reemplazándola. En Windows un archivo que otro proceso
   tiene abierto (la página calculando su huella, un antivirus) no se puede
   reemplazar: se reintenta unos 15 s antes de rendirse. */
static int reemplazar(const char *tmp, const char *ruta) {
    for (int intento = 0; intento < 60; intento++) {
#ifdef _WIN32
        if (MoveFileExA(tmp, ruta, MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)) return 1;
        Sleep(250);
#else
        if (rename(tmp, ruta) == 0) return 1;
        usleep(250000);
#endif
    }
    remove(tmp);
    return 0;
}

/* Pico de memoria residente del proceso, en MB. */
static double memoria_pico_mb(void) {
#ifdef _WIN32
    PROCESS_MEMORY_COUNTERS pmc;
    if (K32GetProcessMemoryInfo(GetCurrentProcess(), &pmc, sizeof pmc)) return pmc.PeakWorkingSetSize / (1024.0 * 1024.0);
    return 0;
#else
    struct rusage ru;
    getrusage(RUSAGE_SELF, &ru);
    return ru.ru_maxrss / 1024.0; /* en KB en Linux */
#endif
}

/* Lee hasta n bytes desde off; cada bloque abre su propio FILE, así que no
 * hay posición compartida entre hilos. */
static size_t leer_en(FILE *f, char *dst, size_t n, int64_t off) {
    if (saca_fseek(f, off) != 0) return 0;
    return fread(dst, 1, n, f);
}

#define BUFFER_BYTES (4 << 20) /* lectura por trozos de 4 MB */
#define MAX_CAMPOS 24
#define LOTE 512               /* valores por mensaje a una agregadora */
#define CAPACIDAD_COLA 64      /* lotes en vuelo por agregadora */
#define NUM_GASES 4

enum { O3, NO2, SO2, CO };
static const char *NOMBRE_GAS[NUM_GASES] = {"o3", "no2", "so2", "co"};
static const char *CODIGO_GAS[NUM_GASES] = {"44201", "42602", "42401", "42101"};
static const int UNIDAD_PPM[NUM_GASES] = {1, 0, 0, 1}; /* unidad esperada: 1 = ppm, 0 = ppb */

enum { ACEPTADA, SIN_MEDICION, QUALIFIER, UNIDAD, FORMATO, NUM_MOTIVOS };
static const char *NOMBRE_MOTIVO[NUM_MOTIVOS] = {"aceptada", "sin_medicion", "qualifier_excluido", "unidad_desconocida", "formato_invalido"};

typedef struct {
    int64_t leidas, por_motivo[NUM_MOTIVOS], convertidas;
} Conteo;

/* Un valor ya validado, listo para unirse al registro de su (estación, minuto). */
typedef struct {
    char est[16];
    int64_t minuto; /* minutos desde 1970-01-01 UTC */
    double valor, lat, lon;
    uint8_t gas, poc;
} Valor;

typedef struct {
    Valor *v;
    int n;
} Lote;

/* ---------- cola acotada productor-consumidor ---------- */

typedef struct {
    pthread_mutex_t mu;
    pthread_cond_t no_llena, no_vacia;
    char *buf;
    size_t tam;
    int cap, ini, cnt, cerrada;
} Cola;

static void cola_iniciar(Cola *c, size_t tam, int cap) {
    pthread_mutex_init(&c->mu, NULL);
    pthread_cond_init(&c->no_llena, NULL);
    pthread_cond_init(&c->no_vacia, NULL);
    c->buf = malloc(tam * cap);
    c->tam = tam;
    c->cap = cap;
    c->ini = c->cnt = c->cerrada = 0;
    if (!c->buf) saca_fallar("sin memoria para una cola", NULL);
}

static void cola_poner(Cola *c, const void *item) {
    pthread_mutex_lock(&c->mu);
    while (c->cnt == c->cap) pthread_cond_wait(&c->no_llena, &c->mu);
    memcpy(c->buf + ((size_t)(c->ini + c->cnt) % c->cap) * c->tam, item, c->tam);
    c->cnt++;
    pthread_cond_signal(&c->no_vacia);
    pthread_mutex_unlock(&c->mu);
}

/* Devuelve 0 cuando la cola está cerrada y vacía. */
static int cola_sacar(Cola *c, void *item) {
    pthread_mutex_lock(&c->mu);
    while (c->cnt == 0 && !c->cerrada) pthread_cond_wait(&c->no_vacia, &c->mu);
    if (c->cnt == 0) {
        pthread_mutex_unlock(&c->mu);
        return 0;
    }
    memcpy(item, c->buf + (size_t)c->ini * c->tam, c->tam);
    c->ini = (c->ini + 1) % c->cap;
    c->cnt--;
    pthread_cond_signal(&c->no_llena);
    pthread_mutex_unlock(&c->mu);
    return 1;
}

static void cola_cerrar(Cola *c) {
    pthread_mutex_lock(&c->mu);
    c->cerrada = 1;
    pthread_cond_broadcast(&c->no_vacia);
    pthread_mutex_unlock(&c->mu);
}

/* ---------- validación de una fila (internal/aqs.procesarFila) ---------- */

/* Parte una línea CSV (campos entre comillas, "" como comilla escapada) en
 * sitio; linea[n] debe poder escribirse. Devuelve cuántos campos hay o -1. */
static int partir_csv(char *linea, size_t n, char **campos) {
    char *p = linea, *fin = linea + n;
    int k = 0;
    while (p <= fin) {
        if (k >= MAX_CAMPOS) return -1;
        if (p < fin && *p == '"') {
            p++;
            char *w = p;
            campos[k++] = w;
            for (;;) {
                if (p >= fin) return -1;
                if (*p == '"') {
                    if (p + 1 < fin && p[1] == '"') { *w++ = '"'; p += 2; continue; }
                    p++;
                    break;
                }
                *w++ = *p++;
            }
            *w = '\0';
            if (p == fin) return k;
            if (*p != ',') return -1;
            p++;
        } else {
            campos[k++] = p;
            while (p < fin && *p != ',') p++;
            *p = '\0';
            if (p == fin) return k;
            p++;
        }
    }
    return k;
}

static int numero(const char *s, double *dst) {
    char *fin;
    errno = 0;
    *dst = strtod(s, &fin);
    return fin != s && *fin == '\0';
}

/* Días desde 1970-01-01 de una fecha civil (algoritmo de Howard Hinnant). */
static int64_t dias_civil(int y, int m, int d) {
    y -= m <= 2;
    int64_t era = (y >= 0 ? y : y - 399) / 400;
    int64_t yoe = y - era * 400;
    int64_t doy = (153 * (m + (m > 2 ? -3 : 9)) + 2) / 5 + d - 1;
    int64_t doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    return era * 146097 + doe - 719468;
}

static int minuto_utc(const char *fecha, const char *hora, int64_t *dst) {
    int y, mo, d, h, mi;
    if (strlen(fecha) != 10 || fecha[4] != '-' || fecha[7] != '-' || strlen(hora) != 5 || hora[2] != ':') return 0;
    if (sscanf(fecha, "%4d-%2d-%2d", &y, &mo, &d) != 3 || sscanf(hora, "%2d:%2d", &h, &mi) != 2) return 0;
    if (mo < 1 || mo > 12 || d < 1 || d > 31 || h < 0 || h > 23 || mi < 0 || mi > 59) return 0;
    *dst = dias_civil(y, mo, d) * 1440 + h * 60 + mi;
    return 1;
}

/* Columnas del CSV de AQS (las mismas constantes que internal/aqs). */
enum { C_STATE = 0, C_COUNTY = 1, C_SITE = 2, C_POC = 4, C_LAT = 5, C_LON = 6, C_DATE_GMT = 11, C_TIME_GMT = 12,
       C_MEDICION = 13, C_UNIDAD = 14, C_QUALIFIER = 17 };

static int procesar_fila(char **f, int gas, Valor *out, int *convertida) {
    *convertida = 0;
    if (f[C_MEDICION][0] == '\0') return SIN_MEDICION;
    const char *q = f[C_QUALIFIER];
    if (q[0] == 'R' && q[1] >= 'A' && q[1] <= 'Z') return QUALIFIER;

    double valor, lat, lon;
    if (!numero(f[C_MEDICION], &valor) || !numero(f[C_LAT], &lat) || !numero(f[C_LON], &lon)) return FORMATO;

    int es_ppm = strcmp(f[C_UNIDAD], "Parts per million") == 0;
    int es_ppb = strcmp(f[C_UNIDAD], "Parts per billion") == 0;
    if (UNIDAD_PPM[gas]) {
        if (es_ppb) { valor /= 1000.0; *convertida = 1; } else if (!es_ppm) return UNIDAD;
    } else {
        if (es_ppm) { valor *= 1000.0; *convertida = 1; } else if (!es_ppb) return UNIDAD;
    }

    int64_t minuto;
    char *fin;
    long poc = strtol(f[C_POC], &fin, 10);
    if (!minuto_utc(f[C_DATE_GMT], f[C_TIME_GMT], &minuto) || fin == f[C_POC] || *fin != '\0' || poc < 0) return FORMATO;

    snprintf(out->est, sizeof out->est, "%s-%s-%s", f[C_STATE], f[C_COUNTY], f[C_SITE]);
    out->minuto = minuto;
    out->valor = valor;
    out->lat = lat;
    out->lon = lon;
    out->gas = (uint8_t)gas;
    out->poc = (uint8_t)(poc > 255 ? 255 : poc);
    return ACEPTADA;
}

/* ---------- agregadora: tabla hash de (estación, minuto) ---------- */

typedef struct {
    char est[16];
    int64_t minuto;
    double v[NUM_GASES];
    double lat, lon;
    uint8_t poc[NUM_GASES];
    uint8_t tiene[NUM_GASES];
    uint8_t fuente; /* prioridad+1 del gas del que salieron lat/lon; 0 = ninguno */
} Registro;

/* Prioridad de cada gas (O3, NO2, SO2, CO) para elegir las coordenadas:
 * internal/aqs.PrioridadCoord. */
static const uint8_t PRIORIDAD_COORD[NUM_GASES] = {1, 0, 2, 3};

typedef struct {
    Registro *reg; /* registros en orden de llegada */
    size_t n, cap_reg;
    uint32_t *tabla; /* índice+1 en reg; 0 = vacía */
    size_t cap_tabla;
    int64_t duplicados;
} Indice;

static uint64_t hash_est(const char *s) { /* FNV-1a */
    uint64_t h = 1469598103934665603ULL;
    for (; *s; s++) h = (h ^ (unsigned char)*s) * 1099511628211ULL;
    return h;
}

static uint64_t hash_clave(const char *est, int64_t minuto) {
    uint64_t h = hash_est(est) ^ ((uint64_t)minuto * 0x9E3779B97F4A7C15ULL);
    h ^= h >> 32;
    h *= 0xD6E8FEB86659FD93ULL;
    h ^= h >> 32;
    return h;
}

static void indice_iniciar(Indice *ix) {
    memset(ix, 0, sizeof *ix);
    ix->cap_tabla = 1 << 16;
    ix->tabla = calloc(ix->cap_tabla, sizeof *ix->tabla);
    if (!ix->tabla) saca_fallar("sin memoria para el índice", NULL);
}

static void indice_crecer_tabla(Indice *ix) {
    size_t nueva = ix->cap_tabla * 2;
    uint32_t *t = calloc(nueva, sizeof *t);
    if (!t) saca_fallar("sin memoria para el índice", NULL);
    for (size_t i = 0; i < ix->n; i++) {
        size_t pos = hash_clave(ix->reg[i].est, ix->reg[i].minuto) & (nueva - 1);
        while (t[pos]) pos = (pos + 1) & (nueva - 1);
        t[pos] = (uint32_t)(i + 1);
    }
    free(ix->tabla);
    ix->tabla = t;
    ix->cap_tabla = nueva;
}

static void indice_agregar(Indice *ix, const Valor *v) {
    size_t pos = hash_clave(v->est, v->minuto) & (ix->cap_tabla - 1);
    Registro *r = NULL;
    while (ix->tabla[pos]) {
        Registro *c = &ix->reg[ix->tabla[pos] - 1];
        if (c->minuto == v->minuto && strcmp(c->est, v->est) == 0) { r = c; break; }
        pos = (pos + 1) & (ix->cap_tabla - 1);
    }
    if (!r) {
        if (ix->n == ix->cap_reg) {
            ix->cap_reg = ix->cap_reg ? ix->cap_reg * 2 : 1 << 16;
            ix->reg = realloc(ix->reg, ix->cap_reg * sizeof *ix->reg);
            if (!ix->reg) saca_fallar("sin memoria para los registros", NULL);
        }
        r = &ix->reg[ix->n];
        memset(r, 0, sizeof *r);
        memcpy(r->est, v->est, sizeof r->est);
        r->minuto = v->minuto;
        ix->tabla[pos] = (uint32_t)(++ix->n);
        if (ix->n * 2 > ix->cap_tabla) indice_crecer_tabla(ix);
    }
    /* Monitores repetidos: gana el POC más bajo y, con el mismo POC, el
     * valor más bajo, llegue en el orden que llegue. */
    const int g = v->gas;
    const uint8_t fuente = (uint8_t)(PRIORIDAD_COORD[g] + 1);
    if (r->tiene[g]) {
        ix->duplicados++;
        if (v->poc < r->poc[g] || (v->poc == r->poc[g] && v->valor < r->v[g])) {
            /* gana el nuevo valor */
        } else if (v->poc == r->poc[g] && v->valor == r->v[g] && r->fuente == fuente &&
                   (v->lat < r->lat || (v->lat == r->lat && v->lon < r->lon))) {
            r->lat = v->lat; /* mismo valor: solo cambian las coordenadas */
            r->lon = v->lon;
            return;
        } else {
            return;
        }
    }
    r->tiene[g] = 1;
    r->v[g] = v->valor;
    r->poc[g] = v->poc;
    if (r->fuente == 0 || fuente <= r->fuente) {
        r->lat = v->lat;
        r->lon = v->lon;
        r->fuente = fuente;
    }
}

static Registro *indice_buscar(const Indice *ix, const char *est, int64_t minuto) {
    size_t pos = hash_clave(est, minuto) & (ix->cap_tabla - 1);
    while (ix->tabla[pos]) {
        Registro *c = &ix->reg[ix->tabla[pos] - 1];
        if (c->minuto == minuto && strcmp(c->est, est) == 0) return c;
        pos = (pos + 1) & (ix->cap_tabla - 1);
    }
    return NULL;
}

/* ---------- avance (--progreso) ---------- */

/* Milésimas de avance: la lectura llega a 800 y armar la muestra a 1000.
 * Lo escriben los trabajadores (suma atómica de bytes) y el hilo
 * principal; lo lee el hilo informante. */
static int64_t bytes_total = 1, bytes_leidos = 0, avance = 0;
static int informar = 0, informante_fin = 0;

static void sumar_bytes(int64_t n) {
    int64_t hechos = __atomic_add_fetch(&bytes_leidos, n, __ATOMIC_RELAXED);
    __atomic_store_n(&avance, 800 * hechos / bytes_total, __ATOMIC_RELAXED);
}

static void fijar_avance(int64_t milesimas) { __atomic_store_n(&avance, milesimas, __ATOMIC_RELAXED); }

static void dormir_ms(int ms) {
#ifdef _WIN32
    Sleep((DWORD)ms);
#else
    struct timespec t = {ms / 1000, (long)(ms % 1000) * 1000000L};
    nanosleep(&t, NULL);
#endif
}

static void *informante(void *arg) {
    (void)arg;
    while (!__atomic_load_n(&informante_fin, __ATOMIC_ACQUIRE)) {
        printf("SACA_PROGRESO %.4f\n", __atomic_load_n(&avance, __ATOMIC_RELAXED) / 1000.0);
        fflush(stdout);
        dormir_ms(250);
    }
    printf("SACA_PROGRESO %.4f\n", __atomic_load_n(&avance, __ATOMIC_RELAXED) / 1000.0);
    fflush(stdout);
    return NULL;
}

/* ---------- hilos ---------- */

typedef struct {
    const char *ruta;
    int gas;
    int64_t ini, fin;
} Bloque;

typedef struct {
    Cola bloques;
    Cola *salidas; /* una por agregadora */
    int n_agregadoras;
    pthread_mutex_t mu_total;
    Conteo total;
} Contexto;

typedef struct {
    Contexto *ctx;
} ArgTrabajador;

typedef struct {
    Cola *entrada;
    Indice indice;
    double t_indexado;
} Agregadora;

static void enviar_lote(Cola *c, Lote *l) {
    if (l->n > 0) cola_poner(c, l);
    l->v = malloc(LOTE * sizeof(Valor));
    l->n = 0;
    if (!l->v) saca_fallar("sin memoria para un lote", NULL);
}

/* Procesa las líneas completas de buf[0..len) y devuelve cuántos bytes consumió. */
static size_t procesar_lineas(char *buf, size_t len, int final, int gas, Conteo *c, Lote *lotes, Cola *salidas, int k) {
    size_t pos = 0;
    while (pos < len) {
        char *nl = memchr(buf + pos, '\n', len - pos);
        size_t fin_linea;
        if (nl) fin_linea = (size_t)(nl - buf);
        else if (final) fin_linea = len;
        else break;
        size_t largo = fin_linea - pos;
        char *linea = buf + pos;
        pos = nl ? fin_linea + 1 : len;
        if (largo && linea[largo - 1] == '\r') largo--;
        if (largo == 0) continue; /* igual que encoding/csv: las líneas vacías se saltan */
        /* partir_csv escribe un '\0' en linea[largo]: es el '\n' (o el fin del buffer, que tiene 1 byte de holgura). */
        char *campos[MAX_CAMPOS];
        c->leidas++;
        Valor v;
        int convertida = 0, motivo;
        if (partir_csv(linea, largo, campos) != MAX_CAMPOS) motivo = FORMATO;
        else motivo = procesar_fila(campos, gas, &v, &convertida);
        c->por_motivo[motivo]++;
        if (motivo != ACEPTADA) continue;
        c->convertidas += convertida;
        int a = (int)(hash_est(v.est) % (uint64_t)k);
        Lote *l = &lotes[a];
        l->v[l->n++] = v;
        if (l->n == LOTE) enviar_lote(&salidas[a], l);
    }
    return pos;
}

static Conteo procesar_bloque(const Bloque *b, Contexto *ctx, char *buf) {
    Conteo c;
    memset(&c, 0, sizeof c);
    FILE *fd = fopen(b->ruta, "rb");
    if (!fd) saca_fallar("no se pudo abrir", b->ruta);
    int k = ctx->n_agregadoras;
    Lote *lotes = calloc((size_t)k, sizeof *lotes);
    for (int a = 0; a < k; a++) { lotes[a].v = malloc(LOTE * sizeof(Valor)); }

    size_t guardado = 0; /* bytes de una línea partida que quedó del trozo anterior */
    int64_t off = b->ini;
    while (off < b->fin) {
        size_t quiero = BUFFER_BYTES - guardado;
        if ((int64_t)quiero > b->fin - off) quiero = (size_t)(b->fin - off);
        size_t n = leer_en(fd, buf + guardado, quiero, off);
        if (n == 0) saca_fallar("error de lectura en", b->ruta);
        off += (int64_t)n;
        if (informar) sumar_bytes((int64_t)n);
        size_t len = guardado + n;
        int final = off >= b->fin;
        size_t usado = procesar_lineas(buf, len, final, b->gas, &c, lotes, ctx->salidas, k);
        guardado = len - usado;
        if (guardado >= BUFFER_BYTES - 1) saca_fallar("una línea no cabe en el buffer de lectura", b->ruta);
        memmove(buf, buf + usado, guardado);
    }
    for (int a = 0; a < k; a++) {
        if (lotes[a].n > 0) cola_poner(&ctx->salidas[a], &lotes[a]);
        else free(lotes[a].v);
    }
    free(lotes);
    fclose(fd);
    return c;
}

static void *trabajador(void *arg) {
    Contexto *ctx = ((ArgTrabajador *)arg)->ctx;
    char *buf = malloc(BUFFER_BYTES + 1);
    if (!buf) saca_fallar("sin memoria para el buffer de lectura", NULL);
    Bloque b;
    while (cola_sacar(&ctx->bloques, &b)) {
        Conteo c = procesar_bloque(&b, ctx, buf);
        /* Sección crítica: una vez por bloque, no por fila. */
        pthread_mutex_lock(&ctx->mu_total);
        ctx->total.leidas += c.leidas;
        ctx->total.convertidas += c.convertidas;
        for (int m = 0; m < NUM_MOTIVOS; m++) ctx->total.por_motivo[m] += c.por_motivo[m];
        pthread_mutex_unlock(&ctx->mu_total);
    }
    free(buf);
    return NULL;
}

static void *agregadora(void *arg) {
    Agregadora *ag = arg;
    Lote l;
    while (cola_sacar(ag->entrada, &l)) {
        double t = saca_ahora();
        for (int i = 0; i < l.n; i++) indice_agregar(&ag->indice, &l.v[i]);
        ag->t_indexado += saca_ahora() - t;
        free(l.v);
    }
    return NULL;
}

/* ---------- bloques (internal/aqs.LimitesBloques) ---------- */

/* Offset del byte siguiente al primer '\n' a partir de off (o el fin). */
static int64_t fin_de_linea(FILE *fd, int64_t off, int64_t tam) {
    char b[65536];
    while (off < tam) {
        size_t n = leer_en(fd, b, sizeof b, off);
        if (n == 0) break;
        char *nl = memchr(b, '\n', (size_t)n);
        if (nl) return off + (nl - b) + 1;
        off += (int64_t)n;
    }
    return tam;
}

static int64_t *limites_bloques(const char *ruta, int n) {
    FILE *fd = fopen(ruta, "rb");
    if (!fd) saca_fallar("no se pudo abrir", ruta);
#ifdef _WIN32
    _fseeki64(fd, 0, SEEK_END);
    int64_t tam = _ftelli64(fd);
#else
    fseeko(fd, 0, SEEK_END);
    int64_t tam = (int64_t)ftello(fd);
#endif
    int64_t inicio = fin_de_linea(fd, 0, tam);
    int64_t *lim = malloc((size_t)(n + 1) * sizeof *lim);
    lim[0] = inicio;
    for (int i = 1; i < n; i++) lim[i] = fin_de_linea(fd, inicio + (tam - inicio) * i / n, tam);
    lim[n] = tam;
    fclose(fd);
    return lim;
}

/* ---------- muestra.bin v2 (internal/muestrabin) ---------- */

#define D_VARIABLES 20 /* dataset.NombresFeatures: 12 base + 8 de la serie del NO2 */
#define VENTANA_HORAS 24
static const int REZAGOS[6] = {1, 2, 3, 6, 12, 24};
#define MES_CORTE 10
#define UMBRAL_NO2 53.0

typedef struct {
    char est[16];
    int64_t hora; /* horas desde 1970-01-01 UTC */
    int mes;
    double x[D_VARIABLES];
    double y;
} Fila;

typedef struct {
    const Indice *ix;
    Fila *filas;
    size_t n;
} Etiquetado;

/* Mes (1-12) del día z desde 1970-01-01 (Howard Hinnant, inversa de dias_civil). */
static int mes_de_dia(int64_t z) {
    z += 719468;
    int64_t era = (z >= 0 ? z : z - 146096) / 146097;
    int64_t doe = z - era * 146097;
    int64_t yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    int64_t doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    int64_t mp = (5 * doy + 2) / 153;
    return (int)(mp < 10 ? mp + 3 : mp - 9);
}

/* Cada hilo etiqueta los registros de una agregadora: la lectura de la hora
 * siguiente de la misma estación está siempre en el mismo índice, así que
 * solo lee su propia tabla y no necesita ningún lock. */
static void *etiquetar(void *arg) {
    Etiquetado *e = arg;
    const Indice *ix = e->ix;
    e->filas = malloc((ix->n ? ix->n : 1) * sizeof(Fila));
    if (!e->filas) saca_fallar("sin memoria para la muestra", NULL);
    e->n = 0;
    for (size_t i = 0; i < ix->n; i++) {
        const Registro *r = &ix->reg[i];
        if (!r->tiene[NO2]) continue;
        const Registro *fut = indice_buscar(ix, r->est, r->minuto + 60);
        if (!fut || !fut->tiene[NO2]) continue;
        Fila *f = &e->filas[e->n++];
        memset(f, 0, sizeof *f);
        memcpy(f->est, r->est, strnlen(r->est, sizeof f->est - 1)); /* sin la basura que haya tras el \0 */
        const int64_t dia = r->minuto / 1440;
        const int hora = (int)((r->minuto / 60) % 24), dsem = (int)((dia + 4) % 7); /* 1970-01-01 fue jueves */
        f->hora = r->minuto / 60;
        f->mes = mes_de_dia(dia);
        /* El mismo orden que dataset.NombresFeatures. */
        f->x[0] = r->v[NO2];
        f->x[1] = r->tiene[O3] ? r->v[O3] : 0;
        f->x[2] = r->tiene[SO2] ? r->v[SO2] : 0;
        f->x[3] = r->tiene[CO] ? r->v[CO] : 0;
        f->x[4] = r->lat;
        f->x[5] = r->lon;
        f->x[6] = SACA_SIN_HORA[hora];
        f->x[7] = SACA_COS_HORA[hora];
        f->x[8] = SACA_SIN_DIA[dsem];
        f->x[9] = SACA_COS_DIA[dsem];
        f->x[10] = SACA_SIN_MES[f->mes];
        f->x[11] = SACA_COS_MES[f->mes];
        /* Serie propia del NO2 (dataset.VariablesRezago): t-1..t-24; una hora
         * sin medición toma el valor actual. Media y máximo de las 24 h
         * anteriores, sumando de t-1 a t-24 como en Go. */
        const double actual = r->v[NO2];
        for (int j = 0; j < 6; j++) {
            const Registro *p = indice_buscar(ix, r->est, r->minuto - 60LL * REZAGOS[j]);
            f->x[12 + j] = (p && p->tiene[NO2]) ? p->v[NO2] : actual;
        }
        double suma = 0, maximo = 0;
        int n = 0;
        for (int k = 1; k <= VENTANA_HORAS; k++) {
            const Registro *p = indice_buscar(ix, r->est, r->minuto - 60LL * k);
            if (!p || !p->tiene[NO2]) continue;
            const double v = p->v[NO2];
            if (n == 0 || v > maximo) maximo = v;
            suma += v;
            n++;
        }
        f->x[18] = n ? suma / (double)n : actual;
        f->x[19] = n ? maximo : actual;
        f->y = fut->v[NO2] > UMBRAL_NO2 ? 1.0 : 0.0;
    }
    return NULL;
}

static int comparar_filas(const void *a, const void *b) {
    const Fila *x = a, *y = b;
    int c = strcmp(x->est, y->est);
    if (c) return c;
    return (x->hora > y->hora) - (x->hora < y->hora);
}

static void escribir_f64(FILE *f, const double *v, size_t n) {
    if (n && fwrite(v, 8, n, f) != n) saca_fallar("no se pudo escribir la muestra", NULL);
}

/* Etiqueta en paralelo (un hilo por agregadora), ordena por (estación,
 * hora), normaliza en una pasada secuencial y escribe el archivo. */
static void escribir_muestra(const char *ruta, Agregadora *ags, int k) {
    const double t0 = saca_ahora();
    /* Fin de la limpieza, empieza la preparación de variables (el nodo mide
     * las dos fases por separado). */
    if (informar) {
        printf("SACA_FASE variables\n");
        fflush(stdout);
    }
    Etiquetado *et = calloc((size_t)k, sizeof *et);
    pthread_t *th = malloc((size_t)k * sizeof(pthread_t));
    for (int a = 0; a < k; a++) {
        et[a].ix = &ags[a].indice;
        pthread_create(&th[a], NULL, etiquetar, &et[a]);
    }
    size_t n = 0;
    for (int a = 0; a < k; a++) {
        pthread_join(th[a], NULL);
        n += et[a].n;
    }
    Fila *filas = malloc((n ? n : 1) * sizeof(Fila));
    if (!filas) saca_fallar("sin memoria para la muestra", NULL);
    size_t pos = 0;
    for (int a = 0; a < k; a++) {
        memcpy(filas + pos, et[a].filas, et[a].n * sizeof(Fila));
        pos += et[a].n;
        free(et[a].filas);
    }
    free(et);
    free(th);
    fijar_avance(850);
    qsort(filas, n, sizeof(Fila), comparar_filas);

    /* Separación por mes y tabla de estaciones (ya en orden). */
    const size_t d = D_VARIABLES;
    size_t n_ent = 0, n_est = 0;
    for (size_t i = 0; i < n; i++) {
        if (filas[i].mes < MES_CORTE) n_ent++;
        if (i == 0 || strcmp(filas[i].est, filas[i - 1].est) != 0) n_est++;
    }
    const size_t n_pru = n - n_ent;
    if (n_ent == 0 || n_pru == 0) saca_fallar("la partición temporal dejó un conjunto vacío", NULL);
    double *Xe = malloc(n_ent * d * 8), *ye = malloc(n_ent * 8), *Xp = malloc(n_pru * d * 8), *yp = malloc(n_pru * 8);
    uint32_t *ee = malloc(n_ent * 4), *ep = malloc(n_pru * 4);
    int64_t *he = malloc(n_ent * 8), *hp = malloc(n_pru * 8); /* hora de cada fila */
    char *ids = calloc(n_est ? n_est : 1, 16);
    if (!Xe || !ye || !Xp || !yp || !ee || !ep || !he || !hp || !ids) saca_fallar("sin memoria para la muestra", NULL);
    size_t ie = 0, ip = 0;
    int64_t est = -1;
    for (size_t i = 0; i < n; i++) {
        if (i == 0 || strcmp(filas[i].est, filas[i - 1].est) != 0) memcpy(ids + 16 * (size_t)(++est), filas[i].est, 16);
        if (filas[i].mes < MES_CORTE) {
            memcpy(Xe + ie * d, filas[i].x, d * 8);
            ye[ie] = filas[i].y;
            he[ie] = filas[i].hora;
            ee[ie++] = (uint32_t)est;
        } else {
            memcpy(Xp + ip * d, filas[i].x, d * 8);
            yp[ip] = filas[i].y;
            hp[ip] = filas[i].hora;
            ep[ip++] = (uint32_t)est;
        }
    }
    free(filas);
    fijar_avance(900);

    /* Normalización: el mismo recorrido y las mismas operaciones que
     * muestrabin.Normalizar (por eso -ffp-contract=off). */
    double medias[D_VARIABLES] = {0}, desv[D_VARIABLES] = {0};
    for (size_t i = 0; i < n_ent; i++)
        for (size_t j = 0; j < d; j++) medias[j] += Xe[i * d + j];
    for (size_t j = 0; j < d; j++) medias[j] /= (double)n_ent;
    for (size_t i = 0; i < n_ent; i++)
        for (size_t j = 0; j < d; j++) {
            double dif = Xe[i * d + j] - medias[j];
            desv[j] += dif * dif;
        }
    for (size_t j = 0; j < d; j++) {
        desv[j] = sqrt(desv[j] / (double)n_ent);
        if (desv[j] == 0) desv[j] = 1;
    }
    for (size_t i = 0; i < n_ent * d; i++) Xe[i] = (Xe[i] - medias[i % d]) / desv[i % d];
    for (size_t i = 0; i < n_pru * d; i++) Xp[i] = (Xp[i] - medias[i % d]) / desv[i % d];
    fijar_avance(950);

    /* Pesos de clase: logistica.CalcularPesosClase. */
    double positivos = 0;
    for (size_t i = 0; i < n_ent; i++) positivos += ye[i] == 1.0;
    const double nn = (double)n_ent, negativos = nn - positivos;
    const double pc1 = positivos > 0 ? nn / (2 * positivos) : 1, pc0 = negativos > 0 ? nn / (2 * negativos) : 1;

    char tmp[1100];
    snprintf(tmp, sizeof tmp, "%s.tmp", ruta);
    FILE *f = fopen(tmp, "wb");
    if (!f) saca_fallar("no se pudo crear", tmp);
    setvbuf(f, NULL, _IOFBF, 1 << 20);
    unsigned char enc[48] = {0};
    const uint32_t version = 2, tipo = 1, dd = (uint32_t)d;
    const uint64_t ne = n_ent, np = n_pru, nest = n_est;
    memcpy(enc, "SACA", 4);
    memcpy(enc + 4, &version, 4);
    memcpy(enc + 8, &tipo, 4);
    memcpy(enc + 12, &dd, 4);
    memcpy(enc + 16, &ne, 8);
    memcpy(enc + 24, &np, 8);
    memcpy(enc + 32, &pc0, 8);
    memcpy(enc + 40, &pc1, 8);
    int ok = fwrite(enc, 1, 48, f) == 48;
    escribir_f64(f, Xe, n_ent * d);
    escribir_f64(f, ye, n_ent);
    escribir_f64(f, Xp, n_pru * d);
    escribir_f64(f, yp, n_pru);
    escribir_f64(f, medias, d);
    escribir_f64(f, desv, d);
    ok = ok && fwrite(&nest, 8, 1, f) == 1;
    ok = ok && fwrite(ids, 16, n_est, f) == n_est;
    ok = ok && fwrite(ee, 4, n_ent, f) == n_ent;
    ok = ok && fwrite(ep, 4, n_pru, f) == n_pru;
    ok = ok && fwrite(he, 8, n_ent, f) == n_ent; /* horas (Fase 5) */
    ok = ok && fwrite(hp, 8, n_pru, f) == n_pru;
    if (fclose(f) != 0 || !ok) saca_fallar("no se pudo escribir", tmp);
    if (!reemplazar(tmp, ruta)) saca_fallar("no se pudo reemplazar (¿otro programa lo tiene abierto?)", ruta);
    free(Xe), free(ye), free(Xp), free(yp), free(ee), free(ep), free(he), free(hp), free(ids);
    fijar_avance(1000);
    fprintf(stderr, "muestra %s: %zu de entrenamiento, %zu de prueba, %zu estaciones (%.3f s)\n", ruta, n_ent, n_pru, n_est, saca_ahora() - t0);
}

int main(int argc, char **argv) {
    const char *dir = saca_arg(argc, argv, "--datos", "data/raw");
    const char *anio = saca_arg(argc, argv, "--anio", "2025");
    const int hilos = atoi(saca_arg(argc, argv, "--hilos", "4"));
    int agregadoras = atoi(saca_arg(argc, argv, "--agregadoras", "1"));
    const int por_hilo = atoi(saca_arg(argc, argv, "--bloques-por-hilo", "4"));
    const char *resultado = saca_arg(argc, argv, "--resultado", "");
    const char *ruta_bin = saca_arg(argc, argv, "--bin", "");
    informar = saca_flag(argc, argv, "--progreso");
    char maquina[256];
    saca_maquina_por_defecto(maquina, sizeof maquina);
    const char *m_arg = saca_arg(argc, argv, "--maquina", NULL);
    if (m_arg) snprintf(maquina, sizeof maquina, "%s", m_arg);
    if (hilos < 1 || por_hilo < 1 || agregadoras < 0) saca_fallar("--hilos y --bloques-por-hilo deben ser >= 1 y --agregadoras >= 0", NULL);
    char modo[64];
    if (agregadoras == 1) snprintf(modo, sizeof modo, "pthreads");
    else if (agregadoras == 0) snprintf(modo, sizeof modo, "pthreads_particionado");
    else snprintf(modo, sizeof modo, "pthreads_agreg%d", agregadoras);
    if (agregadoras == 0) agregadoras = hilos;

    const double t0 = saca_ahora();

    Contexto ctx;
    memset(&ctx, 0, sizeof ctx);
    ctx.n_agregadoras = agregadoras;
    ctx.salidas = malloc((size_t)agregadoras * sizeof(Cola));
    pthread_mutex_init(&ctx.mu_total, NULL);

    /* Bloques de todos los archivos. */
    const int por_archivo = hilos * por_hilo;
    cola_iniciar(&ctx.bloques, sizeof(Bloque), NUM_GASES * por_archivo);
    char rutas[NUM_GASES][1024];
    for (int g = 0; g < NUM_GASES; g++) {
        snprintf(rutas[g], sizeof rutas[g], "%s/hourly_%s_%s.csv", dir, CODIGO_GAS[g], anio);
        int64_t *lim = limites_bloques(rutas[g], por_archivo);
        for (int i = 0; i < por_archivo; i++) {
            Bloque b = {rutas[g], g, lim[i], lim[i + 1]};
            cola_poner(&ctx.bloques, &b);
        }
        free(lim);
    }
    cola_cerrar(&ctx.bloques);
    pthread_t th_inf;
    if (informar) {
        bytes_total = 0;
        for (int g = 0; g < NUM_GASES; g++) {
            FILE *f = fopen(rutas[g], "rb");
            if (!f) saca_fallar("no se pudo abrir", rutas[g]);
#ifdef _WIN32
            _fseeki64(f, 0, SEEK_END);
            bytes_total += _ftelli64(f);
#else
            fseeko(f, 0, SEEK_END);
            bytes_total += (int64_t)ftello(f);
#endif
            fclose(f);
        }
        if (bytes_total < 1) bytes_total = 1;
        pthread_create(&th_inf, NULL, informante, NULL);
    }
    fprintf(stderr, "%d bloques repartidos entre %d hilos trabajadores y %d agregadoras\n", NUM_GASES * por_archivo, hilos, agregadoras);

    Agregadora *ags = calloc((size_t)agregadoras, sizeof *ags);
    pthread_t *th_ag = malloc((size_t)agregadoras * sizeof(pthread_t));
    for (int a = 0; a < agregadoras; a++) {
        cola_iniciar(&ctx.salidas[a], sizeof(Lote), CAPACIDAD_COLA);
        ags[a].entrada = &ctx.salidas[a];
        indice_iniciar(&ags[a].indice);
        pthread_create(&th_ag[a], NULL, agregadora, &ags[a]);
    }
    pthread_t *th = malloc((size_t)hilos * sizeof(pthread_t));
    ArgTrabajador arg = {&ctx};
    for (int i = 0; i < hilos; i++) pthread_create(&th[i], NULL, trabajador, &arg);

    for (int i = 0; i < hilos; i++) pthread_join(th[i], NULL); /* barrera: todos los bloques procesados */
    for (int a = 0; a < agregadoras; a++) cola_cerrar(&ctx.salidas[a]);
    for (int a = 0; a < agregadoras; a++) pthread_join(th_ag[a], NULL);

    const double t_proc = saca_ahora() - t0;
    double t_indexado = 0.0;
    int64_t registros = 0, duplicados = 0;
    for (int a = 0; a < agregadoras; a++) {
        if (ags[a].t_indexado > t_indexado) t_indexado = ags[a].t_indexado; /* la agregadora más cargada */
        registros += (int64_t)ags[a].indice.n;
        duplicados += ags[a].indice.duplicados;
    }

    fijar_avance(800);
    if (ruta_bin[0]) escribir_muestra(ruta_bin, ags, agregadoras);
    else fijar_avance(1000);
    if (informar) {
        __atomic_store_n(&informante_fin, 1, __ATOMIC_RELEASE);
        pthread_join(th_inf, NULL);
    }
    const double t_total = saca_ahora() - t0;
    const double memoria_mb = memoria_pico_mb();

    printf("total: leidas=%lld descartadas=%lld convertidas=%lld (", (long long)ctx.total.leidas,
           (long long)(ctx.total.leidas - ctx.total.por_motivo[ACEPTADA]), (long long)ctx.total.convertidas);
    for (int m = SIN_MEDICION; m < NUM_MOTIVOS; m++) printf("%s%s=%lld", m > SIN_MEDICION ? " " : "", NOMBRE_MOTIVO[m], (long long)ctx.total.por_motivo[m]);
    printf(")\nregistros unicos (estacion, hora): %lld (%lld valores de monitores repetidos resueltos por POC)\n", (long long)registros, (long long)duplicados);
    printf("%s con %d hilos y %d agregadoras: %.3f s (indexado de la agregadora más cargada %.3f s, pico de memoria %.0f MB)\n", modo, hilos,
           agregadoras, t_proc, t_indexado, memoria_mb);

    char fila[1024];
    snprintf(fila, sizeof fila, "%s,%d,%.3f,%.3f,0.000,%.3f,%lld,%lld,%.0f", modo, hilos, t_proc, t_indexado, t_total,
             (long long)ctx.total.leidas, (long long)registros, memoria_mb);
    saca_csv_agregar(resultado, "modo,goroutines,t_procesamiento_s,t_indexado_s,t_mongo_s,t_total_s,filas_leidas,registros,memoria_mb", fila, maquina);
    (void)NOMBRE_GAS;
    return 0;
}
