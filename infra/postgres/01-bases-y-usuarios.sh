#!/bin/sh
# Crea una base de datos y un usuario por servicio (ADR-003): cada servicio es dueño de su
# propia base y no puede conectarse a la de otro. Se ejecuta una sola vez, al crear el volumen.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
	CREATE ROLE operaciones LOGIN PASSWORD '${OPERACIONES_DB_PASSWORD}';
	CREATE DATABASE operaciones OWNER operaciones;
	REVOKE CONNECT ON DATABASE operaciones FROM PUBLIC;

	CREATE ROLE recursos LOGIN PASSWORD '${RECURSOS_DB_PASSWORD}';
	CREATE DATABASE recursos OWNER recursos;
	REVOKE CONNECT ON DATABASE recursos FROM PUBLIC;
SQL

# btree_gist permite las exclusion constraints que impiden asignaciones superpuestas (D4).
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname operaciones -c "CREATE EXTENSION IF NOT EXISTS btree_gist;"
