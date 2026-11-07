-- Listado de la web por fecha (docs/interfaces-internas.md §12): filtra y ordena por el inicio
-- de la franja del traslado.
ALTER TABLE traslados ADD INDEX idx_traslado_franja (franja_inicio);
