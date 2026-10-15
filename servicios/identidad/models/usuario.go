package models

import "time"

// Usuario es una persona que entra a Rumbo (personal de Rumbo o usuario de una organización
// cliente). La contraseña nunca se guarda en texto plano: solo su hash bcrypt.
type Usuario struct {
	ID             string    `bson:"_id" json:"id"` // "usr-<uuid>"
	Email          string    `bson:"email" json:"email"`
	HashPassword   string    `bson:"hash_password" json:"-"`
	Roles          []string  `bson:"roles" json:"roles"`
	OrganizacionID *string   `bson:"organizacion_id,omitempty" json:"organizacion_id,omitempty"`
	CreadoEn       time.Time `bson:"creado_en" json:"creado_en"`
}
