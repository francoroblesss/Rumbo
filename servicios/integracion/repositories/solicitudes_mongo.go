package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// NombreColeccion es la colección de solicitudes externas en la base integracion.
const NombreColeccion = "solicitudes_externas"

// SolicitudesMongo implementa SolicitudesRepository con el driver oficial de MongoDB.
type SolicitudesMongo struct {
	coleccion *mongo.Collection
}

// NewSolicitudesMongo crea el repositorio sobre la base indicada.
func NewSolicitudesMongo(db *mongo.Database) *SolicitudesMongo {
	return &SolicitudesMongo{coleccion: db.Collection(NombreColeccion)}
}

// AsegurarIndices crea (de forma idempotente) los índices que necesitan las consultas:
//   - único por (organización, clave de idempotencia): la base garantiza que un reintento no
//     duplique la solicitud, aunque lleguen dos a la vez a réplicas distintas;
//   - (organización, actualizado_en): el listado del polling.
func (r *SolicitudesMongo) AsegurarIndices(ctx context.Context) error {
	_, err := r.coleccion.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "organizacion_id", Value: 1}, {Key: "idempotencia.clave", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("organizacion_clave_idempotencia"),
		},
		{
			Keys:    bson.D{{Key: "organizacion_id", Value: 1}, {Key: "actualizado_en", Value: 1}},
			Options: options.Index().SetName("organizacion_actualizado_en"),
		},
	})
	if err != nil {
		return fmt.Errorf("crear índices de %s: %w", NombreColeccion, err)
	}
	return nil
}

func (r *SolicitudesMongo) Crear(ctx context.Context, s models.SolicitudExterna) (models.SolicitudExterna, error) {
	_, err := r.coleccion.InsertOne(ctx, s)
	if err == nil {
		return s, nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return models.SolicitudExterna{}, fmt.Errorf("insertar solicitud: %w", err)
	}
	var existente models.SolicitudExterna
	filtro := bson.D{{Key: "organizacion_id", Value: s.OrganizacionID}, {Key: "idempotencia.clave", Value: s.Idempotencia.Clave}}
	if err := r.coleccion.FindOne(ctx, filtro).Decode(&existente); err != nil {
		return models.SolicitudExterna{}, fmt.Errorf("leer la solicitud con la misma clave: %w", err)
	}
	return existente, ErrClaveExistente
}

func (r *SolicitudesMongo) BuscarPorID(ctx context.Context, organizacionID, id string) (models.SolicitudExterna, error) {
	var s models.SolicitudExterna
	err := r.coleccion.FindOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "organizacion_id", Value: organizacionID}}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.SolicitudExterna{}, ErrNoEncontrada
	}
	if err != nil {
		return models.SolicitudExterna{}, fmt.Errorf("buscar solicitud %s: %w", id, err)
	}
	return s, nil
}

func (r *SolicitudesMongo) BuscarPorClave(ctx context.Context, organizacionID, clave string) (models.SolicitudExterna, error) {
	var s models.SolicitudExterna
	filtro := bson.D{{Key: "organizacion_id", Value: organizacionID}, {Key: "idempotencia.clave", Value: clave}}
	err := r.coleccion.FindOne(ctx, filtro).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.SolicitudExterna{}, ErrNoEncontrada
	}
	if err != nil {
		return models.SolicitudExterna{}, fmt.Errorf("buscar por clave de idempotencia: %w", err)
	}
	return s, nil
}

func (r *SolicitudesMongo) Listar(ctx context.Context, f FiltroListado) ([]models.SolicitudExterna, int64, error) {
	filtro := bson.D{{Key: "organizacion_id", Value: f.OrganizacionID}}
	if f.ActualizadoDesde != nil {
		filtro = append(filtro, bson.E{Key: "actualizado_en", Value: bson.D{{Key: "$gt", Value: *f.ActualizadoDesde}}})
	}
	if f.Estado != "" {
		filtro = append(filtro, bson.E{Key: "vista.traslado.estado", Value: f.Estado})
	}
	total, err := r.coleccion.CountDocuments(ctx, filtro)
	if err != nil {
		return nil, 0, fmt.Errorf("contar solicitudes: %w", err)
	}
	opciones := options.Find().
		SetSort(bson.D{{Key: "actualizado_en", Value: 1}, {Key: "_id", Value: 1}}).
		SetSkip(int64((f.Pagina - 1) * f.Tamanio)).
		SetLimit(int64(f.Tamanio))
	cursor, err := r.coleccion.Find(ctx, filtro, opciones)
	if err != nil {
		return nil, 0, fmt.Errorf("listar solicitudes: %w", err)
	}
	solicitudes := []models.SolicitudExterna{}
	if err := cursor.All(ctx, &solicitudes); err != nil {
		return nil, 0, fmt.Errorf("leer solicitudes: %w", err)
	}
	return solicitudes, total, nil
}

// AplicarVista compara la versión y actualiza en la misma operación (filtro version < nueva):
// un evento viejo o repetido no modifica nada, como en un read model (teórico 5).
func (r *SolicitudesMongo) AplicarVista(ctx context.Context, id string, version int64, vista models.VistaSolicitud, ahora time.Time) (string, bool, error) {
	res, err := r.coleccion.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "version", Value: bson.D{{Key: "$lt", Value: version}}}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "vista", Value: vista},
			{Key: "version", Value: version},
			{Key: "actualizado_en", Value: ahora},
		}}})
	if err != nil {
		return "", false, fmt.Errorf("aplicar vista de %s: %w", id, err)
	}
	if res.ModifiedCount == 0 {
		return "", false, nil
	}
	var doc struct {
		OrganizacionID string `bson:"organizacion_id"`
	}
	err = r.coleccion.FindOne(ctx, bson.D{{Key: "_id", Value: id}},
		options.FindOne().SetProjection(bson.D{{Key: "organizacion_id", Value: 1}})).Decode(&doc)
	if err != nil {
		return "", true, fmt.Errorf("leer organización de %s: %w", id, err)
	}
	return doc.OrganizacionID, true, nil
}
