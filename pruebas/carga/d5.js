// HISTÓRICO: es la medición de ADR-016 (contrato 3.0.0) y ya no corre contra el sistema actual. Las
// solicitudes externas ya no se aceptan a mano (ADR-021) y el clima ya no está en /v1 (ADR-020).
// ejecutar.sh no lo lanza salvo con MEDIR_D5=1. Se conserva como evidencia de cómo se midió.
//
// Genera llamadas salientes para validar los timeouts de D5 (ADR-016). Se corre en paralelo con el
// escalón de 10 req/s del escenario C (ejecutar.sh lo lanza a tiempo) durante 30 s:
//   - Integración → Operaciones: cancelaciones de solicitudes RECIBIDAS creadas en setup (2/s).
//   - Operaciones → clima: aceptaciones de solicitudes al aeropuerto con hora dentro de las
//     próximas 6 h (1/s). El adaptador cachea el clima 5 min (ADR-012), así que la mayoría no sale
//     al proveedor; para tener muestras suficientes del proveedor se consulta además el mock
//     directamente, con la misma request que arma el adaptador (1/s).
// Las latencias se leen después de http.client.request.duration (Prometheus) y de Tempo; el
// resumen de k6 queda en resultado-d5.json como referencia.
import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';

const INTEGRACION = __ENV.INTEGRACION || 'http://balanceador-integracion';
const OPERACIONES = __ENV.OPERACIONES || 'http://operaciones:8082';
const CLIMA = __ENV.CLIMA || 'http://clima-mock:4010';
const DURACION = 30; // s

const EXTERNO = {
  'Content-Type': 'application/json',
  'X-Rumbo-Organizacion': 'org-almacenamiento',
  'X-Rumbo-Roles': 'sistema_externo',
};
const OPERADOR = {
  'Content-Type': 'application/json',
  'X-Rumbo-Usuario': 'usr-prueba-carga',
  'X-Rumbo-Roles': 'operador',
};

export const options = {
  scenarios: {
    cancelaciones: { executor: 'constant-arrival-rate', rate: 2, timeUnit: '1s', duration: `${DURACION}s`, preAllocatedVUs: 5, exec: 'cancelar' },
    aceptaciones: { executor: 'constant-arrival-rate', rate: 1, timeUnit: '1s', duration: `${DURACION}s`, preAllocatedVUs: 5, exec: 'aceptar' },
    clima_directo: { executor: 'constant-arrival-rate', rate: 1, timeUnit: '1s', duration: `${DURACION}s`, preAllocatedVUs: 2, exec: 'climaDirecto' },
  },
  thresholds: {
    'http_req_duration{operacion:cancelar}': ['max>=0'],
    'http_req_duration{operacion:aceptar}': ['max>=0'],
    'http_req_duration{operacion:clima}': ['max>=0'],
    'http_req_failed{operacion:cancelar}': ['rate>=0'],
    'http_req_failed{operacion:aceptar}': ['rate>=0'],
  },
  summaryTrendStats: ['med', 'p(95)', 'p(99)', 'max'],
  setupTimeout: '120s',
};

// horaEnTresHoras devuelve ahora + 3 h (dentro de la ventana de 6 h) en RFC 3339 con zona -03:00:
// la hora local de Córdoba es la UTC menos 3 h, así que +3 h y −3 h se cancelan.
function horaEnTresHoras() {
  return new Date().toISOString().replace(/\.\d+Z$/, '-03:00');
}

function crear(conHora) {
  const cuerpo = {
    cosa: { descripcion: 'equipaje', cantidad: 2 },
    origen: { nombre: 'Hotel', direccion: 'Bv. San Juan 100, Córdoba' },
    destino: { nombre: 'Aeropuerto Taravella', direccion: 'Av. La Voz del Interior 8500, Córdoba' },
  };
  if (conHora) cuerpo.hora_llegada_solicitada = horaEnTresHoras();
  const headers = Object.assign({ 'Idempotency-Key': crypto.randomUUID() }, EXTERNO);
  const res = http.post(`${INTEGRACION}/traslados`, JSON.stringify(cuerpo), { headers, tags: { fase: 'setup' } });
  return res.status === 202 ? res.json('id') : null;
}

export function setup() {
  const paraCancelar = [];
  const paraAceptar = [];
  for (let i = 0; i < 2 * DURACION + 5; i++) paraCancelar.push(crear(false));
  for (let i = 0; i < DURACION + 5; i++) paraAceptar.push(crear(true));
  sleep(5); // que Operaciones procese los comandos de creación (llegan por RabbitMQ)
  return { paraCancelar: paraCancelar.filter(Boolean), paraAceptar: paraAceptar.filter(Boolean) };
}

export function cancelar(datos) {
  const id = datos.paraCancelar[exec.scenario.iterationInTest % datos.paraCancelar.length];
  const headers = Object.assign({ 'Idempotency-Key': crypto.randomUUID() }, EXTERNO);
  const res = http.post(`${INTEGRACION}/traslados/${id}/cancelacion`, JSON.stringify({ motivo: 'prueba de D5' }),
    { headers, tags: { operacion: 'cancelar' } });
  check(res, { 'cancelación 200': (r) => r.status === 200 });
}

export function aceptar(datos) {
  const id = datos.paraAceptar[exec.scenario.iterationInTest % datos.paraAceptar.length];
  const res = http.post(`${OPERACIONES}/solicitudes/${id}/aceptacion`, JSON.stringify({ aeropuerto_extremo: 'DESTINO' }),
    { headers: OPERADOR, tags: { operacion: 'aceptar' } });
  check(res, { 'aceptación 200': (r) => r.status === 200 });
}

export function climaDirecto() {
  const res = http.get(`${CLIMA}/v1/airports/SACO/conditions`,
    { headers: { 'X-API-Key': 'demo', Accept: 'application/json' }, tags: { operacion: 'clima' } });
  check(res, { 'clima 200': (r) => r.status === 200 });
}

export function handleSummary(data) {
  const m = data.metrics;
  const fila = (op) => {
    const d = m[`http_req_duration{operacion:${op}}`];
    const f = m[`http_req_failed{operacion:${op}}`];
    return d ? { p50_ms: d.values.med, p95_ms: d.values['p(95)'], p99_ms: d.values['p(99)'], max_ms: d.values.max,
      errores_pct: f ? f.values.rate * 100 : null } : null;
  };
  const resultado = { fecha: new Date().toISOString(), cancelar: fila('cancelar'), aceptar: fila('aceptar'), clima_directo: fila('clima') };
  return {
    '/scripts/resultado-d5.json': JSON.stringify(resultado, null, 2),
    stdout: JSON.stringify(resultado, null, 2) + '\n',
  };
}
