package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/francoroblesss/Rumbo/servicios/identidad/repositories"
)

const secretoPrueba = "secreto-de-prueba-bastante-largo"

var siembraPrueba = Siembra{
	AdminEmail:          "admin@rumbo.local",
	AdminPassword:       "clave-admin-web",
	ExternoClientID:     "cliente-externo",
	ExternoClientSecret: "secreto-externo",
}

func nuevoServicioPrueba(t *testing.T) (*AuthService, *repositories.UsuariosMock, *repositories.OrganizacionesMock) {
	t.Helper()
	usuarios := repositories.NuevoUsuariosMock()
	organizaciones := repositories.NuevoOrganizacionesMock()
	servicio := NuevoAuthService(usuarios, organizaciones, secretoPrueba, siembraPrueba)
	if err := servicio.Sembrar(context.Background()); err != nil {
		t.Fatal(err)
	}
	return servicio, usuarios, organizaciones
}

// validar verifica firma, emisor, audiencia y vencimiento como lo hace el gateway.
func validar(t *testing.T, token string, secreto string) *reclamos {
	t.Helper()
	r := &reclamos{}
	_, err := jwt.ParseWithClaims(token, r, func(*jwt.Token) (any, error) { return []byte(secreto), nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(EmisorToken),
		jwt.WithAudience(AudienciaToken), jwt.WithExpirationRequired())
	if err != nil {
		t.Fatalf("el token no es válido: %v", err)
	}
	return r
}

func TestLoginEmiteTokenConLosClaimsCorrectos(t *testing.T) {
	servicio, _, _ := nuevoServicioPrueba(t)

	token, err := servicio.Login(context.Background(), siembraPrueba.AdminEmail, siembraPrueba.AdminPassword)
	if err != nil {
		t.Fatal(err)
	}
	if token.TokenType != "Bearer" || token.ExpiraEnSegundos != 3600 {
		t.Fatalf("respuesta inesperada: %+v", token)
	}

	r := validar(t, token.AccessToken, secretoPrueba)
	if !strings.HasPrefix(r.Subject, "usr-") {
		t.Errorf("sub = %q, esperaba un id de usuario usr-<uuid>", r.Subject)
	}
	if len(r.Roles) != 2 || r.Roles[0] != "administrador" || r.Roles[1] != "operador" {
		t.Errorf("roles = %v", r.Roles)
	}
	if r.Org != "" {
		t.Errorf("el administrador no tiene organización, org = %q", r.Org)
	}
	if duracion := r.ExpiresAt.Sub(r.IssuedAt.Time); duracion != time.Hour {
		t.Errorf("el token tiene que durar 1 h, dura %v", duracion)
	}
}

func TestTokenClienteEmiteTokenDelSistemaExterno(t *testing.T) {
	servicio, _, _ := nuevoServicioPrueba(t)

	token, err := servicio.TokenCliente(context.Background(), siembraPrueba.ExternoClientID, siembraPrueba.ExternoClientSecret)
	if err != nil {
		t.Fatal(err)
	}

	r := validar(t, token.AccessToken, secretoPrueba)
	if r.Subject != siembraPrueba.ExternoClientID {
		t.Errorf("sub = %q, esperaba el client_id", r.Subject)
	}
	if r.Org != OrganizacionExternaID {
		t.Errorf("org = %q, esperaba %q", r.Org, OrganizacionExternaID)
	}
	if len(r.Roles) != 1 || r.Roles[0] != "sistema_externo" {
		t.Errorf("roles = %v", r.Roles)
	}
}

func TestElTokenNoValidaConOtroSecreto(t *testing.T) {
	servicio, _, _ := nuevoServicioPrueba(t)
	token, err := servicio.Login(context.Background(), siembraPrueba.AdminEmail, siembraPrueba.AdminPassword)
	if err != nil {
		t.Fatal(err)
	}
	r := &reclamos{}
	_, err = jwt.ParseWithClaims(token.AccessToken, r, func(*jwt.Token) (any, error) { return []byte("otro"), nil })
	if err == nil {
		t.Fatal("un token firmado con el secreto del servicio no puede validar con otro secreto")
	}
}

func TestCredencialesInvalidasDevuelvenSiempreElMismoError(t *testing.T) {
	servicio, _, _ := nuevoServicioPrueba(t)
	ctx := context.Background()

	_, errClave := servicio.Login(ctx, siembraPrueba.AdminEmail, "clave-equivocada")
	_, errUsuario := servicio.Login(ctx, "nadie@rumbo.local", "cualquiera")
	_, errSecreto := servicio.TokenCliente(ctx, siembraPrueba.ExternoClientID, "secreto-equivocado")
	_, errCliente := servicio.TokenCliente(ctx, "cliente-inexistente", "cualquiera")

	for nombre, err := range map[string]error{
		"contraseña equivocada": errClave, "usuario inexistente": errUsuario,
		"secreto equivocado": errSecreto, "client_id inexistente": errCliente,
	} {
		if !errors.Is(err, ErrCredencialesInvalidas) {
			t.Errorf("%s: error = %v, esperaba ErrCredencialesInvalidas", nombre, err)
		}
	}
}

func TestSembrarEsIdempotenteYGuardaSoloHashes(t *testing.T) {
	servicio, usuarios, organizaciones := nuevoServicioPrueba(t)

	if err := servicio.Sembrar(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(usuarios.PorEmail) != 1 || len(organizaciones.PorID) != 1 {
		t.Fatalf("sembrar dos veces duplicó datos: %d usuarios, %d organizaciones",
			len(usuarios.PorEmail), len(organizaciones.PorID))
	}

	admin := usuarios.PorEmail[siembraPrueba.AdminEmail]
	if admin.HashPassword == siembraPrueba.AdminPassword ||
		bcrypt.CompareHashAndPassword([]byte(admin.HashPassword), []byte(siembraPrueba.AdminPassword)) != nil {
		t.Error("la contraseña del administrador tiene que guardarse como hash bcrypt")
	}
	cliente := organizaciones.PorID[OrganizacionExternaID].ClientesAPI[0]
	if cliente.HashSecreto == siembraPrueba.ExternoClientSecret ||
		bcrypt.CompareHashAndPassword([]byte(cliente.HashSecreto), []byte(siembraPrueba.ExternoClientSecret)) != nil {
		t.Error("el secreto del sistema externo tiene que guardarse como hash bcrypt")
	}
}
