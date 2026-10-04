// Crea una base y un usuario por servicio (ADR-003): cada servicio solo puede
// leer y escribir su propia base. Se ejecuta una sola vez, al crear el volumen.

const servicios = [
  { base: "identidad", usuario: "identidad", clave: process.env.IDENTIDAD_DB_PASSWORD },
  { base: "notificaciones", usuario: "notificaciones", clave: process.env.NOTIFICACIONES_DB_PASSWORD },
  { base: "integracion", usuario: "integracion", clave: process.env.INTEGRACION_DB_PASSWORD },
];

for (const s of servicios) {
  db.getSiblingDB(s.base).createUser({
    user: s.usuario,
    pwd: s.clave,
    roles: [{ role: "readWrite", db: s.base }],
  });
}
