// Consulta por id con Memcached prendido y apagado (ADR-007): cuánto tarda GET /traslados/{id}
// según el resultado de la caché.
//
// 20 req/s constantes de GET /traslados/{id} durante 9 min, sobre traslados reservados en setup()
// (durante la prueba no se reserva). Lo corre cache-encendida-apagada.sh, que apaga Memcached en el
// minuto 3 y lo vuelve a prender en el 6. Como traslados.js, se ataca al balanceador de Integración
// dentro de la red de Docker, con los headers de identidad que pondría el gateway (su límite de 60
// pedidos por minuto cortaría la prueba). Además, como misma-franja.js, el secreto interno
// (X-Rumbo-Interno, ADR-026): sin él Integración responde 401.
//
// Uso (lo corre cache-encendida-apagada.sh):
//   docker run --rm --network rumbo_default -v "$PWD/pruebas/carga:/scripts" \
//     grafana/k6:1.3.0 run -e SECRETO_INTERNO=... /scripts/consulta-por-id.js
import http from 'k6/http';
import { check } from 'k6';

const DESTINO = __ENV.DESTINO || 'http://balanceador-integracion';
const SOLICITUDES_INICIALES = 20;

const HEADERS = {
  'Content-Type': 'application/json',
  'X-Rumbo-Organizacion': 'org-almacenamiento',
  'X-Rumbo-Roles': 'sistema_externo',
  'X-Rumbo-Interno': __ENV.SECRETO_INTERNO,
};

export const options = {
  scenarios: {
    consultas: {
      executor: 'constant-arrival-rate',
      rate: 20,
      timeUnit: '1s',
      duration: '9m',
      preAllocatedVUs: 20,
      maxVUs: 80,
    },
  },
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max'],
  setupTimeout: '60s',
};

// franjaAlAzar devuelve el inicio de una franja (8, 10, …, 18 h de Córdoba) entre mañana y dentro
// de 29 días, en RFC 3339 con -03:00.
function franjaAlAzar() {
  const dias = 1 + Math.floor(Math.random() * 29);
  const fecha = new Date(Date.now() - 3 * 3600 * 1000 + dias * 24 * 3600 * 1000).toISOString().slice(0, 10);
  const hora = String(8 + 2 * Math.floor(Math.random() * 6)).padStart(2, '0');
  return `${fecha}T${hora}:00:00-03:00`;
}

function nuevoTraslado() {
  return JSON.stringify({
    referencia_externa: `k6-cache-${Date.now()}`,
    cosa: { descripcion: 'cajas de mercadería seca', cantidad: 4, peso_kg: 80 },
    origen: { nombre: 'Depósito central', direccion: 'Av. Colón 1000, Córdoba' },
    destino: { nombre: 'Sucursal Nueva Córdoba', direccion: 'Obispo Trejo 300, Córdoba' },
    franja_inicio: franjaAlAzar(),
  });
}

// setup crea las reservas que después se consultan por id.
export function setup() {
  const ids = [];
  for (let i = 0; i < SOLICITUDES_INICIALES; i++) {
    const headers = Object.assign({ 'Idempotency-Key': crypto.randomUUID() }, HEADERS);
    const res = http.post(`${DESTINO}/traslados`, nuevoTraslado(), { headers, tags: { fase: 'setup' } });
    if (res.status === 201) {
      ids.push(res.json('id'));
    }
  }
  if (ids.length === 0) {
    throw new Error('setup no pudo reservar ningún traslado: ¿está levantado el sistema (con cupo alto)?');
  }
  return { ids };
}

export default function (datos) {
  const id = datos.ids[Math.floor(Math.random() * datos.ids.length)];
  const res = http.get(`${DESTINO}/traslados/${id}`, { headers: HEADERS, tags: { operacion: 'consultar' } });
  check(res, { 'consulta 200': (r) => r.status === 200 });
}
