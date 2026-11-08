package repositories

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/francoroblesss/Rumbo/servicios/notificaciones/models"
)

// NombreColeccion es la colección de avisos en la base notificaciones.
const NombreColeccion = "avisos"

// AvisosMongo implementa AvisosRepository con el driver oficial de MongoDB.
type AvisosMongo struct {
	coleccion *mongo.Collection
}

var _ AvisosRepository = (*AvisosMongo)(nil)

// NewAvisosMongo crea el repositorio sobre la base indicada.
func NewAvisosMongo(db *mongo.Database) *AvisosMongo {
	return &AvisosMongo{coleccion: db.Collection(NombreColeccion)}
}

// AsegurarIndices crea (de forma idempotente) el índice por destinatario y fecha: lo usan la
// bandeja (más nuevos primero) y la consulta de cada segundo del stream (teórico 2: índices
// según los patrones de consulta). El _id único ya es la idempotencia del consumidor.
func (r *AvisosMongo) AsegurarIndices(ctx context.Context) error {
	_, err := r.coleccion.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "destinatario.tipo", Value: 1},
			{Key: "destinatario.valor", Value: 1},
			{Key: "creado_en", Value: -1},
		},
		Options: options.Index().SetName("destinatario_creado_en"),
	})
	if err != nil {
		return fmt.Errorf("crear índices de %s: %w", NombreColeccion, err)
	}
	return nil
}

func (r *AvisosMongo) Insertar(ctx context.Context, a models.Aviso) error {
	_, err := r.coleccion.InsertOne(ctx, a)
	if mongo.IsDuplicateKeyError(err) {
		return ErrAvisoExistente
	}
	if err != nil {
		return fmt.Errorf("insertar aviso %s: %w", a.ID, err)
	}
	return nil
}

func (r *AvisosMongo) Listar(ctx context.Context, visibles []models.Destinatario, soloNoLeidos bool, pagina, tamanio int) ([]models.Aviso, int64, error) {
	filtro := filtroVisibles(visibles)
	if soloNoLeidos {
		filtro = append(filtro, bson.E{Key: "leido", Value: false})
	}
	total, err := r.coleccion.CountDocuments(ctx, filtro)
	if err != nil {
		return nil, 0, fmt.Errorf("contar avisos: %w", err)
	}
	opciones := options.Find().
		SetSort(bson.D{{Key: "creado_en", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64((pagina - 1) * tamanio)).
		SetLimit(int64(tamanio))
	avisos, err := r.buscar(ctx, filtro, opciones)
	if err != nil {
		return nil, 0, err
	}
	return avisos, total, nil
}

func (r *AvisosMongo) Buscar(ctx context.Context, visibles []models.Destinatario, id string) (models.Aviso, error) {
	var a models.Aviso
	err := r.coleccion.FindOne(ctx, append(filtroVisibles(visibles), bson.E{Key: "_id", Value: id})).Decode(&a)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.Aviso{}, ErrNoEncontrado
	}
	if err != nil {
		return models.Aviso{}, fmt.Errorf("buscar aviso %s: %w", id, err)
	}
	return a, nil
}

func (r *AvisosMongo) MarcarLeido(ctx context.Context, visibles []models.Destinatario, id string) error {
	res, err := r.coleccion.UpdateOne(ctx,
		append(filtroVisibles(visibles), bson.E{Key: "_id", Value: id}),
		bson.D{{Key: "$set", Value: bson.D{{Key: "leido", Value: true}}}})
	if err != nil {
		return fmt.Errorf("marcar leído el aviso %s: %w", id, err)
	}
	// MatchedCount y no ModifiedCount: marcar uno que ya estaba leído también es un 204.
	if res.MatchedCount == 0 {
		return ErrNoEncontrado
	}
	return nil
}

func (r *AvisosMongo) Posteriores(ctx context.Context, visibles []models.Destinatario, desde Cursor, limite int) ([]models.Aviso, error) {
	filtro := bson.D{{Key: "$and", Value: bson.A{
		filtroVisibles(visibles),
		bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "creado_en", Value: bson.D{{Key: "$gt", Value: desde.CreadoEn}}}},
			bson.D{{Key: "creado_en", Value: desde.CreadoEn}, {Key: "_id", Value: bson.D{{Key: "$gt", Value: desde.ID}}}},
		}}},
	}}}
	opciones := options.Find().
		SetSort(bson.D{{Key: "creado_en", Value: 1}, {Key: "_id", Value: 1}}).
		SetLimit(int64(limite))
	return r.buscar(ctx, filtro, opciones)
}

func (r *AvisosMongo) buscar(ctx context.Context, filtro bson.D, opciones *options.FindOptions) ([]models.Aviso, error) {
	cursor, err := r.coleccion.Find(ctx, filtro, opciones)
	if err != nil {
		return nil, fmt.Errorf("buscar avisos: %w", err)
	}
	avisos := []models.Aviso{}
	if err := cursor.All(ctx, &avisos); err != nil {
		return nil, fmt.Errorf("leer avisos: %w", err)
	}
	return avisos, nil
}

// filtroVisibles arma el $or de los destinatarios visibles, con los campos separados para que
// MongoDB use el índice destinatario_creado_en. Sin destinatarios no se ve nada.
func filtroVisibles(visibles []models.Destinatario) bson.D {
	if len(visibles) == 0 {
		return bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{}}}}}
	}
	alternativas := bson.A{}
	for _, d := range visibles {
		alternativas = append(alternativas, bson.D{{Key: "destinatario.tipo", Value: d.Tipo}, {Key: "destinatario.valor", Value: d.Valor}})
	}
	return bson.D{{Key: "$or", Value: alternativas}}
}
