// Package puertos define los contratos del núcleo con el exterior:
//   - de entrada: las capacidades que ofrece Operaciones (los casos de uso);
//   - de salida: lo que el núcleo necesita (repositorios, disponibilidad de Recursos, datos de
//     personas de Identidad, clima, publicación de eventos).
//
// Los adaptadores implementan o invocan estos contratos; el núcleo nunca depende de ellos.
package puertos
