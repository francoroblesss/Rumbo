// Package solr es el adaptador de salida del motor de búsqueda (ADR-011): implementa
// puertos.BuscadorTraslados sobre el core "traslados" de Apache Solr 9 con su API HTTP, como en la
// clase práctica 5 (eDisMax, fq con {!term}). Es la única parte de Operaciones que conoce las URL,
// los parámetros y el JSON de Solr; la forma de sus respuestas no sale de este paquete.
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

// Tiempo máximo de cada llamada a Solr (lo usa main al armar el cliente HTTP, como en la clase 5).
const Timeout = 2 * time.Second

// intentosVersion es cuántas veces se reintenta una escritura que perdió la carrera de versiones
// contra otra escritura del mismo documento (409 de Solr).
const intentosVersion = 5

// formatoFecha es el formato de fecha de Solr (siempre en UTC).
const formatoFecha = "2006-01-02T15:04:05Z"

// tipoTexto es el análisis del texto en español (teórico 5: el mismo en el índice y en la
// consulta): tokenización, minúsculas, stopwords, sin tildes y plural a singular. Se define acá
// porque el text_es que trae Solr usa el stemmer liviano, que deja "cajas" como "caj" y "caja"
// como "caja", y así una búsqueda por "caja" no encuentra "Cajas". Son todos filtros de Lucene:
// solo se elige cuáles.
const tipoTexto = "texto_es_rumbo"

var definicionTipoTexto = map[string]any{
	"name": tipoTexto, "class": "solr.TextField", "positionIncrementGap": "100",
	"analyzer": map[string]any{
		"tokenizer": map[string]any{"class": "solr.StandardTokenizerFactory"},
		"filters": []map[string]any{
			{"class": "solr.LowerCaseFilterFactory"},
			{"class": "solr.StopFilterFactory", "words": "lang/stopwords_es.txt", "format": "snowball", "ignoreCase": "true"},
			{"class": "solr.ASCIIFoldingFilterFactory"},       // "camión" = "camion"
			{"class": "solr.SpanishMinimalStemFilterFactory"}, // "cajas" = "caja"
		},
	},
}

// campos es el esquema del core (Schema API). texto y descripcion_texto se analizan en español
// (tipoTexto); el resto son valores exactos para filtrar y ordenar. Los de texto no se guardan:
// para mostrar se usan los campos exactos.
var campos = []map[string]any{
	{"name": "traslado_id", "type": "string"},
	{"name": "organizacion_id", "type": "string"},
	{"name": "referencia_externa", "type": "string"},
	{"name": "descripcion", "type": "string", "indexed": false},
	{"name": "origen_nombre", "type": "string"},
	{"name": "destino_nombre", "type": "string"},
	{"name": "estado_traslado", "type": "string"},
	{"name": "alerta_climatica", "type": "string"},
	{"name": "cancelacion_tardia", "type": "boolean"},
	// sortMissingLast: una solicitud sin franja (de antes de ADR-024) queda al final del orden por fecha.
	{"name": "franja_inicio", "type": "pdate", "sortMissingLast": true},
	{"name": "franja_fin", "type": "pdate"},
	{"name": "version", "type": "plong"},
	{"name": "actualizado_en", "type": "pdate"},
	{"name": "descripcion_texto", "type": tipoTexto, "stored": false},
	{"name": "texto", "type": tipoTexto, "stored": false, "multiValued": true},
}

// qf pondera los campos de texto (boost, clase práctica 5): lo que se traslada pesa más que las
// direcciones y la referencia.
const qf = "descripcion_texto^3 texto"

// fl son los campos que se piden en cada respuesta.
const fl = "id,traslado_id,organizacion_id,referencia_externa,descripcion,origen_nombre,destino_nombre," +
	"estado_traslado,alerta_climatica,cancelacion_tardia,franja_inicio,franja_fin,version,actualizado_en"

// Cliente implementa puertos.BuscadorTraslados.
type Cliente struct {
	base string // http://solr:8983/solr/traslados
	http *http.Client

	mu    sync.Mutex
	listo bool // el esquema ya está aplicado
}

var _ puertos.BuscadorTraslados = (*Cliente)(nil)

// NuevoCliente crea el adaptador. base es la URL del core.
func NuevoCliente(base string, cliente *http.Client) *Cliente {
	return &Cliente{base: strings.TrimRight(base, "/"), http: cliente}
}

// AsegurarEsquema agrega al core el tipo de texto y los campos que le falten (idempotente: se puede llamar en cada
// arranque y desde varias réplicas). Antes de aplicarlo no se busca ni se indexa: Solr crearía
// solo los campos con tipos adivinados (modo sin esquema del configset por defecto).
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
	tipoExiste := false
	for _, t := range actual.Schema.FieldTypes {
		tipoExiste = tipoExiste || t.Name == tipoTexto
	}
	// Primero el tipo y después los campos que lo usan, en pedidos separados (Solr aplica los
	// comandos en orden, y un mapa de Go no lo conserva). Si otra réplica los agregó al mismo
	// tiempo, Solr responde error y se vuelve a mirar en el próximo intento.
	if !tipoExiste {
		if err := c.pedir(ctx, http.MethodPost, "/schema", nil, map[string]any{"add-field-type": definicionTipoTexto}, nil); err != nil {
			return err
		}
	}
	existe := map[string]bool{}
	for _, f := range actual.Schema.Fields {
		existe[f.Name] = true
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

// documento es la forma del documento en Solr.
type documento struct {
	ID                string   `json:"id"`
	TrasladoID        string   `json:"traslado_id,omitempty"`
	OrganizacionID    string   `json:"organizacion_id"`
	ReferenciaExterna string   `json:"referencia_externa,omitempty"`
	Descripcion       string   `json:"descripcion"`
	OrigenNombre      string   `json:"origen_nombre"`
	DestinoNombre     string   `json:"destino_nombre"`
	EstadoTraslado    string   `json:"estado_traslado,omitempty"`
	AlertaClimatica   string   `json:"alerta_climatica,omitempty"`
	CancelacionTardia bool     `json:"cancelacion_tardia"`
	FranjaInicio      string   `json:"franja_inicio,omitempty"`
	FranjaFin         string   `json:"franja_fin,omitempty"`
	Version           int64    `json:"version"`
	ActualizadoEn     string   `json:"actualizado_en"`
	DescripcionTexto  string   `json:"descripcion_texto,omitempty"`
	Texto             []string `json:"texto,omitempty"`
	VersionSolr       int64    `json:"_version_,omitempty"`
}

// Indexar aplica el documento solo si su versión es mayor que la indexada, con la concurrencia
// optimista de Solr (ADR-011): se lee la versión actual con el real-time get (ve también lo que
// todavía no se confirmó) y se escribe con su _version_. Si otra escritura del mismo documento
// ganó en el medio, Solr responde 409 y se vuelve a leer: comparación y escritura quedan atómicas
// sin bloqueos.
func (c *Cliente) Indexar(ctx context.Context, d puertos.DocumentoTraslado) (bool, error) {
	if err := c.AsegurarEsquema(ctx); err != nil {
		return false, err
	}
	nuevo := aSolr(d)
	for range intentosVersion {
		var actual struct {
			Doc *struct {
				Version     int64 `json:"version"`
				VersionSolr int64 `json:"_version_"`
			} `json:"doc"`
		}
		consulta := url.Values{"id": {d.SolicitudID}, "fl": {"version,_version_"}}
		if err := c.pedir(ctx, http.MethodGet, "/get", consulta, nil, &actual); err != nil {
			return false, err
		}
		if actual.Doc != nil && actual.Doc.Version >= d.Version {
			return false, nil
		}
		// _version_ < 0: el documento no tiene que existir; > 0: tiene que tener esa versión.
		nuevo.VersionSolr = -1
		if actual.Doc != nil {
			nuevo.VersionSolr = actual.Doc.VersionSolr
		}
		// Soft commit en cada escritura: el cambio queda visible en la búsqueda al responder, y la
		// demora que se mide es la real. Con el volumen de esta etapa no pesa.
		escritura := url.Values{"commit": {"true"}, "softCommit": {"true"}}
		err := c.pedir(ctx, http.MethodPost, "/update", escritura, []documento{nuevo}, nil)
		var e errorSolr
		if errors.As(err, &e) && e.estado == http.StatusConflict {
			continue
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("%w: el documento %s cambió %d veces mientras se indexaba", puertos.ErrBusquedaNoDisponible, d.SolicitudID, intentosVersion)
}

func aSolr(d puertos.DocumentoTraslado) documento {
	s := documento{
		ID: d.SolicitudID, TrasladoID: d.TrasladoID, OrganizacionID: d.OrganizacionID, ReferenciaExterna: d.ReferenciaExterna,
		Descripcion: d.Descripcion, OrigenNombre: d.OrigenNombre, DestinoNombre: d.DestinoNombre,
		EstadoTraslado: d.EstadoTraslado, AlertaClimatica: d.AlertaClimatica, CancelacionTardia: d.CancelacionTardia,
		Version: d.Version, ActualizadoEn: d.ActualizadoEn.UTC().Format(formatoFecha), DescripcionTexto: d.Descripcion,
	}
	if d.FranjaInicio != nil {
		s.FranjaInicio = d.FranjaInicio.UTC().Format(formatoFecha)
	}
	if d.FranjaFin != nil {
		s.FranjaFin = d.FranjaFin.UTC().Format(formatoFecha)
	}
	for _, t := range []string{d.Descripcion, d.ReferenciaExterna, d.OrigenNombre, d.OrigenDireccion, d.DestinoNombre, d.DestinoDireccion} {
		if t != "" {
			s.Texto = append(s.Texto, t)
		}
	}
	return s
}

// Buscar traduce el filtro a parámetros de Solr: q con eDisMax sobre los campos de texto, y cada
// filtro como fq (restringe sin cambiar el puntaje). Los valores exactos van con {!term}, así son
// literales y nunca sintaxis de Solr escrita por el usuario (clase práctica 5).
func (c *Cliente) Buscar(ctx context.Context, f puertos.FiltroBusqueda) ([]puertos.DocumentoTraslado, int, error) {
	if err := c.AsegurarEsquema(ctx); err != nil {
		return nil, 0, err
	}
	p := url.Values{
		"fl":    {fl},
		"start": {strconv.Itoa(f.Desde)},
		"rows":  {strconv.Itoa(f.Limite)},
		"wt":    {"json"},
	}
	if f.Texto != "" {
		p.Set("q", f.Texto)
		p.Set("defType", "edismax")
		p.Set("qf", qf)
		// Con eDisMax el operador por defecto es OR (clase 5): "cajas aeropuerto" trae las que
		// tienen cualquiera de las dos, y las que tienen las dos quedan arriba por puntaje.
	} else {
		p.Set("q", "*:*")
	}
	if f.PorRelevancia && f.Texto != "" {
		p.Set("sort", "score desc,franja_inicio asc,id asc")
	} else {
		p.Set("sort", "franja_inicio asc,id asc")
	}
	if f.Organizacion != "" {
		p.Add("fq", "{!term f=organizacion_id}"+f.Organizacion)
	}
	if f.EstadoTraslado != "" {
		p.Add("fq", "{!term f=estado_traslado}"+f.EstadoTraslado)
	}
	if f.SoloConAlerta {
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
		p.Add("fq", "franja_inicio:["+desde+" TO "+hasta+"}") // desde inclusive, hasta exclusive
	}

	var r struct {
		Response struct {
			NumFound int         `json:"numFound"`
			Docs     []documento `json:"docs"`
		} `json:"response"`
	}
	if err := c.pedir(ctx, http.MethodGet, "/select", p, nil, &r); err != nil {
		return nil, 0, err
	}
	docs := make([]puertos.DocumentoTraslado, 0, len(r.Response.Docs))
	for _, d := range r.Response.Docs {
		docs = append(docs, deSolr(d))
	}
	return docs, r.Response.NumFound, nil
}

func deSolr(s documento) puertos.DocumentoTraslado {
	d := puertos.DocumentoTraslado{
		SolicitudID: s.ID, TrasladoID: s.TrasladoID, OrganizacionID: s.OrganizacionID, ReferenciaExterna: s.ReferenciaExterna,
		Descripcion: s.Descripcion, OrigenNombre: s.OrigenNombre, DestinoNombre: s.DestinoNombre,
		EstadoTraslado: s.EstadoTraslado, AlertaClimatica: s.AlertaClimatica, CancelacionTardia: s.CancelacionTardia,
		Version: s.Version, FranjaInicio: fecha(s.FranjaInicio), FranjaFin: fecha(s.FranjaFin),
	}
	if t := fecha(s.ActualizadoEn); t != nil {
		d.ActualizadoEn = *t
	}
	return d
}

func fecha(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

// errorSolr es una respuesta de Solr fuera de 2xx.
type errorSolr struct {
	estado int
	cuerpo string
}

func (e errorSolr) Error() string { return fmt.Sprintf("Solr respondió %d: %s", e.estado, e.cuerpo) }

// pedir hace una llamada a Solr. Cualquier falla (red, timeout o un estado fuera de 2xx) sale
// envuelta en puertos.ErrBusquedaNoDisponible, así el núcleo responde 503 sin conocer Solr.
func (c *Cliente) pedir(ctx context.Context, metodo, ruta string, consulta url.Values, cuerpo, destino any) error {
	u := c.base + ruta
	if len(consulta) > 0 {
		u += "?" + consulta.Encode()
	}
	var lector io.Reader
	if cuerpo != nil {
		crudo, err := json.Marshal(cuerpo)
		if err != nil {
			return fmt.Errorf("serializar el pedido a Solr: %w", err)
		}
		lector = bytes.NewReader(crudo)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, u, lector)
	if err != nil {
		return fmt.Errorf("%w: %v", puertos.ErrBusquedaNoDisponible, err)
	}
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", puertos.ErrBusquedaNoDisponible, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		texto, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: %w", puertos.ErrBusquedaNoDisponible, errorSolr{estado: resp.StatusCode, cuerpo: string(texto)})
	}
	if destino == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(destino); err != nil {
		return fmt.Errorf("%w: respuesta ilegible de Solr: %v", puertos.ErrBusquedaNoDisponible, err)
	}
	return nil
}
