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
{ "tipo": "registro", "worker_id": "w1" }
```

### `particion_asignada` (maestro → trabajador)

```json
{
  "tipo": "particion_asignada",
  "worker_id": "w1",
  "filtro_mongo": { "estacion_id_hash_mod": 4, "estacion_id_hash_resto": 0 }
}
```

`filtro_mongo` es el criterio que el trabajador usa para consultar su porción en MongoDB; el reparto es por hash de `estacion_id` para que cada trabajador cargue un subconjunto disjunto y estable entre épocas.

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
  "n_muestras": 214300
}
```

`gradiente` y `gradiente_sesgo` son la suma (no el promedio) del gradiente parcial sobre `n_muestras` filas; el maestro combina sumando y dividiendo entre el total de muestras de todos los trabajadores.

### `entrenamiento_finalizado` (maestro → trabajador)

```json
{ "tipo": "entrenamiento_finalizado", "epoca_final": 40, "perdida_final": 0.312 }
```

### `baja` (cualquiera → cualquiera)

```json
{ "tipo": "baja", "worker_id": "w1", "motivo": "cierre_normal" }
```

## Caída de un nodo

Si un trabajador se desconecta a mitad de una época, el maestro lo da de baja del registro activo:

- En modo síncrono, deja de esperar su `gradiente_parcial` para esa época y reparte de nuevo su partición entre los trabajadores restantes en la siguiente.
- En modo asíncrono, simplemente ya no llegan más gradientes de ese `worker_id`; no bloquea a los demás.

## Estado de implementación

- Semana 7: `internal/protocolo` (mensajes), `internal/cluster` (registro de nodos con mutex), `entrenamiento/cmd/maestro` y `entrenamiento/cmd/trabajador` implementan los pasos 1, 2 y 7: registro, asignación de partición por `internal/particion` (hash FNV-32a de `estacion_id`), rechazo de nodos sobrantes con `baja` (`motivo: registro_rechazado`) y detección de bajas y caídas.
- Semana 9: pasos 3 a 6 (difusión de `parametros`, `gradiente_parcial`, agregación síncrona).
