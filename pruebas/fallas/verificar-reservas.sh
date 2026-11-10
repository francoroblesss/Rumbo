#!/usr/bin/env bash
# Cruza las reservas de k6 con MySQL de Operaciones: cada Idempotency-Key que recibió 201 (en k6 o en
# los reintentos) tiene que tener exactamente una fila en claves_idempotencia y una solicitud, y
# ninguna clave que recibió solo 503 puede haber dejado una reserva. Necesita mysql levantado.
# Desde la raíz: bash pruebas/fallas/verificar-reservas.sh > docs/evidencias/postmortem/reservas.txt
set -euo pipefail
EV="${EVIDENCIAS:-docs/evidencias/postmortem}"
PASS=$(grep -E '^OPERACIONES_DB_PASSWORD=' .env | cut -d= -f2-)
sql() { docker compose exec -T mysql mysql --default-character-set=utf8mb4 -uoperaciones -p"$PASS" operaciones -N -e "$1" 2>/dev/null; }

# Claves por resultado final según k6 + reintentos (una clave reintentada cuenta con su último estado).
node --input-type=module -e "
import { leerPedidos } from './pruebas/fallas/resumir.mjs';
import { readFileSync, writeFileSync } from 'node:fs';
const ultimo = new Map();
for (const p of leerPedidos('$EV/k6.json').filter((p) => p.operacion === 'reservar')) ultimo.set(p.clave, p.estado);
for (const l of readFileSync('$EV/reintentos.txt', 'utf8').split('\n')) {
  const m = l.match(/clave=(\S+) .*→ (\d{3})/);
  if (m) ultimo.set(m[1], m[2]);
}
const por = (e) => [...ultimo].filter(([, v]) => v === e).map(([k]) => k);
writeFileSync('/tmp/claves-201.txt', por('201').join('\n'));
writeFileSync('/tmp/claves-503.txt', por('503').join('\n'));
console.log('claves con 201: ' + por('201').length + ' · con 503 (sin reintentar): ' + por('503').length + ' · con 409: ' + por('409').length);
"
lista() { sed "s/.*/'&'/" "$1" | paste -sd, -; }
echo "Verificación contra MySQL de Operaciones ($(date -u +%Y-%m-%dT%H:%M:%SZ))"
echo "- claves con 201 que tienen fila en claves_idempotencia (endpoint reserva, estado 201): $(sql "SELECT COUNT(*) FROM claves_idempotencia WHERE endpoint='reserva' AND estado_http=201 AND clave IN ($(lista /tmp/claves-201.txt));")"
echo "- filas por clave (máximo; la clave primaria impide más de una): $(sql "SELECT COALESCE(MAX(n),0) FROM (SELECT COUNT(*) n FROM claves_idempotencia WHERE endpoint='reserva' AND clave IN ($(lista /tmp/claves-201.txt)) GROUP BY organizacion_id, clave) x;")"
echo "- claves que recibieron solo 503 y tienen fila (reserva hecha sin que el consumidor lo sepa): $(sql "SELECT COUNT(*) FROM claves_idempotencia WHERE endpoint='reserva' AND clave IN ($(lista /tmp/claves-503.txt));")"
echo "- solicitudes con referencia_externa = una clave con 201 (el cuerpo usa la clave como referencia): $(sql "SELECT COUNT(*) FROM solicitudes WHERE referencia_externa IN ($(lista /tmp/claves-201.txt));")"
echo "- referencias externas repetidas entre esas solicitudes: $(sql "SELECT COUNT(*) FROM (SELECT referencia_externa FROM solicitudes WHERE referencia_externa IN ($(lista /tmp/claves-201.txt)) GROUP BY referencia_externa HAVING COUNT(*) > 1) x;")"
