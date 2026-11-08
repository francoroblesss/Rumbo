// Package repositories define el acceso a la colección avisos en MongoDB. El _id de cada aviso
// deriva del id del mensaje que lo originó, así un evento repetido no duplica el aviso
// (ADR-003, ADR-027).
package repositories
