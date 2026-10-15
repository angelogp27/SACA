# Contrato — protocolo TCP maestro-trabajador (Parte B)

Define los mensajes que intercambian el maestro y cada nodo trabajador durante el entrenamiento distribuido de la regresión logística.

## Transporte y formato

- TCP, un mensaje JSON por línea (delimitado por `\n`), leído con `bufio.Scanner` en ambos extremos.
- Cada mensaje trae un campo `tipo` que identifica su forma; el resto de campos depende del tipo.

## Secuencia

1. El trabajador se conecta al maestro y envía `registro`.
2. El maestro responde `particion_asignada` con el filtro que ese trabajador debe usar para leer su porción de datos desde MongoDB.
3. El maestro difunde `parametros` a todos los trabajadores registrados al inicio de cada época.
4. Cada trabajador calcula el gradiente parcial sobre su partición y responde `gradiente_parcial`.
5. El maestro agrega los gradientes recibidos (según el modo, ver abajo), actualiza los parámetros y repite desde el paso 3 hasta convergencia.
6. Al terminar, el maestro envía `entrenamiento_finalizado` a todos los trabajadores.
7. Cualquiera de los dos lados puede cerrar la conexión enviando `baja` antes de desconectar.

## Modo de agregación

Se fija por configuración del maestro al lanzar el entrenamiento, no por mensaje:

- **Síncrono**: el maestro espera `gradiente_parcial` de todos los trabajadores registrados antes de actualizar y difundir la nueva época.
- **Asíncrono**: el maestro actualiza los parámetros apenas recibe cada `gradiente_parcial`, sin esperar al resto.

## Mensajes

### `registro` (trabajador → maestro)

```json
{ "tipo": "registro", "worker_id": "w1", "huella": "5e90db4b1158...", "motor": "cuda", "dispositivo": "NVIDIA GeForce RTX 3060, sm_86, 28 SM, 12.0 GB" }
```

- `huella` es el SHA-256 del archivo de la muestra (`data/muestra.gob`) que cargó el nodo. El maestro rechaza con `baja` (`motivo: muestra_distinta: ...`) a un nodo cuya huella no coincide con la suya: con otros datos, la suma de los gradientes no sería la del conjunto de entrenamiento.
- `motor` (`go` o `cuda`) y `dispositivo` (texto libre) son informativos, para la tabla de nodos del maestro.

### `particion_asignada` (maestro → trabajador)

```json
{
  "tipo": "particion_asignada",
  "worker_id": "w1",
  "filtro_mongo": { "estacion_id_hash_mod": 4, "estacion_id_hash_resto": 0 }
}
```

`filtro_mongo` es el criterio que el trabajador usa para quedarse con su porción: las estaciones cuyo hash FNV-32a de `estacion_id` módulo `mod` es `resto`. Así cada trabajador tiene un subconjunto disjunto y estable entre épocas. Pese al nombre, que se conserva para no romper el contrato, la implementación actual no consulta MongoDB: el trabajador filtra su copia local de la muestra (`distribuido.Filtrar`).

### `parametros` (maestro → trabajador)

```json
{
  "tipo": "parametros",
  "epoca": 7,
  "pesos": [0.12, -0.03, 0.44, "..."],
  "sesgo": -2.003
}
```

### `gradiente_parcial` (trabajador → maestro)

```json
{
  "tipo": "gradiente_parcial",
  "worker_id": "w1",
  "epoca": 7,
  "gradiente": [0.001, -0.0004, 0.002, "..."],
  "gradiente_sesgo": 0.0123,
  "perdida": 2810.44,
  "n_muestras": 214300,
  "t_calculo_ms": 3.21,
  "motor": "cuda"
}
```

- `gradiente`, `gradiente_sesgo` y `perdida` son **sumas** (no promedios) sobre las `n_muestras` filas de la partición. El maestro suma las de todos los trabajadores en el orden de las particiones y divide entre el total.
- Si la suma de `n_muestras` no da el tamaño del conjunto de entrenamiento, el maestro aborta.
- `t_calculo_ms` es el tiempo que el nodo dedicó a calcular, sin la red. El maestro lo usa para separar cálculo de comunicación.
- `motor` indica con qué se calculó realmente: si el motor CUDA no pasa la validación, el nodo cae a `go`.

### `entrenamiento_finalizado` (maestro → trabajador)

```json
{ "tipo": "entrenamiento_finalizado", "epoca_final": 40, "perdida_final": 0.312 }
```

### `baja` (cualquiera → cualquiera)

```json
{ "tipo": "baja", "worker_id": "w1", "motivo": "cierre_normal" }
```

## Caída de un nodo

Implementado hoy (modo síncrono): si un nodo se desconecta, no se le pueden enviar los parámetros o no envía su gradiente dentro de `-espera` (2 min por defecto), el maestro **aborta** el entrenamiento con un mensaje que nombra al nodo y la época, y despide a los demás con `baja`.

Diseño para más adelante: el maestro da de baja al nodo del registro activo y sigue.

- En modo síncrono, deja de esperar su `gradiente_parcial` para esa época y reparte de nuevo su partición entre los trabajadores restantes en la siguiente.
- En modo asíncrono, simplemente ya no llegan más gradientes de ese `worker_id`; no bloquea a los demás.

## Estado de implementación

- Semana 7: `internal/protocolo` (mensajes), `internal/cluster` (registro de nodos con mutex), `entrenamiento/cmd/maestro` y `entrenamiento/cmd/trabajador` implementan los pasos 1, 2 y 7: registro, asignación de partición por `internal/particion` (hash FNV-32a de `estacion_id`), rechazo de nodos sobrantes con `baja` (`motivo: registro_rechazado`) y detección de bajas y caídas.
- 15 de octubre de 2026: pasos 3 a 6 en modo síncrono (`internal/distribuido`):
  - difusión de `parametros`, `gradiente_parcial` y agregación;
  - campos `huella`, `motor`, `dispositivo`, `perdida` y `t_calculo_ms`;
  - rechazo por muestra distinta y aborto ante la caída de un nodo.

  Cada trabajador calcula con goroutines o con el motor CUDA (`nativo/cuda/gradiente_servidor.cu`), al que habla por stdin/stdout con el protocolo binario de `internal/distribuido/externo.go`. La pérdida final coincide con la del entrenamiento secuencial (diferencia ~1e-15).
- Pendiente: modo asíncrono y redistribución de la partición de un nodo caído (Experimento 4).
