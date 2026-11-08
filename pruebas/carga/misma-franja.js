// Prueba de carga de reservas simultáneas sobre la misma franja (ADR-022, D4; método de ADR-015).
//
// Mide cuánto cuesta la serialización por el SELECT … FOR UPDATE de la franja: cada reserva bloquea
// la fila de su franja hasta el fin de la transacción, así que las reservas sobre UNA franja se
// atienden de a una. Dos modos, con los mismos escalones:
//   MODO=misma       todas las reservas van a la franja FRANJA (la que elige ejecutar-misma-franja.sh);
//   MODO=repartidas  control: las reservas rotan entre las franjas de los próximos 29 días (menos FRANJA),
//                    así casi nunca dos reservas esperan por la misma fila.
//
// Escenario ramping-arrival-rate: escalones de 10, 25, 50 y 100 reservas por segundo, cada uno con
// 5 s de rampa y 30 s de meseta. Solo la meseta entra en la tabla (etiqueta "escalon"); la rampa
// queda con escalon=rampa. Cada reserva lleva una Idempotency-Key distinta (es otra intención).
//
// Destino: el balanceador de Integración dentro de la red de Docker, como la prueba de la Entrega 2:
// el límite del gateway (60 pedidos por minuto) cortaría la prueba. El token del sistema externo se
// pide UNA vez al gateway en setup(), y de él salen los headers de identidad que pondría el gateway
// (X-Rumbo-Usuario, X-Rumbo-Organizacion, X-Rumbo-Roles); se agrega el secreto interno (ADR-026),
// sin el cual Integración responde 401.
//
// Uso (lo corre ejecutar-misma-franja.sh, que pasa las variables desde .env):
//   docker run --rm --network rumbo_default -v "$PWD/pruebas/carga:/scripts" grafana/k6:1.3.0 \
//     run /scripts/misma-franja.js -e MODO=misma -e FRANJA=2026-11-12T10:00:00-03:00 \
//     -e CLIENT_ID=... -e CLIENT_SECRET=... -e SECRETO_INTERNO=...
import http from 'k6/http';
import encoding from 'k6/encoding';
import exec from 'k6/execution';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const DESTINO = __ENV.DESTINO || 'http://balanceador-integracion';
const GATEWAY = __ENV.GATEWAY || 'http://gateway:8080';
const MODO = __ENV.MODO || 'misma';
const FRANJA = __ENV.FRANJA;
const ESCALONES = [10, 25, 50, 100]; // reservas por segundo
const RAMPA = 5; // s
const MESETA = 30; // s

if (MODO !== 'misma' && MODO !== 'repartidas') {
  throw new Error(`MODO inválido: ${MODO} (misma | repartidas)`);
}
if (!FRANJA) {
  throw new Error('falta FRANJA (inicio de la franja en RFC 3339 con -03:00)');
}

// Reservas que respondieron 201: la verificación compara su total con traslados_reservados de la franja.
const reservas201 = new Counter('reservas_201');

const etapas = [];
const thresholds = {};
ESCALONES.forEach((tasa) => {
  etapas.push({ target: tasa, duration: `${RAMPA}s` });
  etapas.push({ target: tasa, duration: `${MESETA}s` });
  // Thresholds triviales: fuerzan a k6 a calcular las submétricas por escalón (las lee handleSummary).
  thresholds[`http_req_duration{escalon:${tasa}}`] = ['max>=0'];
  thresholds[`http_req_failed{escalon:${tasa}}`] = ['rate>=0'];
  thresholds[`http_reqs{escalon:${tasa}}`] = ['count>=0'];
  thresholds[`reservas_201{escalon:${tasa}}`] = ['count>=0'];
});

export const options = {
  scenarios: {
    reservas: {
      executor: 'ramping-arrival-rate',
      startRate: ESCALONES[0],
      timeUnit: '1s',
      stages: etapas,
      preAllocatedVUs: 100,
      maxVUs: 400,
      gracefulStop: '10s',
    },
  },
  thresholds,
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max'],
  setupTimeout: '30s',
};

// escalonActual dice en qué tramo del escenario está la iteración: el valor del escalón si está en
// su meseta, "rampa" si está subiendo al siguiente.
function escalonActual() {
  const t = (Date.now() - exec.scenario.startTime) / 1000;
  const i = Math.floor(t / (RAMPA + MESETA));
  if (i >= ESCALONES.length) return String(ESCALONES[ESCALONES.length - 1]); // gracefulStop
  return t - i * (RAMPA + MESETA) < RAMPA ? 'rampa' : String(ESCALONES[i]);
}

// franjasRepartidas: inicio de cada franja (8, 10, …, 18 h de Córdoba) de mañana a dentro de 29
// días, sin FRANJA. Son 173 franjas: a 100 reservas/s, cada una recibe ~0,6 por segundo.
function franjasRepartidas() {
  const franjas = [];
  for (let dia = 1; dia <= 29; dia++) {
    const fecha = new Date(Date.now() - 3 * 3600 * 1000 + dia * 24 * 3600 * 1000).toISOString().slice(0, 10);
    for (let hora = 8; hora <= 18; hora += 2) {
      const inicio = `${fecha}T${String(hora).padStart(2, '0')}:00:00-03:00`;
      if (inicio !== FRANJA) franjas.push(inicio);
    }
  }
  return franjas;
}

// setup pide el token al gateway y arma los headers que el gateway pondría al reenviar.
export function setup() {
  const res = http.post(`${GATEWAY}/auth/token`,
    JSON.stringify({ client_id: __ENV.CLIENT_ID, client_secret: __ENV.CLIENT_SECRET }),
    { headers: { 'Content-Type': 'application/json' }, tags: { fase: 'setup' } });
  if (res.status !== 200) {
    throw new Error(`no se pudo obtener el token (${res.status}): ¿está levantado el sistema?`);
  }
  const datos = JSON.parse(encoding.b64decode(res.json('access_token').split('.')[1], 'rawurl', 's'));
  return {
    headers: {
      'Content-Type': 'application/json',
      'X-Rumbo-Usuario': datos.sub,
      'X-Rumbo-Organizacion': datos.org,
      'X-Rumbo-Roles': datos.roles.join(','),
      'X-Rumbo-Interno': __ENV.SECRETO_INTERNO,
    },
    franjas: MODO === 'misma' ? [FRANJA] : franjasRepartidas(),
    // Cuándo arranca el escenario: resumir-misma-franja.mjs lo cruza con las muestras de bloqueos.
    inicio: Math.floor(Date.now() / 1000),
  };
}

export default function (datos) {
  const franjas = datos.franjas;
  const franja = franjas[exec.scenario.iterationInTest % franjas.length];
  const cuerpo = JSON.stringify({
    referencia_externa: `k6-${MODO}-${exec.scenario.iterationInTest}`,
    cosa: { descripcion: 'cajas de mercadería seca', cantidad: 4, peso_kg: 80 },
    origen: { nombre: 'Depósito central', direccion: 'Av. Colón 1000, Córdoba' },
    destino: { nombre: 'Sucursal Nueva Córdoba', direccion: 'Obispo Trejo 300, Córdoba' },
    franja_inicio: franja,
  });
  const escalon = escalonActual();
  const headers = Object.assign({ 'Idempotency-Key': crypto.randomUUID() }, datos.headers);
  const res = http.post(`${DESTINO}/traslados`, cuerpo, {
    headers,
    tags: { escalon },
    responseCallback: http.expectedStatuses(201),
  });
  if (res.status === 201) {
    reservas201.add(1, { escalon });
  }
  check(res, { 'reserva 201': (r) => r.status === 201 });
}

// handleSummary deja resultado-<modo>.json con lo que lee resumir-misma-franja.mjs.
export function handleSummary(data) {
  const m = data.metrics;
  const valor = (nombre, clave) => (m[nombre] ? m[nombre].values[clave] : null);
  const redondear = (x) => (x === null || x === undefined ? null : Number(x.toFixed(1)));
  const resultado = {
    modo: MODO,
    franja: FRANJA,
    fecha: new Date().toISOString(),
    inicio_escenario: data.setup_data ? data.setup_data.inicio : null,
    rampa_s: RAMPA,
    meseta_s: MESETA,
    reservas_201_total: valor('reservas_201', 'count') || 0,
    requests_total: valor('http_reqs', 'count') || 0,
    escalones: ESCALONES.map((tasa) => {
      const dur = `http_req_duration{escalon:${tasa}}`;
      const cantidad = valor(`http_reqs{escalon:${tasa}}`, 'count') || 0;
      const fallas = valor(`http_req_failed{escalon:${tasa}}`, 'rate');
      return {
        objetivo_rps: tasa,
        logrado_rps: Number((cantidad / MESETA).toFixed(2)),
        requests: cantidad,
        reservas_201: valor(`reservas_201{escalon:${tasa}}`, 'count') || 0,
        p50_ms: redondear(valor(dur, 'med')),
        p95_ms: redondear(valor(dur, 'p(95)')),
        p99_ms: redondear(valor(dur, 'p(99)')),
        max_ms: redondear(valor(dur, 'max')),
        errores_pct: fallas === null ? null : Number((fallas * 100).toFixed(2)),
      };
    }),
  };
  return {
    [`/scripts/resultado-${MODO}.json`]: JSON.stringify(resultado, null, 2),
    stdout: JSON.stringify(resultado, null, 2) + '\n',
  };
}
