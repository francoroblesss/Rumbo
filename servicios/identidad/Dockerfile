# Imagen multi-stage: se compila con Go y se ejecuta solo el binario en una imagen mínima.
FROM golang:1.25-alpine AS compilacion
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /servicio .

FROM alpine:3.20
COPY --from=compilacion /servicio /servicio
EXPOSE 8081
ENTRYPOINT ["/servicio"]
