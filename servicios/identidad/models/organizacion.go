package models

import "time"

// Tipos de servicio de una organización cliente (dominio.md §2).
const (
	ServicioPacientes = "pacientes"
	ServicioEscolar   = "escolar"
	ServicioEmpresa   = "empresa"
	ServicioStock     = "stock"
)

// ClienteAPI son las credenciales con las que un sistema externo pide tokens (client credentials).
// El secreto se guarda solo como hash bcrypt.
type ClienteAPI struct {
	ClientID    string `bson:"client_id" json:"client_id"`
	HashSecreto string `bson:"hash_secreto" json:"-"`
}

// Organizacion es un cliente de Rumbo, con su tipo de servicio y, si es un sistema externo,
// sus credenciales de API embebidas.
type Organizacion struct {
	ID           string       `bson:"_id" json:"id"`
	Nombre       string       `bson:"nombre" json:"nombre"`
	TipoServicio string       `bson:"tipo_servicio" json:"tipo_servicio"`
	ClientesAPI  []ClienteAPI `bson:"clientes_api,omitempty" json:"clientes_api,omitempty"`
	CreadoEn     time.Time    `bson:"creado_en" json:"creado_en"`
}
