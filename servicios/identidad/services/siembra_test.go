package services

import (
	"context"
	"slices"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/francoroblesss/Rumbo/servicios/identidad/models"
	"github.com/francoroblesss/Rumbo/servicios/identidad/repositories"
)

// Siembra de §15 de docs/interfaces-internas.md: organización, administrador, operador y cliente.

func TestSembrarCreaLaOrganizacionYLosTresUsuarios(t *testing.T) {
	_, usuarios, organizaciones := nuevoServicioPrueba(t)

	organizacion := organizaciones.PorID[OrganizacionExternaID]
	if organizacion == nil || organizacion.Nombre != "Gestión de almacenamiento" || organizacion.TipoServicio != models.ServicioStock {
		t.Fatalf("organización sembrada = %+v", organizacion)
	}

	casos := []struct {
		email, clave, nombre string
		roles                []string
		organizacion         string
	}{
		{siembraPrueba.AdminEmail, siembraPrueba.AdminPassword, "Administrador", []string{"administrador", "operador"}, ""},
		{siembraPrueba.OperadorEmail, siembraPrueba.OperadorPassword, "Operador de Rumbo", []string{"operador"}, ""},
		{siembraPrueba.ClienteEmail, siembraPrueba.ClientePassword, "Usuario de almacenamiento", []string{"cliente"}, OrganizacionExternaID},
	}
	for _, caso := range casos {
		usuario := usuarios.PorEmail[caso.email]
		if usuario == nil {
			t.Errorf("%s: no se sembró", caso.email)
			continue
		}
		if usuario.Nombre != caso.nombre || !slices.Equal(usuario.Roles, caso.roles) {
			t.Errorf("%s: nombre = %q, roles = %v", caso.email, usuario.Nombre, usuario.Roles)
		}
		organizacionUsuario := ""
		if usuario.OrganizacionID != nil {
			organizacionUsuario = *usuario.OrganizacionID
		}
		if organizacionUsuario != caso.organizacion {
			t.Errorf("%s: organización = %q, esperaba %q", caso.email, organizacionUsuario, caso.organizacion)
		}
		if usuario.HashPassword == caso.clave ||
			bcrypt.CompareHashAndPassword([]byte(usuario.HashPassword), []byte(caso.clave)) != nil {
			t.Errorf("%s: la contraseña tiene que guardarse como hash bcrypt", caso.email)
		}
	}
}

func TestSegundaSiembraNoCambiaNadaNiLaContrasenia(t *testing.T) {
	servicio, usuarios, organizaciones := nuevoServicioPrueba(t)
	antes := map[string]models.Usuario{}
	for email, usuario := range usuarios.PorEmail {
		antes[email] = *usuario
	}
	secretoAntes := organizaciones.PorID[OrganizacionExternaID].ClientesAPI[0].HashSecreto

	// Aunque cambien las contraseñas configuradas, un usuario que ya existe no se toca.
	otra := siembraPrueba
	otra.AdminPassword, otra.OperadorPassword, otra.ClientePassword = "nueva-1", "nueva-2", "nueva-3"
	otra.ExternoOrganizacionNombre = "Otro nombre"
	segunda := NuevoAuthService(usuarios, organizaciones, secretoPrueba, otra)
	for _, s := range []*AuthService{servicio, segunda} {
		if err := s.Sembrar(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	if len(usuarios.PorEmail) != 3 || len(organizaciones.PorID) != 1 {
		t.Fatalf("la segunda siembra duplicó datos: %d usuarios, %d organizaciones", len(usuarios.PorEmail), len(organizaciones.PorID))
	}
	for email, usuario := range usuarios.PorEmail {
		previo := antes[email]
		if usuario.ID != previo.ID || usuario.HashPassword != previo.HashPassword || usuario.Nombre != previo.Nombre {
			t.Errorf("%s cambió con la segunda siembra", email)
		}
	}
	organizacion := organizaciones.PorID[OrganizacionExternaID]
	if organizacion.Nombre != "Gestión de almacenamiento" || organizacion.ClientesAPI[0].HashSecreto != secretoAntes {
		t.Errorf("la organización cambió con la segunda siembra: %+v", organizacion)
	}
	if _, err := segunda.Login(context.Background(), siembraPrueba.OperadorEmail, siembraPrueba.OperadorPassword); err != nil {
		t.Errorf("el operador tiene que seguir entrando con su contraseña original: %v", err)
	}
}

func TestSinEmailDeOperadorNiDeClienteNoSeSiembran(t *testing.T) {
	usuarios := repositories.NuevoUsuariosMock()
	siembra := siembraPrueba
	siembra.OperadorEmail, siembra.OperadorPassword = "", ""
	siembra.ClienteEmail, siembra.ClientePassword = "", ""
	servicio := NuevoAuthService(usuarios, repositories.NuevoOrganizacionesMock(), secretoPrueba, siembra)
	if err := servicio.Sembrar(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(usuarios.PorEmail) != 1 || usuarios.PorEmail[siembraPrueba.AdminEmail] == nil {
		t.Fatalf("esperaba solo el administrador, hay %d usuarios", len(usuarios.PorEmail))
	}
}

func TestSembrarCompletaElNombreQueFaltaSinPisarLaContrasenia(t *testing.T) {
	usuarios := repositories.NuevoUsuariosMock()
	organizaciones := repositories.NuevoOrganizacionesMock()
	// Datos guardados antes de §15: sin nombre.
	hash, _ := bcrypt.GenerateFromPassword([]byte("clave-vieja"), bcrypt.MinCost)
	administrador := &models.Usuario{ID: "usr-viejo", Email: siembraPrueba.AdminEmail, HashPassword: string(hash),
		Roles: []string{"administrador", "operador"}, CreadoEn: time.Now().UTC()}
	_ = usuarios.Crear(context.Background(), administrador)
	_ = organizaciones.Crear(context.Background(), &models.Organizacion{ID: OrganizacionExternaID,
		TipoServicio: models.ServicioStock, CreadoEn: time.Now().UTC()})

	servicio := NuevoAuthService(usuarios, organizaciones, secretoPrueba, siembraPrueba)
	if err := servicio.Sembrar(context.Background()); err != nil {
		t.Fatal(err)
	}

	if administrador.Nombre != "Administrador" || administrador.HashPassword != string(hash) {
		t.Errorf("administrador = nombre %q; la contraseña se tiene que conservar", administrador.Nombre)
	}
	if nombre := organizaciones.PorID[OrganizacionExternaID].Nombre; nombre != "Gestión de almacenamiento" {
		t.Errorf("nombre de la organización = %q", nombre)
	}
}
