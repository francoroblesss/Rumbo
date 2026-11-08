// Paso 6 del procedimiento del postmortem: toma N reservas que dieron 503 durante la caída (de
// k6.json) y las reintenta por el gateway con la MISMA Idempotency-Key y el mismo cuerpo, como
// indica el contrato ante un 503. Imprime el resultado de cada una.
//
// Uso (desde la raíz, con el sistema levantado):
//   CLIENT_ID=... CLIENT_SECRET=... node pruebas/fallas/reintentar.mjs [N] [k6.json]
import { leerPedidos } from './resumir.mjs';

const N = Number(process.argv[2] || 3);
const entrada = process.argv[3] || 'docs/evidencias/postmortem/k6.json';
const GATEWAY = process.env.GATEWAY || 'http://localhost:8080';

// cuerpoReserva es la misma función que en caida-operaciones.js (mismo orden de campos).
function cuerpoReserva(clave, franja) {
  return JSON.stringify({
    referencia_externa: clave,
    cosa: { descripcion: 'cajas de mercadería seca', cantidad: 4, peso_kg: 80 },
    origen: { nombre: 'Depósito central', direccion: 'Av. Colón 1000, Córdoba' },
    destino: { nombre: 'Sucursal Nueva Córdoba', direccion: 'Obispo Trejo 300, Córdoba' },
    franja_inicio: franja,
  });
}

const fallidas = leerPedidos(entrada).filter((p) => p.operacion === 'reservar' && p.estado === '503');
if (fallidas.length < N) {
  console.error(`solo hay ${fallidas.length} reservas con 503 en ${entrada}`);
  process.exit(1);
}
// Una del principio, una del medio y una del final de la caída.
const elegidas = [0, Math.floor(fallidas.length / 2), fallidas.length - 1].slice(0, N).map((i) => fallidas[i]);

const token = await fetch(`${GATEWAY}/auth/token`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ client_id: process.env.CLIENT_ID, client_secret: process.env.CLIENT_SECRET }),
}).then((r) => r.json()).then((j) => j.access_token);

for (const p of elegidas) {
  const res = await fetch(`${GATEWAY}/traslados`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}`, 'Idempotency-Key': p.clave },
    body: cuerpoReserva(p.clave, p.franja),
  });
  const cuerpo = await res.json();
  console.log(`${new Date().toISOString()} clave=${p.clave} (503 a las ${p.hora.toISOString()}) → ${res.status} id=${cuerpo.id ?? '-'} estado=${cuerpo.estado ?? cuerpo.codigo}`);
}
console.log(`CLAVES=${elegidas.map((p) => p.clave).join(',')}`);
