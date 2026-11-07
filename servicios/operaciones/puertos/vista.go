package puertos

// Vista es la solicitud tal como la ven los demás servicios: respuestas internas y datos de los
// eventos (docs/interfaces-internas.md §6). Es el dato que devuelve el puerto de entrada, por
// eso vive acá (aplicacion la arma con NuevaVista). Las horas van en RFC 3339 con -03:00.
type Vista struct {
	ID                    string         `json:"id"`
	ReferenciaExterna     *string        `json:"referencia_externa"`
	Estado                string         `json:"estado"`
	Cosa                  VistaCosa      `json:"cosa"`
	Origen                VistaUbicacion `json:"origen"`
	Destino               VistaUbicacion `json:"destino"`
	HoraLlegadaSolicitada *string        `json:"hora_llegada_solicitada"`
	ToleranciaMinutos     int            `json:"tolerancia_minutos"`
	Observaciones         *string        `json:"observaciones"`
	// PropuestaDeHorario es el campo "contrapropuesta" de la revisión de solicitudes: desde
	// ADR-024 queda siempre en null, pero se conserva para no cambiar la forma de la vista (§12).
	PropuestaDeHorario *struct{}         `json:"contrapropuesta"`
	MotivoRechazo      *string           `json:"motivo_rechazo"`
	Traslado           *VistaTraslado    `json:"traslado"`
	Cancelacion        *VistaCancelacion `json:"cancelacion"`
	Version            int64             `json:"version"`
	CreadoEn           string            `json:"creado_en"`
	ActualizadoEn      string            `json:"actualizado_en"`
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

type VistaTraslado struct {
	ID                      string       `json:"id"`
	Estado                  string       `json:"estado"`
	HoraLlegadaComprometida string       `json:"hora_llegada_comprometida"`
	DemoraMinutos           *int         `json:"demora_minutos"`
	FinalizadoEn            *string      `json:"finalizado_en"`
	DetalleResultado        *string      `json:"detalle_resultado"`
	Franja                  *VistaFranja `json:"franja"`
	AlertaClimatica         *VistaAlerta `json:"alerta_climatica"`
}

// VistaFranja es la ventana de llegada comprometida en una reserva externa.
type VistaFranja struct {
	Inicio string `json:"inicio"`
	Fin    string `json:"fin"`
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
	Tardia       bool   `json:"tardia"`
}

// Pagina es una página del listado de la web (§11.1).
type Pagina struct {
	Resultados []Vista `json:"resultados"`
	Pagina     int     `json:"pagina"`
	Tamanio    int     `json:"tamanio"`
	Total      int     `json:"total"`
}

// Disponibilidad son las franjas de una fecha que todavía se pueden reservar (ADR-021).
type Disponibilidad struct {
	Fecha   string             `json:"fecha"`
	Franjas []FranjaDisponible `json:"franjas"`
}

// FranjaDisponible dice si una carga del peso consultado entra en la franja y cuánto lugar queda.
type FranjaDisponible struct {
	Inicio               string  `json:"inicio"`
	Fin                  string  `json:"fin"`
	Disponible           bool    `json:"disponible"`
	TrasladosDisponibles int     `json:"traslados_disponibles"`
	KgDisponibles        float64 `json:"kg_disponibles"`
}

// FranjasDelDia son todas las franjas del calendario de una fecha con su ocupación: lo que ve el
// operador en la web (§12).
type FranjasDelDia struct {
	Fecha   string          `json:"fecha"`
	Franjas []FranjaOcupada `json:"franjas"`
}

// FranjaOcupada es una franja con su cupo y lo que ya se reservó.
type FranjaOcupada struct {
	Inicio              string  `json:"inicio"`
	Fin                 string  `json:"fin"`
	CupoTraslados       int     `json:"cupo_traslados"`
	TrasladosReservados int     `json:"traslados_reservados"`
	CupoKg              float64 `json:"cupo_kg"`
	KgReservados        float64 `json:"kg_reservados"`
}
