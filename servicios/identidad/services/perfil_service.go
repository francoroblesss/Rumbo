package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/francoroblesss/Rumbo/servicios/identidad/repositories"
)

// ErrUsuarioNoEncontrado: el sub del token no es un usuario. Pasa, por ejemplo, con el token del
// sistema externo, cuyo sub es un client_id (docs/interfaces-internas.md §15).
var ErrUsuarioNoEncontrado = errors.New("usuario no encontrado")

// Perfil es lo que la web necesita del usuario que ingresó: su nombre, sus roles (para armar el
// menú) y su organización (para el encabezado). Nunca incluye la contraseña ni su hash.
type Perfil struct {
	ID           string              `json:"id"`
	Email        string              `json:"email"`
	Nombre       string              `json:"nombre"`
	Roles        []string            `json:"roles"`
	Organizacion *OrganizacionPerfil `json:"organizacion"` // null para el personal de Rumbo
}

// OrganizacionPerfil es la organización del usuario, resumida.
type OrganizacionPerfil struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
}

// PerfilService arma el perfil del usuario identificado por el gateway.
type PerfilService struct {
	usuarios       repositories.UsuariosRepository
	organizaciones repositories.OrganizacionesRepository
}

func NuevoPerfilService(usuarios repositories.UsuariosRepository, organizaciones repositories.OrganizacionesRepository) *PerfilService {
	return &PerfilService{usuarios: usuarios, organizaciones: organizaciones}
}

// Perfil devuelve el perfil del usuario con ese id (el X-Rumbo-Usuario que puso el gateway).
func (s *PerfilService) Perfil(ctx context.Context, usuarioID string) (*Perfil, error) {
	usuario, err := s.usuarios.BuscarPorID(ctx, usuarioID)
	if errors.Is(err, repositories.ErrNoEncontrado) {
		return nil, ErrUsuarioNoEncontrado
	}
	if err != nil {
		return nil, fmt.Errorf("buscar usuario: %w", err)
	}

	roles := usuario.Roles
	if roles == nil {
		roles = []string{} // en el JSON, [] y no null
	}
	perfil := &Perfil{ID: usuario.ID, Email: usuario.Email, Nombre: usuario.Nombre, Roles: roles}
	if usuario.OrganizacionID == nil {
		return perfil, nil
	}

	perfil.Organizacion = &OrganizacionPerfil{ID: *usuario.OrganizacionID}
	organizacion, err := s.organizaciones.BuscarPorID(ctx, *usuario.OrganizacionID)
	switch {
	case err == nil:
		perfil.Organizacion.Nombre = organizacion.Nombre
	case !errors.Is(err, repositories.ErrNoEncontrado):
		return nil, fmt.Errorf("buscar organización del usuario: %w", err)
	}
	// Si la organización no está, se devuelve igual su id con el nombre vacío: el usuario existe
	// y pertenece a ella, aunque no se pueda mostrar su nombre.
	return perfil, nil
}
