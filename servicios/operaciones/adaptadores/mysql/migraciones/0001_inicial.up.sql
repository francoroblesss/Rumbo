CREATE TABLE solicitudes (
  id VARCHAR(64) PRIMARY KEY,
  organizacion_id VARCHAR(64) NOT NULL,
  origen_solicitud VARCHAR(16) NOT NULL,
  referencia_externa VARCHAR(64) NULL,
  cosa JSON NOT NULL,
  origen JSON NOT NULL,
  destino JSON NOT NULL,
  hora_llegada_solicitada DATETIME(6) NULL,
  tolerancia_minutos INT NOT NULL DEFAULT 0,
  observaciones VARCHAR(500) NULL,
  estado VARCHAR(32) NOT NULL,
  contrapropuesta_hora DATETIME(6) NULL,
  contrapropuesta_vence DATETIME(6) NULL,
  motivo_rechazo VARCHAR(500) NULL,
  cancelacion_estado VARCHAR(32) NULL,
  cancelacion_motivo VARCHAR(500) NULL,
  cancelacion_solicitada_en DATETIME(6) NULL,
  version BIGINT NOT NULL,
  creado_en DATETIME(6) NOT NULL,
  actualizado_en DATETIME(6) NOT NULL,
  INDEX idx_estado_vence (estado, contrapropuesta_vence),
  INDEX idx_org_actualizado (organizacion_id, actualizado_en),
  INDEX idx_estado_actualizado (estado, actualizado_en)
);
CREATE TABLE traslados (
  id VARCHAR(64) PRIMARY KEY,
  solicitud_id VARCHAR(64) NOT NULL UNIQUE,
  estado VARCHAR(32) NOT NULL,
  hora_llegada_comprometida DATETIME(6) NOT NULL,
  demora_minutos INT NULL,
  finalizado_en DATETIME(6) NULL,
  detalle_resultado VARCHAR(500) NULL,
  aeropuerto_extremo VARCHAR(16) NULL,
  alerta_estado VARCHAR(32) NULL,
  alerta_motivos JSON NULL,
  alerta_observado_en DATETIME(6) NULL,
  CONSTRAINT fk_traslado_solicitud FOREIGN KEY (solicitud_id) REFERENCES solicitudes(id)
);
CREATE TABLE outbox (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  mensaje_id VARCHAR(64) NOT NULL UNIQUE,
  exchange_nombre VARCHAR(128) NOT NULL,
  routing_key VARCHAR(128) NOT NULL,
  sobre JSON NOT NULL,
  contexto_traza JSON NULL,
  creado_en DATETIME(6) NOT NULL,
  publicado_en DATETIME(6) NULL,
  INDEX idx_pendientes (publicado_en, id)
);
CREATE TABLE mensajes_procesados (
  mensaje_id VARCHAR(64) PRIMARY KEY,
  procesado_en DATETIME(6) NOT NULL
);
CREATE TABLE claves_idempotencia (
  organizacion_id VARCHAR(64) NOT NULL,
  endpoint VARCHAR(64) NOT NULL,
  clave VARCHAR(128) NOT NULL,
  hash_cuerpo CHAR(64) NOT NULL,
  estado_http INT NOT NULL,
  respuesta JSON NOT NULL,
  creada_en DATETIME(6) NOT NULL,
  PRIMARY KEY (organizacion_id, endpoint, clave)
);
