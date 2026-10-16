// Mide p50 y p99 de las llamadas salientes para ADR-016, a partir de los spans de cliente de Tempo
// (exactos) y de la métrica http.client.request.duration de Prometheus (por buckets), a través del
// proxy de datasources de Grafana (perfil observabilidad levantado).
// Uso, desde la raíz: node pruebas/carga/latencias-d5.mjs <desde_epoch_s> <hasta_epoch_s>
//   (sin argumentos: la ventana de resultado-d5.json, es decir la corrida de d5.js)
import { readFileSync, existsSync } from 'node:fs';

const GRAFANA = process.env.GRAFANA || 'http://localhost:3000';
const AUTH = 'Basic ' + Buffer.from(process.env.GRAFANA_AUTH || 'admin:admin').toString('base64');

let [desde, hasta] = process.argv.slice(2).map(Number);
if (!desde && existsSync('pruebas/carga/resultado-d5.json')) {
  const r = JSON.parse(readFileSync('pruebas/carga/resultado-d5.json', 'utf8'));
  hasta = Math.ceil(Date.parse(r.fecha) / 1000);
  desde = hasta - 40;
}

async function get(ruta, params) {
  const res = await fetch(`${GRAFANA}${ruta}?${new URLSearchParams(params)}`, { headers: { Authorization: AUTH } });
  if (!res.ok) throw new Error(`${ruta}: ${res.status} ${await res.text()}`);
  return res.json();
}

function percentil(ordenados, p) {
  if (!ordenados.length) return null;
  return ordenados[Math.min(ordenados.length - 1, Math.ceil((p / 100) * ordenados.length) - 1)];
}

// spans de cliente HTTP hacia un servidor de destino, en ms.
async function spans(servidor, ini, fin) {
  const r = await get('/api/datasources/proxy/uid/tempo/api/search', {
    q: `{ kind = client && span.server.address = "${servidor}" }`,
    start: ini, end: fin, limit: 1000, spss: 50,
  });
  const porSpan = new Map(); // spanSet y spanSets repiten los mismos spans
  for (const t of r.traces || []) {
    for (const s of (t.spanSets || [t.spanSet]).flatMap((x) => x?.spans || [])) porSpan.set(s.spanID, Number(s.durationNanos) / 1e6);
  }
  return [...porSpan.values()].sort((a, b) => a - b);
}

async function prometheus(servidor, ini, fin) {
  // Los servicios exportan métricas cada 60 s: la ventana tiene que abarcar al menos dos muestras.
  const ventana = `${Math.max(180, fin - ini + 120)}s`;
  fin += 60;
  const q = (p) => `histogram_quantile(${p}, sum by (le) (increase(http_client_request_duration_seconds_bucket{server_address="${servidor}"}[${ventana}])))`;
  const valor = async (p) => {
    const r = await get('/api/datasources/proxy/uid/prometheus/api/v1/query', { query: q(p), time: fin });
    const v = r.data.result[0]?.value[1];
    return v && v !== 'NaN' ? Number(v) * 1000 : null;
  };
  return { p50_ms: await valor(0.5), p99_ms: await valor(0.99) };
}

const fmt = (v) => (v === null ? '—' : v.toFixed(1));
console.log(`Ventana: ${new Date(desde * 1000).toISOString()} → ${new Date(hasta * 1000).toISOString()}`);
for (const servidor of ['operaciones', 'clima-mock']) {
  const ms = await spans(servidor, desde, hasta);
  const prom = await prometheus(servidor, desde, hasta);
  console.log(`${servidor}: Tempo n=${ms.length} p50=${fmt(percentil(ms, 50))} ms p99=${fmt(percentil(ms, 99))} ms max=${fmt(ms.at(-1) ?? null)} ms` +
    ` | Prometheus (buckets) p50=${fmt(prom.p50_ms)} ms p99=${fmt(prom.p99_ms)} ms`);
}
