-- Logs de todos los servicios (ADR-023). La imagen de MySQL ejecuta este archivo una sola vez, al
-- crear el volumen, dentro de la base MYSQL_DATABASE (observabilidad). Lo usa también el test de
-- integración de servicios/comun/observabilidad.
CREATE TABLE IF NOT EXISTS logs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  creado_en DATETIME(3) NOT NULL,
  nivel VARCHAR(5) NOT NULL,           -- INFO, WARN o ERROR
  servicio VARCHAR(32) NOT NULL,
  instancia VARCHAR(64) NOT NULL,
  mensaje VARCHAR(1000) NOT NULL,
  correlation_id VARCHAR(64) NULL,
  trace_id CHAR(32) NULL,
  atributos JSON NULL,                 -- el resto de los campos del log (solicitud_id, ruta, error…)
  -- Índices según cómo se busca en el tablero: por tiempo, por nivel o servicio en un período y
  -- todos los logs de una operación (correlation_id).
  INDEX idx_logs_creado (creado_en),
  INDEX idx_logs_nivel_creado (nivel, creado_en),
  INDEX idx_logs_servicio_creado (servicio, creado_en),
  INDEX idx_logs_correlacion (correlation_id)
);

-- Retención: se borran los logs de más de 7 días una vez por día. Alcanza para mirar "qué pasó la
-- semana pasada" y la tabla no crece sin límite (el event scheduler está activo por defecto).
CREATE EVENT IF NOT EXISTS borrar_logs_viejos
  ON SCHEDULE EVERY 1 DAY
  DO DELETE FROM logs WHERE creado_en < NOW(3) - INTERVAL 7 DAY;
