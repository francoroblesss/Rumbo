# Aplicación web de Rumbo (ADR-029): se compila con Node y la sirve un NGINX como archivos estáticos.

# 1. Compilación
FROM node:22-alpine AS compilacion
WORKDIR /web
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# La URL del gateway queda fija en el bundle al compilar (Vite reemplaza import.meta.env).
ARG VITE_API_URL=http://localhost:8080
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

# 2. Servidor de archivos estáticos
FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=compilacion /web/dist /usr/share/nginx/html
EXPOSE 80
