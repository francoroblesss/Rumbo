ALTER TABLE solicitudes DROP COLUMN cancelacion_tardia;
ALTER TABLE traslados DROP INDEX idx_franja_inicio, DROP COLUMN franja_inicio;
DROP TABLE franjas;
