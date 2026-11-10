// Tráfico de la caída provocada de Operaciones (consigna §2.9, docs/POSTMORTEM.md).
//
// Durante 15 minutos, por el gateway (como lo ve el consumidor del contrato), con el token del
// sistema externo pedido en setup():
//   - 0,4 reservas por segundo (POST /traslados), cada una con su Idempotency-Key, sobre franjas
//     de los próximos días;
//   - 0,4 consultas por segundo (GET /traslados/{id}) de traslados reservados en setup();
//   - 0,1 consultas por segundo de GET /disponibilidad de mañana (también depende de Operaciones).
// Son 54 pedidos por minuto: no llega al límite del gateway (60 por minuto por organización).
//
// Cada pedido queda en la salida JSON de k6 (--out json) con su hora, la operación, el estado HTTP
// (tag status) y, en las reservas, la Idempotency-Key y la franja (tags clave y franja): con esos
// dos datos se rearma el mismo cuerpo para reintentar (reintentar.mjs).
//
// Uso (lo corre caida.sh):
//   docker run --rm --network rumbo_default -v "$PWD:/trabajo" grafana/k6:1.3.0 run \
//     -e CLIENT_ID=... -e CLIENT_SECRET=... --out json=/trabajo/docs/evidencias/postmortem/k6.json \
//     /trabajo/pruebas/fallas/caida-operaciones.js
import http from 'k6/http';
import { check, sleep } from 'k6';

const GATEWAY = __ENV.GATEWAY || 'http://gateway:8080';
const DURACION = __ENV.DURACION || '15m';
const RESERVAS_INICIALES = 6; // las que después se consultan por id

export const options = {
  scenarios: {
    reservas: {
      executor: 'constant-arrival-rate',
      rate: 2,
      timeUnit: '5s', // 0,4 por segundo
      duration: DURACION,
      preAllocatedVUs: 4,
      maxVUs: 20,
      exec: 'reservar',
    },
    consultas: {
      executor: 'constant-arrival-rate',
      rate: 2,
      timeUnit: '5s',
      duration: DURACION,
      preAllocatedVUs: 4,
      maxVUs: 20,
      exec: 'consultar',
    },
    disponibilidad: {
      executor: 'constant-arrival-rate',
      rate: 1,
      timeUnit: '10s', // 0,1 por segundo
      duration: DURACION,
      preAllocatedVUs: 2,
      maxVUs: 10,
      exec: 'consultarDisponibilidad',
    },
  },
  setupTimeout: '120s',
  // Solo los tags que se usan: la salida JSON queda más chica.
  systemTags: ['status', 'method', 'scenario'],
};

// cuerpoReserva arma el cuerpo de POST /traslados a partir de la clave y la franja. Es
// determinístico a propósito: reintentar.mjs tiene la misma función para reenviar el mismo cuerpo
// con la misma clave.
export function cuerpoReserva(clave, franja) {
  return JSON.stringify({
    referencia_externa: clave,
    cosa: { descripcion: 'cajas de mercadería seca', cantidad: 4, peso_kg: 80 },
    origen: { nombre: 'Depósito central', direccion: 'Av. Colón 1000, Córdoba' },
    destino: { nombre: 'Sucursal Nueva Córdoba', direccion: 'Obispo Trejo 300, Córdoba' },
    franja_inicio: franja,
  });
}

// franjaAlAzar devuelve el inicio de una franja (8, 10, …, 18 h de Córdoba) entre pasado mañana y
// dentro de 29 días, en RFC 3339 con -03:00.
function franjaAlAzar() {
  const dias = 2 + Math.floor(Math.random() * 28);
  const fecha = new Date(Date.now() - 3 * 3600 * 1000 + dias * 24 * 3600 * 1000).toISOString().slice(0, 10);
  const hora = String(8 + 2 * Math.floor(Math.random() * 6)).padStart(2, '0');
  return `${fecha}T${hora}:00:00-03:00`;
}

function nuevaReserva(token, fase) {
  const clave = crypto.randomUUID();
  const franja = franjaAlAzar();
  const res = http.post(`${GATEWAY}/traslados`, cuerpoReserva(clave, franja), {
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}`, 'Idempotency-Key': clave },
    tags: { operacion: 'reservar', clave, franja, fase },
  });
  return res;
}

export function setup() {
  const credenciales = JSON.stringify({ client_id: __ENV.CLIENT_ID, client_secret: __ENV.CLIENT_SECRET });
  const res = http.post(`${GATEWAY}/auth/token`, credenciales, {
    headers: { 'Content-Type': 'application/json' },
    tags: { operacion: 'token' },
  });
  if (res.status !== 200) {
    throw new Error(`no se pudo obtener el token: ${res.status} ${res.body}`);
  }
  const token = res.json('access_token');
  const ids = [];
  for (let i = 0; i < RESERVAS_INICIALES; i++) {
    const r = nuevaReserva(token, 'setup');
    if (r.status === 201) {
      ids.push(r.json('id'));
    }
    sleep(1.5); // sin ráfagas: el límite del gateway es por organización
  }
  if (ids.length === 0) {
    throw new Error('setup no pudo reservar ningún traslado: ¿está levantado el sistema (con cupo alto)?');
  }
  return { token, ids, inicio: new Date().toISOString() };
}

export function reservar(datos) {
  const res = nuevaReserva(datos.token, 'trafico');
  check(res, { 'reserva 201': (r) => r.status === 201 });
}

export function consultar(datos) {
  const id = datos.ids[Math.floor(Math.random() * datos.ids.length)];
  const res = http.get(`${GATEWAY}/traslados/${id}`, {
    headers: { Authorization: `Bearer ${datos.token}` },
    tags: { operacion: 'consultar' },
  });
  check(res, { 'consulta 200': (r) => r.status === 200 });
}

// consultarDisponibilidad pide las franjas de mañana (fecha de Córdoba, AAAA-MM-DD). Con Operaciones
// caído tiene que responder 503 reintentable con Retry-After (ADR-008, ADR-021).
export function consultarDisponibilidad(datos) {
  const manana = new Date(Date.now() - 3 * 3600 * 1000 + 24 * 3600 * 1000).toISOString().slice(0, 10);
  const res = http.get(`${GATEWAY}/disponibilidad?fecha=${manana}&peso_kg=50`, {
    headers: { Authorization: `Bearer ${datos.token}` },
    tags: { operacion: 'disponibilidad' },
  });
  check(res, {
    'disponibilidad 200': (r) => r.status === 200,
    '503 con Retry-After': (r) => r.status !== 503 || !!r.headers['Retry-After'],
  });
}
