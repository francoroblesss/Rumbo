// Package solr es el adaptador de salida del índice de búsqueda de traslados (puerto
// BuscadorTraslados, ADR-011) sobre Apache Solr 9. Usa la API HTTP de Solr con net/http, como la
// clase práctica 5 (no hay cliente oficial para Go). La forma JSON de Solr (response.numFound,
// docs, _version_) no sale de este paquete.
package solr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

const (
	// qf: dónde se busca el texto y con qué peso. Lo que se traslada pesa más que el resto (boost).
	camposTexto = "descripcion_texto^3 texto"
	// fl: los campos que se devuelven (los de la respuesta de §16).
	camposDevueltos = "id,traslado_id,organizacion_id,referencia_externa,descripcion,origen_nombre,destino_nombre," +
		"estado_traslado,franja_inicio,franja_fin,alerta_climatica,cancelacion_tardia"
	formatoFecha = "2006-01-02T15:04:05Z"
	// maxConflictos: cuántas veces se reintenta una escritura que perdió contra otra del mismo
	// documento (409 de la concurrencia optimista).
	maxConflictos = 5
)

// Cliente habla con el core de traslados (por ejemplo, http://solr:8983/solr/traslados).
type Cliente struct {
	url  string
	http *http.Client

	mu    sync.Mutex
	listo bool // el esquema ya está aplicado
}

var _ puertos.BuscadorTraslados = (*Cliente)(nil)

// NuevoCliente arma el cliente. Solr no es de Rumbo: va con observabilidad.ClienteHTTP (sin el
// secreto interno) y con timeout, para que una búsqueda lenta no retenga la request.
func NuevoCliente(urlCore string, cliente *http.Client) *Cliente {
	return &Cliente{url: strings.TrimSuffix(urlCore, "/"), http: cliente}
}

// ---------------------------------------------------------------- Esquema

// tipoTexto es el análisis de texto en español (teórico 5): tokenizar, minúsculas, stopwords,
// sin tildes y plural a singular, igual al indexar y al consultar. El text_es de Solr usa un
// stemmer que deja "cajas" como "caj" y "caja" como "caja", y buscar "caja" no encontraba
// "Cajas": con SpanishMinimalStem, "Cajas", "caja" y "CAJA" quedan como "caja".
var tipoTexto = map[string]any{
	"name":                 "texto_es_rumbo",
	"class":                "solr.TextField",
	"positionIncrementGap": "100",
	"analyzer": map[string]any{
		"tokenizer": map[string]any{"class": "solr.StandardTokenizerFactory"},
		"filters": []map[string]any{
			{"class": "solr.LowerCaseFilterFactory"},
			{"class": "solr.StopFilterFactory", "ignoreCase": "true", "words": "lang/stopwords_es.txt", "format": "snowball"},
			{"class": "solr.ASCIIFoldingFilterFactory"},
			{"class": "solr.SpanishMinimalStemFilterFactory"},
		},
	},
}

// campos del documento (diseñado para la lectura): texto, filtros, orden y respuesta. "id" (el id
// de la solicitud) y "_version_" ya vienen en el esquema por defecto.
var campos = []map[string]any{
	campo("traslado_id", "string"),
	campo("organizacion_id", "string"),
	campo("referencia_externa", "string"),
	campo("origen_nombre", "string"),
	campo("destino_nombre", "string"),
	campo("estado_traslado", "string"),
	campo("alerta_climatica", "string"),
	{"name": "descripcion", "type": "string", "indexed": false, "stored": true}, // solo para mostrar
	campo("cancelacion_tardia", "boolean"),
	// sortMissingLast: un traslado sin franja queda último al ordenar por fecha (y no primero).
	{"name": "franja_inicio", "type": "pdate", "indexed": true, "stored": true, "sortMissingLast": true},
	campo("franja_fin", "pdate"),
	campo("version", "plong"),
	campo("actualizado_en", "pdate"),
	{"name": "descripcion_texto", "type": "texto_es_rumbo", "indexed": true, "stored": false},
	{"name": "texto", "type": "texto_es_rumbo", "indexed": true, "stored": false, "multiValued": true},
}

func campo(nombre, tipo string) map[string]any {
	return map[string]any{"name": nombre, "type": tipo, "indexed": true, "stored": true}
}

// AsegurarEsquema aplica el tipo de texto y los campos con la Schema API, agregando solo lo que
// falta (es idempotente: se puede llamar en cada arranque y desde varias réplicas). Solo agrega:
// cambiar un campo que ya existe requiere borrar el índice y reindexar (es derivado).
func (c *Cliente) AsegurarEsquema(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.listo {
		return nil
	}
	var actual struct {
		Schema struct {
			FieldTypes []struct {
				Name string `json:"name"`
			} `json:"fieldTypes"`
			Fields []struct {
				Name string `json:"name"`
			} `json:"fields"`
		} `json:"schema"`
	}
	if err := c.pedir(ctx, http.MethodGet, "/schema", nil, nil, &actual); err != nil {
		return err
	}
	existe := map[string]bool{}
	for _, t := range actual.Schema.FieldTypes {
		existe["tipo:"+t.Name] = true
	}
	for _, f := range actual.Schema.Fields {
		existe[f.Name] = true
	}
	// Dos pedidos: si el tipo y los campos van en el mismo JSON armado con un map, las claves
	// salen en orden alfabético (add-field antes que add-field-type) y Solr rechaza los campos
	// porque el tipo todavía no existe.
	if !existe["tipo:texto_es_rumbo"] {
		if err := c.pedir(ctx, http.MethodPost, "/schema", nil, map[string]any{"add-field-type": tipoTexto}, nil); err != nil {
			return err
		}
	}
	var faltan []map[string]any
	for _, f := range campos {
		if !existe[f["name"].(string)] {
			faltan = append(faltan, f)
		}
	}
	if len(faltan) > 0 {
		if err := c.pedir(ctx, http.MethodPost, "/schema", nil, map[string]any{"add-field": faltan}, nil); err != nil {
			return err
		}
	}
	c.listo = true
	return nil
}

// ---------------------------------------------------------------- Búsqueda

// documento es la forma en que Solr devuelve un documento (no sale del paquete).
type documento struct {
	ID                string  `json:"id"`
	TrasladoID        string  `json:"traslado_id"`
	OrganizacionID    string  `json:"organizacion_id"`
	ReferenciaExterna *string `json:"referencia_externa"`
	Descripcion       string  `json:"descripcion"`
	OrigenNombre      string  `json:"origen_nombre"`
	DestinoNombre     string  `json:"destino_nombre"`
	EstadoTraslado    string  `json:"estado_traslado"`
	FranjaInicio      *string `json:"franja_inicio"`
	FranjaFin         *string `json:"franja_fin"`
	AlertaClimatica   *string `json:"alerta_climatica"`
	CancelacionTardia bool    `json:"cancelacion_tardia"`
}

// Buscar traduce el filtro a una consulta eDisMax: el texto va en q (participa del puntaje) y
// cada filtro en un fq (restringe sin tocar el puntaje). Los literales van con {!term}, así un
// valor con sintaxis de Solr no amplía el filtro.
func (c *Cliente) Buscar(ctx context.Context, f puertos.FiltroBusqueda) ([]puertos.DocumentoTraslado, int, error) {
	if err := c.AsegurarEsquema(ctx); err != nil {
		return nil, 0, err
	}
	p := url.Values{}
	texto := strings.TrimSpace(f.Texto)
	if texto == "" {
		p.Set("q", "*:*")
	} else {
		p.Set("q", texto)
	}
	p.Set("defType", "edismax")
	p.Set("qf", camposTexto)
	if f.Organizacion != "" {
		p.Add("fq", "{!term f=organizacion_id}"+f.Organizacion)
	}
	if f.Estado != "" {
		p.Add("fq", "{!term f=estado_traslado}"+f.Estado)
	}
	if f.SoloAlerta {
		p.Add("fq", "alerta_climatica:[* TO *]")
	}
	if f.FranjaDesde != nil || f.FranjaHasta != nil {
		desde, hasta := "*", "*"
		if f.FranjaDesde != nil {
			desde = f.FranjaDesde.UTC().Format(formatoFecha)
		}
		if f.FranjaHasta != nil {
			hasta = f.FranjaHasta.UTC().Format(formatoFecha)
		}
		p.Add("fq", "franja_inicio:["+desde+" TO "+hasta+"}") // hasta exclusive
	}
	if f.Orden == puertos.OrdenRelevancia && texto != "" {
		p.Set("sort", "score desc,franja_inicio asc,id asc")
	} else {
		p.Set("sort", "franja_inicio asc,id asc") // relevancia sin texto: por fecha
	}
	p.Set("start", strconv.Itoa(f.Desde))
	p.Set("rows", strconv.Itoa(f.Limite))
	p.Set("fl", camposDevueltos)
	p.Set("wt", "json")

	var respuesta struct {
		Response struct {
			NumFound int         `json:"numFound"`
			Docs     []documento `json:"docs"`
		} `json:"response"`
	}
	if err := c.pedir(ctx, http.MethodGet, "/select", p, nil, &respuesta); err != nil {
		return nil, 0, err
	}
	resultado := make([]puertos.DocumentoTraslado, 0, len(respuesta.Response.Docs))
	for _, d := range respuesta.Response.Docs {
		inicio, err1 := fecha(d.FranjaInicio)
		fin, err2 := fecha(d.FranjaFin)
		if err := errors.Join(err1, err2); err != nil {
			return nil, 0, fmt.Errorf("%w: fecha ilegible en %s: %v", puertos.ErrBusquedaNoDisponible, d.ID, err)
		}
		resultado = append(resultado, puertos.DocumentoTraslado{
			SolicitudID: d.ID, TrasladoID: d.TrasladoID, OrganizacionID: d.OrganizacionID,
			ReferenciaExterna: d.ReferenciaExterna, Descripcion: d.Descripcion,
			OrigenNombre: d.OrigenNombre, DestinoNombre: d.DestinoNombre, EstadoTraslado: d.EstadoTraslado,
			FranjaInicio: inicio, FranjaFin: fin, AlertaClimatica: d.AlertaClimatica, CancelacionTardia: d.CancelacionTardia,
		})
	}
	return resultado, respuesta.Response.NumFound, nil
}

func fecha(texto *string) (*time.Time, error) {
	if texto == nil {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *texto)
	if err != nil {
		return nil, err
	}
	t = t.UTC()
	return &t, nil
}

// ---------------------------------------------------------------- Indexación

// Indexar es el upsert idempotente con versión monotónica (ADR-011, teórico 5). La comparación y
// la escritura son atómicas con la concurrencia optimista de Solr, sin bloqueos:
//  1. real-time get de la versión indexada (ve también lo que todavía no se confirmó);
//  2. si la indexada es igual o mayor, no hay nada que hacer (evento repetido o viejo);
//  3. si no, se escribe con _version_ = la leída (o -1, "no tiene que existir");
//  4. si otra escritura del mismo documento ganó en el medio, Solr responde 409 y se vuelve a 1.
func (c *Cliente) Indexar(ctx context.Context, d puertos.DocumentoTraslado) (bool, error) {
	if err := c.AsegurarEsquema(ctx); err != nil {
		return false, err
	}
	for intento := 0; intento < maxConflictos; intento++ {
		var actual struct {
			Doc *struct {
				Version     int64 `json:"version"`
				VersionSolr int64 `json:"_version_"`
			} `json:"doc"`
		}
		consulta := url.Values{"id": {d.SolicitudID}, "fl": {"version,_version_"}, "wt": {"json"}}
		if err := c.pedir(ctx, http.MethodGet, "/get", consulta, nil, &actual); err != nil {
			return false, err
		}
		esperada := int64(-1)
		if actual.Doc != nil {
			if actual.Doc.Version >= d.Version {
				return false, nil
			}
			esperada = actual.Doc.VersionSolr
		}
		// El soft commit en cada escritura hace visible el cambio enseguida: así la demora medida
		// es la real. Con el volumen de Rumbo no pesa.
		escritura := url.Values{"commit": {"true"}, "softCommit": {"true"}, "wt": {"json"}}
		err := c.pedir(ctx, http.MethodPost, "/update", escritura, []map[string]any{aSolr(d, esperada)}, nil)
		var conflicto errorConflicto
		if errors.As(err, &conflicto) {
			continue
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("%w: %s: %d conflictos de versión seguidos", puertos.ErrBusquedaNoDisponible, d.SolicitudID, maxConflictos)
}

// aSolr arma el documento a escribir. Los campos sin valor se omiten.
func aSolr(d puertos.DocumentoTraslado, versionEsperada int64) map[string]any {
	doc := map[string]any{
		"id":                 d.SolicitudID,
		"_version_":          versionEsperada,
		"traslado_id":        d.TrasladoID,
		"organizacion_id":    d.OrganizacionID,
		"descripcion":        d.Descripcion,
		"origen_nombre":      d.OrigenNombre,
		"destino_nombre":     d.DestinoNombre,
		"estado_traslado":    d.EstadoTraslado,
		"cancelacion_tardia": d.CancelacionTardia,
		"version":            d.Version,
		"actualizado_en":     d.ActualizadoEn.UTC().Format(formatoFecha),
		"descripcion_texto":  d.Descripcion,
	}
	texto := []string{d.Descripcion}
	if d.ReferenciaExterna != nil {
		doc["referencia_externa"] = *d.ReferenciaExterna
		texto = append(texto, *d.ReferenciaExterna)
	}
	doc["texto"] = append(texto, d.OrigenNombre, d.OrigenDireccion, d.DestinoNombre, d.DestinoDireccion)
	if d.FranjaInicio != nil {
		doc["franja_inicio"] = d.FranjaInicio.UTC().Format(formatoFecha)
	}
	if d.FranjaFin != nil {
		doc["franja_fin"] = d.FranjaFin.UTC().Format(formatoFecha)
	}
	if d.AlertaClimatica != nil {
		doc["alerta_climatica"] = *d.AlertaClimatica
	}
	return doc
}

// ---------------------------------------------------------------- HTTP

// errorConflicto es el 409 de Solr: la versión esperada ya no es la del documento.
type errorConflicto struct{ detalle string }

func (e errorConflicto) Error() string { return "conflicto de versión: " + e.detalle }

// pedir hace una llamada a Solr. Cualquier falla (red, timeout, estado fuera de 2xx, JSON
// ilegible) vuelve envuelta en ErrBusquedaNoDisponible; un 409, como errorConflicto.
func (c *Cliente) pedir(ctx context.Context, metodo, ruta string, consulta url.Values, cuerpo, destino any) error {
	direccion := c.url + ruta
	if len(consulta) > 0 {
		direccion += "?" + consulta.Encode()
	}
	var lector io.Reader
	if cuerpo != nil {
		crudo, err := json.Marshal(cuerpo)
		if err != nil {
			return fmt.Errorf("%w: serializar el pedido a %s: %v", puertos.ErrBusquedaNoDisponible, ruta, err)
		}
		lector = bytes.NewReader(crudo)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, direccion, lector)
	if err != nil {
		return fmt.Errorf("%w: armar el pedido a %s: %v", puertos.ErrBusquedaNoDisponible, ruta, err)
	}
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s %s: %v", puertos.ErrBusquedaNoDisponible, metodo, ruta, err)
	}
	defer resp.Body.Close()
	crudo, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: leer la respuesta de %s: %v", puertos.ErrBusquedaNoDisponible, ruta, err)
	}
	if resp.StatusCode == http.StatusConflict {
		return errorConflicto{detalle: recortar(crudo)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w: %s %s respondió %d: %s", puertos.ErrBusquedaNoDisponible, metodo, ruta, resp.StatusCode, recortar(crudo))
	}
	if destino == nil {
		return nil
	}
	if err := json.Unmarshal(crudo, destino); err != nil {
		return fmt.Errorf("%w: respuesta ilegible de %s: %v", puertos.ErrBusquedaNoDisponible, ruta, err)
	}
	return nil
}

// recortar deja el cuerpo de un error en un largo razonable para el mensaje.
func recortar(crudo []byte) string {
	texto := strings.TrimSpace(string(crudo))
	if len(texto) > 300 {
		return texto[:300] + "…"
	}
	return texto
}
