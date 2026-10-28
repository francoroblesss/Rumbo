-- Franjas de llegada de la capacidad publicada (ADR-021) y su cupo (D4, ADR-022). Una fila por
-- franja en la que alguien reservó: la crea la primera reserva con el cupo del calendario.
-- Los CHECK son la última defensa: aunque el código fallara, la base no deja reservar de más.
CREATE TABLE franjas (
  inicio DATETIME(6) PRIMARY KEY,
  fin DATETIME(6) NOT NULL,
  cupo_traslados INT NOT NULL,
  cupo_kg DOUBLE NOT NULL,
  traslados_reservados INT NOT NULL DEFAULT 0,
  kg_reservados DOUBLE NOT NULL DEFAULT 0,
  CONSTRAINT chk_franja_traslados CHECK (traslados_reservados BETWEEN 0 AND cupo_traslados),
  CONSTRAINT chk_franja_kg CHECK (kg_reservados >= 0 AND kg_reservados <= cupo_kg + 0.001)
);
ALTER TABLE traslados ADD COLUMN franja_inicio DATETIME(6) NULL, ADD INDEX idx_franja_inicio (franja_inicio);
ALTER TABLE solicitudes ADD COLUMN cancelacion_tardia BOOLEAN NOT NULL DEFAULT FALSE;
