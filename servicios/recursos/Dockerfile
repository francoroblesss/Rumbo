# Imagen multi-stage: se compila con Go y se ejecuta solo el binario en una imagen mínima.
# El contexto de build es servicios/ (ver compose.yaml) para poder incluir el módulo comun/.
FROM golang:1.26-alpine AS compilacion
WORKDIR /src
COPY comun/ comun/
COPY recursos/go.mod recursos/go.sum recursos/
WORKDIR /src/recursos
RUN go mod download
COPY recursos/ .
RUN CGO_ENABLED=0 go build -o /servicio .

FROM alpine:3.20
COPY --from=compilacion /servicio /servicio
EXPOSE 8083
ENTRYPOINT ["/servicio"]
