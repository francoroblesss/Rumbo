# ADR-005 — MySQL como motor relacional (reemplaza parcialmente a ADR-003)

- **Fecha:** 2026-10-11
- **Estado:** aceptada
- **Reemplaza a:** la elección de **PostgreSQL** de [ADR-003](ADR-003-persistencia.md) para Operaciones y Recursos. El resto de ADR-003 sigue vigente: tipo de almacenamiento por servicio, MongoDB, una base por microservicio, patrones de acceso y limitaciones.

## Contexto
En la corrección de la Entrega 1, el profesor indicó **cambiar PostgreSQL por MySQL, por ser más simple**. ADR-003 había elegido PostgreSQL sobre todo por las *exclusion constraints* sobre rangos de tiempo: la base misma impedía guardar dos asignaciones superpuestas del mismo chofer o vehículo. MySQL no tiene ese mecanismo, así que hay que decidir cómo se mantiene esa garantía.

El teórico 2 ubica a MySQL y PostgreSQL en el mismo lugar: bases relacionales para relaciones, JOINs, ACID estricto ("finanzas, reservas") y auditoría. Entre los criterios de elección incluye la **madurez del equipo**, y el equipo ya trabajó con MySQL en Desarrollo de Software (SQL Workbench es parte del stack de la materia).

## Decisión
- **MySQL 8.4** (motor InnoDB) para **Operaciones** y **Recursos**, con **una base de datos por microservicio** (`operaciones`, `recursos`) y **un usuario por servicio** que solo puede acceder a su base. Es una instancia en Docker Compose, publicada en el puerto **13306** del host para no chocar con un MySQL instalado localmente.
- **Acceso desde Go:** `database/sql` con el driver `github.com/go-sql-driver/mysql`. Las consultas son SQL explícito, así se ven claramente la transacción y el bloqueo de la operación crítica.
- **Garantía contra asignaciones superpuestas** (antes la daba la exclusion constraint):
  1. Cada recurso asignable (chofer y vehículo) tiene una **fila de bloqueo** en una tabla de Operaciones.
  2. La asignación corre en **una transacción InnoDB** que primero toma el bloqueo de las filas del chofer y del vehículo con `SELECT … FOR UPDATE`.
  3. Con el bloqueo tomado, verifica que no haya otra asignación activa superpuesta (incluido el margen mínimo) e inserta la nueva.
  4. Dos operadores que asignan el mismo recurso a la vez quedan **en fila**: el segundo espera, ve la asignación del primero y recibe un rechazo.
  - El diseño completo de la operación crítica (orden en que se toman los bloqueos para evitar *deadlocks*, tiempo de espera y reintento) se documenta en **D4**.
- El **outbox** de eventos y las **claves de idempotencia** se siguen guardando en la misma transacción local que el cambio de negocio (ADR-004). Eso no depende del motor.

## Alternativas consideradas
- **Mantener PostgreSQL** (ADR-003). Descartada por la indicación del profesor. Tenía una garantía más declarativa, pero suma un motor que el equipo conoce menos.
- **GORM** (el ORM que se usó en DSW). Descartada: la operación crítica depende de una transacción y un bloqueo explícitos, y con un ORM quedan más escondidos y son más difíciles de defender y de testear.
- **Bloqueo con `GET_LOCK()`** (bloqueos con nombre de MySQL). Descartada: el bloqueo no forma parte de la transacción y hay que liberarlo a mano. Si el proceso falla entre medio, queda más frágil que `SELECT … FOR UPDATE`, que se libera solo al confirmar o deshacer.
- **Restricción única sobre franjas fijas** (por ejemplo, bloques de 15 minutos). Descartada: obliga a discretizar los horarios del dominio y complica el margen variable entre recorridos.

## Consecuencias
- **Ganamos:**
  - un motor más simple y conocido por el equipo;
  - una sola herramienta relacional en todo el sistema;
  - la operación crítica sigue siendo atómica y local a Operaciones (ADR-001).
- **Pagamos:**
  - **la garantía de no superposición pasa a depender del código** (transacción + bloqueo) y ya no de una restricción de la base. Un error en ese código podría permitir superposiciones, por eso es obligatorio un **test de integración de concurrencia contra MySQL real**;
  - los bloqueos serializan las asignaciones del mismo recurso: es aceptable, porque compiten pocos operadores sobre el mismo chofer o vehículo;
  - hay que cuidar el orden de los bloqueos para evitar deadlocks (D4).

## Cómo se verifica
- Cada servicio se conecta con su usuario y recibe `Access denied` al intentar entrar a la base del otro. **Verificado** el 11/10/2026 con `docker compose`.
- **Test de concurrencia (D4):** N asignaciones simultáneas del mismo chofer a recorridos superpuestos → exactamente una confirmada.
