# Plan de aprendizaje — Proyecto 9

Orden sugerido, según cuándo lo pide el cronograma (ver `proyecto9.md`).

1. **Go básico.** Sintaxis, structs, interfaces, slices/maps, manejo de errores, módulos (`go.mod`). Requisito para todo lo demás.
2. **Concurrencia en Go.** Goroutines, channels, `select`, `sync.WaitGroup`, `sync.Mutex`, `sync/atomic`. Patrones: worker pool, fan-out, fan-in. Correr siempre con `go run -race`. Es el corazón de la Parte A y de la sección crítica de la Parte B.
3. **Regresión logística y descenso de gradiente.** Solo lo esencial: sigmoide, log loss, y que el gradiente es una suma sobre ejemplos independientes (por eso se puede repartir entre nodos).
4. **Procesar archivos grandes sin cargarlos en memoria.** `bufio.Scanner`/`bufio.Reader`, partir un archivo en bloques de bytes ajustados al salto de línea. Diseño literal de la Parte A.
5. **`net` y TCP en Go.** `net.Listen`, `net.Dial`, serialización (JSON o `encoding/gob`). Para el protocolo maestro-trabajador de la Parte B (no urgente, recién semana 7-9).
6. **MongoDB y Redis desde Go.** `mongo-driver` (`InsertMany` por lotes), `go-redis` (GET/SET/TTL). Necesario desde semana 5-6.
7. **Docker y Docker Compose.** `Dockerfile`, `docker-compose.yml`, redes entre contenedores, réplicas. Conviene adelantarlo, se usa desde el principio para levantar Mongo/Redis.
8. **JWT y WebSocket en Go.** `golang-jwt/jwt`, `gorilla/websocket`. Para la Parte C, semana 9-11.

Bibliografía ya sugerida por el curso para 1 y 2: *Learn Concurrent Programming with Go* (Cutajar, Manning).
