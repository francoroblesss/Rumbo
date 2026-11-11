#!/bin/sh
# Usuarios del MySQL de logs (ADR-023), con las contraseñas de .env:
#   - rumbo_logs: los servicios solo pueden insertar;
#   - grafana_lector: Grafana solo puede leer (como el grafana_reader de la clase práctica 8).
set -e

mysql -uroot -p"$MYSQL_ROOT_PASSWORD" <<SQL
CREATE USER 'rumbo_logs'@'%' IDENTIFIED BY '${LOGS_DB_PASSWORD}';
GRANT INSERT ON observabilidad.logs TO 'rumbo_logs'@'%';
CREATE USER 'grafana_lector'@'%' IDENTIFIED BY '${GRAFANA_LOGS_DB_PASSWORD}';
GRANT SELECT ON observabilidad.logs TO 'grafana_lector'@'%';
SQL
