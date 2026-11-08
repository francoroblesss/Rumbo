#!/usr/bin/env sh
# Despierta los servicios de Rumbo desplegados en Render antes de una demo (ADR-026).
# En el plan gratuito un servicio se duerme a los 15 minutos sin tráfico y tarda ~1 minuto en
# volver. Pide /salud al gateway y a cada servicio cada 5 s, hasta que los cuatro respondan 200
# o pasen 3 minutos.
#
# Uso:
#   IDENTIDAD_URL=https://… OPERACIONES_URL=https://… INTEGRACION_URL=https://… \
#     infra/render/despertar.sh https://<gateway>.onrender.com
# Si solo se pasa la URL del gateway, el gateway despierta al resto solo (DESPERTAR_AL_ARRANCAR)
# y el script espera a que el gateway responda.
set -u

if [ $# -ne 1 ]; then
  echo "uso: $0 <URL del gateway>" >&2
  exit 2
fi

destinos="$1 ${IDENTIDAD_URL:-} ${OPERACIONES_URL:-} ${INTEGRACION_URL:-}"
limite=$(( $(date +%s) + 180 ))

for destino in $destinos; do
  printf 'despertando %s ' "$destino"
  until [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$destino/salud")" = "200" ]; do
    if [ "$(date +%s)" -ge "$limite" ]; then
      echo "no respondió en 3 minutos"
      exit 1
    fi
    printf '.'
    sleep 5
  done
  echo "despierto"
done
echo "listo: todos los servicios responden /salud"
