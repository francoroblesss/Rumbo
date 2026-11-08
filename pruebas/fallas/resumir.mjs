// Resume la salida de k6 de la caída provocada (docs/evidencias/postmortem/k6.json): cuántos 201,
// 503, 200, etc. hubo por minuto y por operación, y el total. Escribe resumen.md.
//
// Uso (desde la raíz): node pruebas/fallas/resumir.mjs [k6.json] [resumen.md]
import { readFileSync, writeFileSync } from 'node:fs';

const entrada = process.argv[2] || 'docs/evidencias/postmortem/k6.json';
const salida = process.argv[3] || 'docs/evidencias/postmortem/resumen.md';

// leerPedidos devuelve un pedido por punto de la métrica http_reqs (k6 emite uno por request), con
// su duración: el punto de http_req_duration del mismo request, que tiene la misma hora y los
// mismos tags. La hora de k6 es la del comienzo del pedido.
export function leerPedidos(archivo) {
  const pedidos = [];
  const duraciones = new Map();
  const id = (p) => p.data.time + JSON.stringify(p.data.tags);
  for (const linea of readFileSync(archivo, 'utf8').split('\n')) {
    if (!linea.includes('"Point"')) continue;
    const p = JSON.parse(linea);
    if (p.metric === 'http_req_duration') {
      duraciones.set(id(p), p.data.value);
      continue;
    }
    if (p.metric !== 'http_reqs') continue;
    const t = p.data.tags;
    pedidos.push({ id: id(p), hora: new Date(p.data.time), operacion: t.operacion, estado: t.status, clave: t.clave, franja: t.franja, fase: t.fase });
  }
  for (const p of pedidos) p.duracionMs = duraciones.get(p.id);
  return pedidos;
}

function percentil(valores, q) {
  if (valores.length === 0) return '-';
  const v = [...valores].sort((a, b) => a - b);
  return Math.round(v[Math.min(v.length - 1, Math.ceil(q * v.length) - 1)]);
}

function tabla(filas, columnas) {
  const lineas = [`| ${columnas.join(' | ')} |`, `| ${columnas.map(() => '---').join(' | ')} |`];
  for (const f of filas) lineas.push(`| ${f.join(' | ')} |`);
  return lineas.join('\n');
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const pedidos = leerPedidos(entrada).filter((p) => p.operacion !== 'token');
  const t0 = Math.min(...pedidos.map((p) => p.hora.getTime()));
  const estados = [...new Set(pedidos.map((p) => p.estado))].sort();
  const operaciones = ['reservar', 'consultar'];

  const porMinuto = new Map();
  for (const p of pedidos) {
    const minuto = Math.floor((p.hora.getTime() - t0) / 60000);
    const clave = `${minuto}|${p.operacion}`;
    const conteo = porMinuto.get(clave) || {};
    conteo[p.estado] = (conteo[p.estado] || 0) + 1;
    porMinuto.set(clave, conteo);
  }
  const minutos = [...new Set([...porMinuto.keys()].map((k) => Number(k.split('|')[0])))].sort((a, b) => a - b);
  const filas = [];
  for (const m of minutos) {
    const desde = new Date(t0 + m * 60000).toISOString().slice(11, 19);
    for (const op of operaciones) {
      const c = porMinuto.get(`${m}|${op}`) || {};
      filas.push([m, desde, op, ...estados.map((e) => c[e] || 0)]);
    }
  }
  const totales = operaciones.map((op) => [op, ...estados.map((e) => pedidos.filter((p) => p.operacion === op && p.estado === e).length),
    pedidos.filter((p) => p.operacion === op).length]);

  // Duración de las reservas por minuto y estado (p50, p95 y máximo, en ms).
  const duraciones = [];
  for (const m of minutos) {
    for (const e of estados) {
      const d = pedidos.filter((p) => p.operacion === 'reservar' && p.estado === e &&
        Math.floor((p.hora.getTime() - t0) / 60000) === m).map((p) => p.duracionMs);
      if (d.length) duraciones.push([m, e, d.length, percentil(d, 0.5), percentil(d, 0.95), Math.round(Math.max(...d))]);
    }
  }
  const reservas = pedidos.filter((p) => p.operacion === 'reservar').sort((a, b) => a.hora - b.hora);
  const con503 = reservas.filter((p) => p.estado === '503');
  const hitos = [];
  if (con503.length) {
    const ultimo503 = con503[con503.length - 1];
    const primer201Despues = reservas.find((p) => p.estado === '201' && p.hora > con503[0].hora);
    hitos.push(['primera reserva con 503', con503[0].hora.toISOString(), `${Math.round(con503[0].duracionMs)} ms`]);
    const rapida = con503.find((p) => p.duracionMs < 100);
    if (rapida) hitos.push(['primera reserva con 503 en menos de 100 ms (sin esperar el timeout)', rapida.hora.toISOString(), `${Math.round(rapida.duracionMs)} ms`]);
    hitos.push(['última reserva con 503', ultimo503.hora.toISOString(), `${Math.round(ultimo503.duracionMs)} ms`]);
    if (primer201Despues) hitos.push(['primera reserva con 201 después de la caída', primer201Despues.hora.toISOString(), `${Math.round(primer201Despues.duracionMs)} ms`]);
  }

  const md = `# Resumen de k6 de la caída provocada

Generado por \`pruebas/fallas/resumir.mjs\` a partir de \`k6.json\`. Minuto 0 = primer pedido (${new Date(t0).toISOString()}). Horas en UTC. Incluye las reservas de \`setup()\` (minuto 0).

## Totales
${tabla(totales, ['operación', ...estados, 'total'])}

## Hitos de las reservas
${tabla(hitos, ['hito', 'hora (UTC)', 'duración'])}

## Por minuto
${tabla(filas, ['minuto', 'desde (UTC)', 'operación', ...estados])}

## Duración de las reservas por minuto y estado (ms, medida por k6)
${tabla(duraciones, ['minuto', 'estado', 'pedidos', 'p50', 'p95', 'máximo'])}
`;
  writeFileSync(salida, md);
  console.log(md);
}
