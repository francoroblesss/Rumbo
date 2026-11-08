// Cruza los resultados de misma-franja.js con las muestras de bloqueos de InnoDB y arma las tablas
// de docs/capacidad/informe-presentacion.md: misma franja contra franjas repartidas, por escalón.
// Verifica además que la franja no se sobrerreservó (traslados_reservados == cantidad de 201).
// Uso, desde la raíz: node pruebas/carga/resumir-misma-franja.mjs
import { readFileSync } from 'node:fs';

const CARPETA = 'pruebas/carga';
const MODOS = ['misma', 'repartidas'];
const SLO_P95_MS = 300; // ADR-014
const SLO_ERRORES_PCT = 1;
const CONTENEDORES = ['operaciones', 'mysql', 'integracion-1', 'integracion-2'];
const MARGEN_S = 5; // se descartan los primeros segundos de la meseta (como resumir.mjs)

// leerStats lee stats-<modo>.txt (mismo formato que ejecutar.sh): [{t, cpu: {servicio: %}}].
function leerStats(archivo) {
  const muestras = [];
  let actual = null;
  for (const linea of readFileSync(archivo, 'utf8').split('\n')) {
    if (linea.startsWith('--- ')) {
      actual = { t: Number(linea.slice(4)), cpu: {} };
      muestras.push(actual);
      continue;
    }
    const [nombre, cpu, mem] = linea.split('\t');
    if (!actual || !mem) continue;
    // "rumbo-integracion-1-1" → "integracion-1"
    actual.cpu[nombre.replace(/^[^-]+-/, '').replace(/-\d+$/, '')] = parseFloat(cpu);
  }
  return muestras;
}

// mediana de la CPU de cada contenedor en la ventana (la mediana, por los picos de los healthchecks;
// ver resumir.mjs y ADR-015).
function cpuEn(muestras, desde, hasta) {
  const dentro = muestras.filter((m) => m.t >= desde + MARGEN_S && m.t <= hasta);
  const cpu = {};
  for (const c of CONTENEDORES) {
    const xs = dentro.map((m) => m.cpu[c]).filter((x) => x !== undefined).sort((a, b) => a - b);
    const k = Math.floor(xs.length / 2);
    cpu[c] = xs.length ? (xs.length % 2 ? xs[k] : (xs[k - 1] + xs[k]) / 2) : null;
  }
  return cpu;
}

// leerBloqueos devuelve [{t, Innodb_row_lock_waits, Innodb_row_lock_time, ...}] de bloqueos-<modo>.txt.
function leerBloqueos(archivo) {
  const muestras = [];
  let actual = null;
  for (const linea of readFileSync(archivo, 'utf8').split('\n')) {
    if (linea.startsWith('--- ')) {
      actual = { t: Number(linea.slice(4)) };
      muestras.push(actual);
      continue;
    }
    const [nombre, valor] = linea.split('\t');
    if (actual && valor !== undefined) actual[nombre] = Number(valor);
  }
  return muestras.filter((m) => m.Innodb_row_lock_waits !== undefined);
}

// bloqueosEn calcula, para la ventana [desde, hasta], cuántas esperas por bloqueo de fila hubo, el
// tiempo total esperando y cuántas transacciones esperaban a la vez en promedio. Los contadores
// son acumulados: se resta la última muestra antes de la ventana a la primera después.
function bloqueosEn(muestras, desde, hasta) {
  const antes = muestras.filter((m) => m.t <= desde).at(-1);
  const despues = muestras.find((m) => m.t >= hasta);
  if (!antes || !despues) return null;
  const esperas = despues.Innodb_row_lock_waits - antes.Innodb_row_lock_waits;
  const tiempoMs = despues.Innodb_row_lock_time - antes.Innodb_row_lock_time;
  const dentro = muestras.filter((m) => m.t > desde && m.t < hasta);
  const aLaVez = dentro.length
    ? dentro.reduce((a, m) => a + m.Innodb_row_lock_current_waits, 0) / dentro.length
    : null;
  return { esperas, tiempoMs, mediaMs: esperas ? tiempoMs / esperas : 0, aLaVez };
}

const fmt = (x, d = 1) => (x === null || x === undefined ? '—' : x.toFixed(d).replace('.', ','));

const filas = {};
const totales = {};
for (const modo of MODOS) {
  const r = JSON.parse(readFileSync(`${CARPETA}/resultado-${modo}.json`, 'utf8'));
  const muestras = leerBloqueos(`${CARPETA}/bloqueos-${modo}.txt`);
  const stats = leerStats(`${CARPETA}/stats-${modo}.txt`);
  totales[modo] = r;
  r.escalones.forEach((e, i) => {
    const desde = r.inicio_escenario + i * (r.rampa_s + r.meseta_s) + r.rampa_s;
    const b = bloqueosEn(muestras, desde, desde + r.meseta_s);
    const cpu = cpuEn(stats, desde, desde + r.meseta_s);
    (filas[e.objetivo_rps] ||= {})[modo] = { ...e, b, cpu };
  });
}

console.log('| Escalón (reservas/s) | Franjas | Logradas (req/s) | p50 (ms) | p95 (ms) | p99 (ms) | Errores (%) | SLO | Esperas por bloqueo | Tiempo esperando (ms) | Espera media (ms) | Esperando a la vez |');
console.log('| ---: | --- | ---: | ---: | ---: | ---: | ---: | :---: | ---: | ---: | ---: | ---: |');
const tablaCpu = [];
for (const [tasa, porModo] of Object.entries(filas)) {
  for (const modo of MODOS) {
    const e = porModo[modo];
    const slo = e.p95_ms <= SLO_P95_MS && e.errores_pct <= SLO_ERRORES_PCT ? 'sí' : 'no';
    console.log(`| ${tasa} | ${modo === 'misma' ? 'misma' : 'distintas'} | ${fmt(e.logrado_rps)} | ${fmt(e.p50_ms)} | ${fmt(e.p95_ms)} | ${fmt(e.p99_ms)} | ${fmt(e.errores_pct, 2)} | ${slo} | ${e.b ? e.b.esperas : '—'} | ${e.b ? e.b.tiempoMs : '—'} | ${e.b ? fmt(e.b.mediaMs) : '—'} | ${e.b ? fmt(e.b.aLaVez) : '—'} |`);
    tablaCpu.push(`| ${tasa} | ${modo === 'misma' ? 'misma' : 'distintas'} | ${CONTENEDORES.map((c) => fmt(e.cpu[c])).join(' | ')} |`);
  }
}

console.log(`\nCPU (% de un núcleo, mediana de la meseta)\n\n| Escalón (reservas/s) | Franjas | ${CONTENEDORES.join(' | ')} |`);
console.log(`| ---: | --- | ${CONTENEDORES.map(() => '---:').join(' | ')} |`);
tablaCpu.forEach((f) => console.log(f));

// Verificación de D4: la franja tiene exactamente un lugar ocupado por cada 201 (ni uno más).
const v = JSON.parse(readFileSync(`${CARPETA}/verificacion-misma.json`, 'utf8'));
const ocupados = v.traslados_reservados_despues - v.traslados_reservados_antes;
const creadas = totales.misma.reservas_201_total;
console.log(`\nFranja ${v.franja}: traslados_reservados ${v.traslados_reservados_antes} → ${v.traslados_reservados_despues} ` +
  `(+${ocupados}); reservas con 201: ${creadas}; requests: ${totales.misma.requests_total}. ` +
  (ocupados === creadas ? 'Coinciden: no hubo sobrerreserva.' : '¡NO COINCIDEN!'));
