package puertos

// Vista es la solicitud tal como la ven los demás servicios: respuestas internas y datos de los
// eventos (docs/interfaces-internas.md §6). Es el dato que devuelve el puerto de entrada, por
// eso vive acá (aplicacion la arma con NuevaVista). Las horas van en RFC 3339 con -03:00.
type Vista struct {
	ID                    string                `json:"id"`
	ReferenciaExterna     *string               `json:"referencia_externa"`
	Estado                string                `json:"estado"`
	Cosa                  VistaCosa             `json:"cosa"`
	Origen                VistaUbicacion        `json:"origen"`
	Destino               VistaUbicacion        `json:"destino"`
	HoraLlegadaSolicitada *string               `json:"hora_llegada_solicitada"`
	ToleranciaMinutos     int                   `json:"tolerancia_minutos"`
	Observaciones         *string               `json:"observaciones"`
	Contrapropuesta       *VistaContrapropuesta `json:"contrapropuesta"`
	MotivoRechazo         *string               `json:"motivo_rechazo"`
	Traslado              *VistaTraslado        `json:"traslado"`
	Cancelacion           *VistaCancelacion     `json:"cancelacion"`
	Version               int64                 `json:"version"`
	CreadoEn              string                `json:"creado_en"`
	ActualizadoEn         string                `json:"actualizado_en"`
}

type VistaCosa struct {
	Descripcion string   `json:"descripcion"`
	Cantidad    int      `json:"cantidad"`
	PesoKg      *float64 `json:"peso_kg"`
	VolumenM3   *float64 `json:"volumen_m3"`
}

type VistaUbicacion struct {
	Nombre    string  `json:"nombre"`
	Direccion string  `json:"direccion"`
	Contacto  *string `json:"contacto"`
}

type VistaContrapropuesta struct {
	HoraLlegadaPropuesta string `json:"hora_llegada_propuesta"`
	VenceEn              string `json:"vence_en"`
}

type VistaTraslado struct {
	ID                      string       `json:"id"`
	Estado                  string       `json:"estado"`
	HoraLlegadaComprometida string       `json:"hora_llegada_comprometida"`
	DemoraMinutos           *int         `json:"demora_minutos"`
	FinalizadoEn            *string      `json:"finalizado_en"`
	DetalleResultado        *string      `json:"detalle_resultado"`
	AlertaClimatica         *VistaAlerta `json:"alerta_climatica"`
}

type VistaAlerta struct {
	Estado      string   `json:"estado"`
	Motivos     []string `json:"motivos"`
	ObservadoEn *string  `json:"observado_en"`
}

type VistaCancelacion struct {
	Estado       string `json:"estado"`
	Motivo       string `json:"motivo"`
	SolicitadaEn string `json:"solicitada_en"`
}

// Pagina es una página del listado del operador.
type Pagina struct {
	Resultados []Vista `json:"resultados"`
	Pagina     int     `json:"pagina"`
	Tamanio    int     `json:"tamanio"`
	Total      int     `json:"total"`
}
