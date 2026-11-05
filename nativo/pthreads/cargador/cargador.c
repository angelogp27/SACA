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
 * No escribe en MongoDB: se compara con
 *     go run ./cargador/cmd/concurrente -sin-mongo -goroutines N -agregadoras K
 *
 * Compilar:  gcc -O3 -pthread -o bin/cargador_pthreads nativo/pthreads/cargador/cargador.c -lm
 * Ejecutar:  bin/cargador_pthreads --datos data/raw --hilos 8 --agregadoras 4 --resultado resultados/experimento5.csv
 */
#include <errno.h>
#include <fcntl.h>
#include <pthread.h>
#include <sys/resource.h>
#include <unistd.h>

#include "../../comun/saca.h"

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
    double valor;
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
    out->gas = (uint8_t)gas;
    out->poc = (uint8_t)(poc > 255 ? 255 : poc);
    return ACEPTADA;
}

/* ---------- agregadora: tabla hash de (estación, minuto) ---------- */

typedef struct {
    char est[16];
    int64_t minuto;
    double v[NUM_GASES];
    uint8_t poc[NUM_GASES];
    uint8_t tiene[NUM_GASES];
} Registro;

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
    /* Monitores repetidos: gana el POC más bajo, llegue en el orden que llegue. */
    if (r->tiene[v->gas]) {
        ix->duplicados++;
        if (v->poc >= r->poc[v->gas]) return;
    }
    r->tiene[v->gas] = 1;
    r->v[v->gas] = v->valor;
    r->poc[v->gas] = v->poc;
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
    int fd = open(b->ruta, O_RDONLY);
    if (fd < 0) saca_fallar("no se pudo abrir", b->ruta);
    int k = ctx->n_agregadoras;
    Lote *lotes = calloc((size_t)k, sizeof *lotes);
    for (int a = 0; a < k; a++) { lotes[a].v = malloc(LOTE * sizeof(Valor)); }

    size_t guardado = 0; /* bytes de una línea partida que quedó del trozo anterior */
    int64_t off = b->ini;
    while (off < b->fin) {
        size_t quiero = BUFFER_BYTES - guardado;
        if ((int64_t)quiero > b->fin - off) quiero = (size_t)(b->fin - off);
        ssize_t n = pread(fd, buf + guardado, quiero, off);
        if (n <= 0) saca_fallar("error de lectura en", b->ruta);
        off += n;
        size_t len = guardado + (size_t)n;
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
    close(fd);
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
static int64_t fin_de_linea(int fd, int64_t off, int64_t tam) {
    char b[65536];
    while (off < tam) {
        ssize_t n = pread(fd, b, sizeof b, off);
        if (n <= 0) break;
        char *nl = memchr(b, '\n', (size_t)n);
        if (nl) return off + (nl - b) + 1;
        off += n;
    }
    return tam;
}

static int64_t *limites_bloques(const char *ruta, int n) {
    int fd = open(ruta, O_RDONLY);
    if (fd < 0) saca_fallar("no se pudo abrir", ruta);
    int64_t tam = lseek(fd, 0, SEEK_END);
    int64_t inicio = fin_de_linea(fd, 0, tam);
    int64_t *lim = malloc((size_t)(n + 1) * sizeof *lim);
    lim[0] = inicio;
    for (int i = 1; i < n; i++) lim[i] = fin_de_linea(fd, inicio + (tam - inicio) * i / n, tam);
    lim[n] = tam;
    close(fd);
    return lim;
}

int main(int argc, char **argv) {
    const char *dir = saca_arg(argc, argv, "--datos", "data/raw");
    const char *anio = saca_arg(argc, argv, "--anio", "2025");
    const int hilos = atoi(saca_arg(argc, argv, "--hilos", "4"));
    int agregadoras = atoi(saca_arg(argc, argv, "--agregadoras", "1"));
    const int por_hilo = atoi(saca_arg(argc, argv, "--bloques-por-hilo", "4"));
    const char *resultado = saca_arg(argc, argv, "--resultado", "");
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

    struct rusage ru;
    getrusage(RUSAGE_SELF, &ru);
    const double memoria_mb = ru.ru_maxrss / 1024.0; /* pico de memoria residente, en KB en Linux */

    printf("total: leidas=%lld descartadas=%lld convertidas=%lld (", (long long)ctx.total.leidas,
           (long long)(ctx.total.leidas - ctx.total.por_motivo[ACEPTADA]), (long long)ctx.total.convertidas);
    for (int m = SIN_MEDICION; m < NUM_MOTIVOS; m++) printf("%s%s=%lld", m > SIN_MEDICION ? " " : "", NOMBRE_MOTIVO[m], (long long)ctx.total.por_motivo[m]);
    printf(")\nregistros unicos (estacion, hora): %lld (%lld valores de monitores repetidos resueltos por POC)\n", (long long)registros, (long long)duplicados);
    printf("%s con %d hilos y %d agregadoras: %.3f s (indexado de la agregadora más cargada %.3f s, pico de memoria %.0f MB)\n", modo, hilos,
           agregadoras, t_proc, t_indexado, memoria_mb);

    char fila[1024];
    snprintf(fila, sizeof fila, "%s,%d,%.3f,%.3f,0.000,%.3f,%lld,%lld,%.0f", modo, hilos, t_proc, t_indexado, t_proc,
             (long long)ctx.total.leidas, (long long)registros, memoria_mb);
    saca_csv_agregar(resultado, "modo,goroutines,t_procesamiento_s,t_indexado_s,t_mongo_s,t_total_s,filas_leidas,registros,memoria_mb", fila, maquina);
    (void)NOMBRE_GAS;
    return 0;
}
