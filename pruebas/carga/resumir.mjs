// Cruza los resultados de k6 con docker stats y arma las tablas del informe de capacidad
// (docs/capacidad/informe-entrega-2.md), aplicando la regla del límite de ADR-015.
// Uso, desde la raíz: node pruebas/carga/resumir.mjs [A B C]
import { readFileSync, existsSync } from 'node:fs';

const CARPETA = 'pruebas/carga';
const CONTENEDORES = ['integracion-1', 'integracion-2', 'mongo', 'memcached', 'balanceador-integracion'];
const SLO_P95_MS = 300;
const SLO_ERRORES_PCT = 1;
const MARGEN_S = 5; // se descartan los primeros segundos de cada escalón (transición)

// leerStats devuelve [{t, contenedores: {nombre: {cpu, memMiB}}}] a partir de stats-<E>.txt.
function leerStats(archivo) {
  const muestras = [];
  let actual = null;
  for (const linea of readFileSync(archivo, 'utf8').split('\n')) {
    if (linea.startsWith('--- ')) {
      actual = { t: Number(linea.slice(4)), contenedores: {} };
      muestras.push(actual);
      continue;
    }
    const [nombre, cpu, mem] = linea.split('\t');
    if (!actual || !mem) continue;
    actual.contenedores[servicio(nombre)] = { cpu: parseFloat(cpu), memMiB: aMiB(mem.split('/')[0].trim()) };
  }
  return muestras;
}

// servicio pasa "rumbo-integracion-1-1" a "integracion-1".
function servicio(nombre) {
  return nombre.replace(/^[^-]+-/, '').replace(/-\d+$/, '');
}

function aMiB(valor) {
  const n = parseFloat(valor);
  if (valor.endsWith('GiB')) return n * 1024;
  if (valor.endsWith('KiB')) return n / 1024;
  if (valor.endsWith('MiB')) return n;
  return n / (1024 * 1024); // B
}

// mediana: la CPU por escalón se resume con la mediana porque los healthchecks de mongo (mongosh)
// y rabbitmq (rabbitmq-diagnostics) generan picos de 60-180 % en algunas muestras, sin relación con
// la carga. El promedio se usa solo para el costo (el proveedor cobra también esos picos).
function mediana(xs) {
  if (!xs.length) return null;
  const o = [...xs].sort((a, b) => a - b);
  const m = Math.floor(o.length / 2);
  return o.length % 2 ? o[m] : (o[m - 1] + o[m]) / 2;
}

function promedio(xs) {
  return xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : null;
}

function resumir(escenario) {
  const resultado = JSON.parse(readFileSync(`${CARPETA}/resultado-${escenario}.json`, 'utf8'));
  const muestras = leerStats(`${CARPETA}/stats-${escenario}.txt`);
  const dur = resultado.duracion_escalon_s;
  const filas = resultado.escalones.map((e, i) => {
    const desde = resultado.inicio_escalones + i * dur + MARGEN_S;
    const hasta = resultado.inicio_escalones + (i + 1) * dur;
    const enEscalon = muestras.filter((m) => m.t >= desde && m.t < hasta);
    const cpu = {};
    const mem = {};
    for (const c of CONTENEDORES) {
      const datos = enEscalon.map((m) => m.contenedores[c]).filter(Boolean);
      cpu[c] = mediana(datos.map((d) => d.cpu));
      mem[c] = promedio(datos.map((d) => d.memMiB));
    }
    const cumple = e.p95_ms <= SLO_P95_MS && e.errores_pct <= SLO_ERRORES_PCT;
    return { ...e, cpu, mem, cumple, muestras: enEscalon.length };
  });

  // Regla de ADR-015: capacidad = último escalón que cumple el SLO (sin cortes previos); límite =
  // contenedor con más CPU en el primer escalón que no cumple, o en el de 100 req/s.
  const primeroQueFalla = filas.find((f) => !f.cumple);
  const indiceFalla = primeroQueFalla ? filas.indexOf(primeroQueFalla) : -1;
  const capacidad = indiceFalla === -1 ? filas[filas.length - 1] : filas[indiceFalla - 1];
  const escalonLimite = primeroQueFalla || filas[filas.length - 1];
  const [limite, cpuLimite] = Object.entries(escalonLimite.cpu)
    .filter(([, v]) => v !== null)
    .sort((a, b) => b[1] - a[1])[0];

  const fmt = (v, d = 1) => (v === null || v === undefined ? '—' : v.toFixed(d));
  console.log(`\n### Escenario ${escenario}\n`);
  console.log(`| Escalón (req/s) | Logradas (req/s) | p95 (ms) | p99 (ms) | Errores (%) | SLO | ${CONTENEDORES.map((c) => `CPU ${c} (% mediana)`).join(' | ')} |`);
  console.log(`| ---: | ---: | ---: | ---: | ---: | :---: | ${CONTENEDORES.map(() => '---:').join(' | ')} |`);
  for (const f of filas) {
    console.log(`| ${f.objetivo_rps} | ${fmt(f.logrado_rps)} | ${fmt(f.p95_ms)} | ${fmt(f.p99_ms)} | ${fmt(f.errores_pct, 2)} | ${f.cumple ? 'sí' : 'no'} | ${CONTENEDORES.map((c) => fmt(f.cpu[c])).join(' | ')} |`);
  }
  console.log(`\nCapacidad: ${capacidad ? capacidad.objetivo_rps + ' req/s' : 'ningún escalón cumple'}` +
    `; escalón del límite: ${escalonLimite.objetivo_rps} req/s; contenedor con más CPU: ${limite} (${fmt(cpuLimite)} %)`);
  return filas;
}

const escenarios = process.argv.slice(2).length ? process.argv.slice(2) : ['A', 'B', 'C'];
const resultados = {};
for (const e of escenarios) {
  if (existsSync(`${CARPETA}/resultado-${e}.json`)) resultados[e] = resumir(e);
}

// Uso promedio en el escalón de 10 req/s del escenario C: base de la estimación de costos.
if (resultados.C) {
  const diez = resultados.C.find((f) => f.objetivo_rps === 10);
  const muestras = leerStats(`${CARPETA}/stats-C.txt`);
  const r = JSON.parse(readFileSync(`${CARPETA}/resultado-C.json`, 'utf8'));
  const i = r.escalones.findIndex((e) => e.objetivo_rps === 10);
  const desde = r.inicio_escalones + i * r.duracion_escalon_s + MARGEN_S;
  const hasta = r.inicio_escalones + (i + 1) * r.duracion_escalon_s;
  const enEscalon = muestras.filter((m) => m.t >= desde && m.t < hasta);
  const nombres = [...new Set(enEscalon.flatMap((m) => Object.keys(m.contenedores)))].sort();
  console.log(`\n### Uso por contenedor en C, escalón de 10 req/s (${diez.muestras} muestras)\n`);
  console.log('| Contenedor | CPU promedio (% de un núcleo) | RAM promedio (MiB) |');
  console.log('| --- | ---: | ---: |');
  for (const n of nombres) {
    const datos = enEscalon.map((m) => m.contenedores[n]).filter(Boolean);
    console.log(`| ${n} | ${promedio(datos.map((d) => d.cpu)).toFixed(2)} | ${promedio(datos.map((d) => d.memMiB)).toFixed(1)} |`);
  }
}
