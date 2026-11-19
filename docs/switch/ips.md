# Switch: la IP de cada laptop

Cada laptop usa **una IP fija y distinta** en el cable, en la red `192.168.50.x`, **sin gateway** (así Internet sigue por Wi-Fi). Por convención, el maestro es la `.1`.

| # | Laptop (dueño) | Sistema | IP | Rol |
|---|---|---|---|---|
| 1 | _____ | _____ | `192.168.50.1` | Maestro |
| 2 | _____ | _____ | `192.168.50.2` | Nodo |
| 3 | _____ | _____ | `192.168.50.3` | Nodo |
| 4 | _____ | _____ | `192.168.50.4` | Nodo |
| 5 | _____ | _____ | `192.168.50.5` | Nodo |
| 6 | _____ | _____ | `192.168.50.6` | Nodo |
| 7 | _____ | _____ | `192.168.50.7` | Nodo |
| 8 | _____ | _____ | `192.168.50.8` | Nodo |

La IP no decide quién es el maestro (lo es quien pulse *Iniciar clúster*), pero mantener esta tabla evita que dos laptops usen la misma.

Siguiente: [maestro](maestro.md) o [nodo](nodo.md).
