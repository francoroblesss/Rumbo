// Package mysql es el adaptador de salida de persistencia: repositorios sobre la base
// operaciones en MySQL, bloqueo por recurso (SELECT ... FOR UPDATE) contra asignaciones
// superpuestas y outbox en la misma transacción (ADR-003, ADR-005).
package mysql
