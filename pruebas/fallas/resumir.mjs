// Resume la salida de k6 de la caída provocada (docs/evidencias/postmortem/k6.json): cuántos 201,
// 503, 200, etc. hubo por minuto y por operación, y el total. Escribe resumen.md.
//
// Uso (desde la raíz): node pruebas/fallas/resumir.mjs [k6.json] [resumen.md]
import { readFileSync, writeFileSync } from 'node:fs';

const entrada = process.argv[2] || 'docs/evidencias/postmortem/k6.json';
const salida = process.argv[3] || 'docs/evidencias/postmortem/resumen.md';

// leerPedidos devuelve un pedido por punto de la métrica http_reqs (k6 emite uno por request).
export function leerPedidos(archivo) {
  const pedidos = [];
  for (const linea of readFileSync(archivo, 'utf8').split('\n')) {
    if (!linea.includes('"http_reqs"')) continue;
    const p = JSON.parse(linea);
    if (p.type !== 'Point' || p.metric !== 'http_reqs') continue;
    const t = p.data.tags;
    pedidos.push({ hora: new Date(p.data.time), operacion: t.operacion, estado: t.status, clave: t.clave, franja: t.franja, fase: t.fase });
  }
  return pedidos;
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

  const md = `# Resumen de k6 de la caída provocada

Generado por \`pruebas/fallas/resumir.mjs\` a partir de \`k6.json\`. Minuto 0 = primer pedido (${new Date(t0).toISOString()}). Horas en UTC. Incluye las reservas de \`setup()\` (minuto 0).

## Totales
${tabla(totales, ['operación', ...estados, 'total'])}

## Por minuto
${tabla(filas, ['minuto', 'desde (UTC)', 'operación', ...estados])}
`;
  writeFileSync(salida, md);
  console.log(md);
}
