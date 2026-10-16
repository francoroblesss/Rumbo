package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/identidad/models"
	"github.com/francoroblesss/Rumbo/servicios/identidad/repositories"
)

// Datos del token (docs/interfaces-internas.md §2). El gateway valida exactamente estos valores.
const (
	EmisorToken    = "rumbo-identidad"
	AudienciaToken = "rumbo"
	DuracionToken  = time.Hour

	// OrganizacionExternaID es la organización de tipo stock del sistema del grupo consumidor.
	OrganizacionExternaID = "org-almacenamiento"
)

// ErrCredencialesInvalidas es el único error de autenticación que ve quien llama: no revela si
// el usuario o el client_id existen.
var ErrCredencialesInvalidas = errors.New("credenciales inválidas")

// Token es la respuesta de login y de client credentials.
type Token struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiraEnSegundos int    `json:"expira_en_segundos"`
}

// Siembra son los datos con los que Identidad se inicializa (variables de entorno).
type Siembra struct {
	AdminEmail          string
	AdminPassword       string
	ExternoClientID     string
	ExternoClientSecret string
}

// reclamos son los claims del token: los estándar más la organización y los roles.
type reclamos struct {
	Org   string   `json:"org,omitempty"`
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

// AuthService autentica usuarios y sistemas externos y emite los tokens firmados.
type AuthService struct {
	usuarios       repositories.UsuariosRepository
	organizaciones repositories.OrganizacionesRepository
	secreto        []byte
	siembra        Siembra
	hashFicticio   []byte
}

func NuevoAuthService(usuarios repositories.UsuariosRepository, organizaciones repositories.OrganizacionesRepository,
	secretoToken string, siembra Siembra) *AuthService {
	// Hash de relleno: cuando el usuario no existe se compara igual contra un hash, así el
	// tiempo de respuesta no delata qué usuarios existen. El error solo ocurriría con una clave
	// de más de 72 bytes, que esta constante no es.
	hashFicticio, _ := bcrypt.GenerateFromPassword([]byte("rumbo-relleno"), bcrypt.DefaultCost)
	return &AuthService{
		usuarios:       usuarios,
		organizaciones: organizaciones,
		secreto:        []byte(secretoToken),
		siembra:        siembra,
		hashFicticio:   hashFicticio,
	}
}

// Sembrar crea, si no existen, el usuario administrador y la organización del sistema externo.
// Es idempotente: se puede ejecutar en cada arranque y desde varias réplicas.
func (s *AuthService) Sembrar(ctx context.Context) error {
	if err := s.sembrarAdministrador(ctx); err != nil {
		return err
	}
	return s.sembrarSistemaExterno(ctx)
}

func (s *AuthService) sembrarAdministrador(ctx context.Context) error {
	_, err := s.usuarios.BuscarPorEmail(ctx, s.siembra.AdminEmail)
	if err == nil {
		return nil
	}
	if !errors.Is(err, repositories.ErrNoEncontrado) {
		return fmt.Errorf("buscar administrador: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.siembra.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashear contraseña del administrador: %w", err)
	}
	usuario := &models.Usuario{
		ID:           "usr-" + uuid.NewString(),
		Email:        s.siembra.AdminEmail,
		HashPassword: string(hash),
		Roles:        []string{identidad.RolAdministrador, identidad.RolOperador},
		CreadoEn:     time.Now().UTC(),
	}
	if err := s.usuarios.Crear(ctx, usuario); err != nil && !errors.Is(err, repositories.ErrDuplicado) {
		return fmt.Errorf("crear administrador: %w", err)
	}
	slog.InfoContext(ctx, "usuario administrador sembrado", slog.String("usuario_id", usuario.ID))
	return nil
}

func (s *AuthService) sembrarSistemaExterno(ctx context.Context) error {
	_, err := s.organizaciones.BuscarPorID(ctx, OrganizacionExternaID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, repositories.ErrNoEncontrado) {
		return fmt.Errorf("buscar organización del sistema externo: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.siembra.ExternoClientSecret), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashear secreto del sistema externo: %w", err)
	}
	organizacion := &models.Organizacion{
		ID:           OrganizacionExternaID,
		Nombre:       "Sistema de gestión de almacenamiento (grupo consumidor)",
		TipoServicio: models.ServicioStock,
		ClientesAPI:  []models.ClienteAPI{{ClientID: s.siembra.ExternoClientID, HashSecreto: string(hash)}},
		CreadoEn:     time.Now().UTC(),
	}
	if err := s.organizaciones.Crear(ctx, organizacion); err != nil && !errors.Is(err, repositories.ErrDuplicado) {
		return fmt.Errorf("crear organización del sistema externo: %w", err)
	}
	slog.InfoContext(ctx, "organización del sistema externo sembrada", slog.String("organizacion_id", organizacion.ID))
	return nil
}

// Login valida las credenciales de un usuario de la web y emite su token.
func (s *AuthService) Login(ctx context.Context, email, password string) (*Token, error) {
	usuario, err := s.usuarios.BuscarPorEmail(ctx, email)
	if errors.Is(err, repositories.ErrNoEncontrado) {
		s.compararConRelleno(password)
		return nil, ErrCredencialesInvalidas
	}
	if err != nil {
		return nil, fmt.Errorf("buscar usuario: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(usuario.HashPassword), []byte(password)) != nil {
		return nil, ErrCredencialesInvalidas
	}
	organizacion := ""
	if usuario.OrganizacionID != nil {
		organizacion = *usuario.OrganizacionID
	}
	return s.emitir(usuario.ID, organizacion, usuario.Roles)
}

// TokenCliente valida las credenciales de un sistema externo (client credentials) y emite su token.
func (s *AuthService) TokenCliente(ctx context.Context, clientID, secreto string) (*Token, error) {
	organizacion, err := s.organizaciones.BuscarPorClientID(ctx, clientID)
	if errors.Is(err, repositories.ErrNoEncontrado) {
		s.compararConRelleno(secreto)
		return nil, ErrCredencialesInvalidas
	}
	if err != nil {
		return nil, fmt.Errorf("buscar organización por client_id: %w", err)
	}
	for _, cliente := range organizacion.ClientesAPI {
		if cliente.ClientID != clientID {
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(cliente.HashSecreto), []byte(secreto)) != nil {
			return nil, ErrCredencialesInvalidas
		}
		return s.emitir(clientID, organizacion.ID, []string{identidad.RolSistemaExterno})
	}
	return nil, ErrCredencialesInvalidas
}

func (s *AuthService) compararConRelleno(clave string) {
	_ = bcrypt.CompareHashAndPassword(s.hashFicticio, []byte(clave))
}

func (s *AuthService) emitir(sub, organizacion string, roles []string) (*Token, error) {
	ahora := time.Now()
	firmado, err := jwt.NewWithClaims(jwt.SigningMethodHS256, reclamos{
		Org:   organizacion,
		Roles: roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    EmisorToken,
			Audience:  jwt.ClaimStrings{AudienciaToken},
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(ahora),
			ExpiresAt: jwt.NewNumericDate(ahora.Add(DuracionToken)),
		},
	}).SignedString(s.secreto)
	if err != nil {
		return nil, fmt.Errorf("firmar token: %w", err)
	}
	return &Token{
		AccessToken:      firmado,
		TokenType:        "Bearer",
		ExpiraEnSegundos: int(DuracionToken.Seconds()),
	}, nil
}
