// Prueba de carga de la capacidad publicada "Solicitar traslado" (método de ADR-015).
//
// Escalones de tasa constante (1, 2, 5, 10, 20, 50 y 100 req/s, 30 s cada uno), uno detrás de
// otro. Mezcla: 90 % GET /v2/traslados/{id} (el polling del consumidor) y 10 % POST /v2/traslados.
// Se ataca al balanceador de Integración dentro de la red de Docker, con los headers de identidad
// que pondría el gateway (su límite de 60 pedidos por minuto cortaría la prueba).
//
// Uso (lo corre ejecutar.sh):
//   docker run --rm --network rumbo_default -v "$PWD/pruebas/carga:/scripts" \
//     grafana/k6 run /scripts/traslados.js -e ESCENARIO=A
import http from 'k6/http';
import { check } from 'k6';

const DESTINO = __ENV.DESTINO || 'http://balanceador-integracion';
const ESCENARIO = __ENV.ESCENARIO || 'X';
const ESCALONES = [1, 2, 5, 10, 20, 50, 100]; // req/s
const DURACION_ESCALON = 30; // s
const SOLICITUDES_INICIALES = 20;

const HEADERS = {
  'Content-Type': 'application/json',
  'X-Rumbo-Organizacion': 'org-almacenamiento',
  'X-Rumbo-Roles': 'sistema_externo',
};

// Un escenario constant-arrival-rate por escalón, secuenciales con startTime. Cada uno lleva la
// etiqueta "escalon" para separar sus resultados; los thresholds triviales fuerzan a k6 a calcular
// las submétricas por escalón (que después lee handleSummary).
const escenarios = {};
const thresholds = {};
ESCALONES.forEach((tasa, i) => {
  escenarios[`escalon_${tasa}`] = {
    executor: 'constant-arrival-rate',
    rate: tasa,
    timeUnit: '1s',
    duration: `${DURACION_ESCALON}s`,
    startTime: `${i * DURACION_ESCALON}s`,
    preAllocatedVUs: Math.max(5, tasa),
    maxVUs: Math.max(20, tasa * 4),
    gracefulStop: '5s',
    tags: { escalon: String(tasa) },
  };
  thresholds[`http_req_duration{escalon:${tasa}}`] = ['max>=0'];
  thresholds[`http_req_failed{escalon:${tasa}}`] = ['rate>=0'];
  thresholds[`http_reqs{escalon:${tasa}}`] = ['count>=0'];
});

export const options = {
  scenarios: escenarios,
  thresholds,
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max'],
  setupTimeout: '60s',
};

function nuevoTraslado() {
  return JSON.stringify({
    referencia_externa: `k6-${ESCENARIO}-${Date.now()}`,
    cosa: { descripcion: 'cajas de mercadería seca', cantidad: 4, peso_kg: 80 },
    origen: { nombre: 'Depósito central', direccion: 'Av. Colón 1000, Córdoba' },
    destino: { nombre: 'Sucursal Nueva Córdoba', direccion: 'Obispo Trejo 300, Córdoba' },
    tolerancia_minutos: 30,
  });
}

function crear(tags) {
  const headers = Object.assign({ 'Idempotency-Key': crypto.randomUUID() }, HEADERS);
  return http.post(`${DESTINO}/v2/traslados`, nuevoTraslado(), { headers, tags });
}

// setup crea las solicitudes que después se consultan por id.
export function setup() {
  const ids = [];
  for (let i = 0; i < SOLICITUDES_INICIALES; i++) {
    const res = crear({ fase: 'setup' });
    if (res.status === 202) {
      ids.push(res.json('id'));
    }
  }
  if (ids.length === 0) {
    throw new Error('setup no pudo crear ninguna solicitud: ¿está levantado el sistema?');
  }
  // inicio marca cuándo arrancan los escalones: resumir.mjs lo cruza con docker stats.
  return { ids, inicio: Math.floor(Date.now() / 1000) };
}

export default function (datos) {
  if (Math.random() < 0.9) {
    const id = datos.ids[Math.floor(Math.random() * datos.ids.length)];
    const res = http.get(`${DESTINO}/v2/traslados/${id}`, { headers: HEADERS, tags: { operacion: 'consultar' } });
    check(res, { 'consulta 200': (r) => r.status === 200 });
  } else {
    const res = crear({ operacion: 'crear' });
    check(res, { 'creación 202': (r) => r.status === 202 });
  }
}

// handleSummary deja un JSON chico con lo que pide ADR-015 por escalón, además del resumen en consola.
export function handleSummary(data) {
  const m = data.metrics;
  const resultado = {
    escenario: ESCENARIO,
    fecha: new Date().toISOString(),
    inicio_escalones: data.setup_data ? data.setup_data.inicio : null,
    duracion_escalon_s: DURACION_ESCALON,
    escalones: ESCALONES.map((tasa) => {
      const dur = m[`http_req_duration{escalon:${tasa}}`];
      const fallas = m[`http_req_failed{escalon:${tasa}}`];
      const reqs = m[`http_reqs{escalon:${tasa}}`];
      const cantidad = reqs ? reqs.values.count : 0;
      return {
        objetivo_rps: tasa,
        logrado_rps: Number((cantidad / DURACION_ESCALON).toFixed(2)),
        requests: cantidad,
        p95_ms: dur ? Number(dur.values['p(95)'].toFixed(1)) : null,
        p99_ms: dur ? Number(dur.values['p(99)'].toFixed(1)) : null,
        errores_pct: fallas ? Number((fallas.values.rate * 100).toFixed(2)) : null,
      };
    }),
  };
  return {
    [`/scripts/resultado-${ESCENARIO}.json`]: JSON.stringify(resultado, null, 2),
    stdout: JSON.stringify(resultado, null, 2) + '\n',
  };
}
