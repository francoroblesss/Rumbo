#!/bin/sh
# Crea una base de datos y un usuario por servicio (ADR-003, ADR-005): cada servicio es dueño de
# su propia base y no puede acceder a la de otro. Se ejecuta una sola vez, al crear el volumen.
set -e

mysql -uroot -p"$MYSQL_ROOT_PASSWORD" <<SQL
CREATE DATABASE operaciones CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE USER 'operaciones'@'%' IDENTIFIED BY '${OPERACIONES_DB_PASSWORD}';
GRANT ALL PRIVILEGES ON operaciones.* TO 'operaciones'@'%';

CREATE DATABASE recursos CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE USER 'recursos'@'%' IDENTIFIED BY '${RECURSOS_DB_PASSWORD}';
GRANT ALL PRIVILEGES ON recursos.* TO 'recursos'@'%';
SQL
