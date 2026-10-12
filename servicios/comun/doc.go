// Package comun agrupa las piezas que comparten todos los servicios de Rumbo y que no son
// lógica de negocio: observabilidad, mensajería, identidad propagada por el gateway y errores
// en formato Problem Details. Su contrato está en docs/interfaces-internas.md.
//
// Se escribió sobre librerías existentes (OpenTelemetry, amqp091-go, Gin, uuid): acá solo
// vive el pegamento mínimo para que todos los servicios hagan lo mismo de la misma forma.
package comun
