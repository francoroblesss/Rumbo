// Package dominio es el núcleo de Operaciones (arquitectura hexagonal): solicitudes, traslados,
// franjas y la regla de clima, con sus estados e invariantes. No importa Gin, database/sql,
// RabbitMQ ni ningún adaptador; sus structs no llevan tags json ni db.
package dominio
