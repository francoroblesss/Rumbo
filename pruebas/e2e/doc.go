// Package e2e contiene los tests de punta a punta del flujo de la Presentación (ADR-028): hablan
// solo con el gateway, como un cliente real, contra el sistema levantado con Docker Compose.
//
// Los archivos de test llevan el build tag e2e, así `go test ./...` no los corre sin el sistema
// levantado. Para correrlos:
//
//	docker compose up --build -d --wait
//	export $(grep -E '^(EXTERNO_CLIENT_|OPERADOR_|CLIENTE_)' .env | xargs)   # credenciales de .env
//	cd pruebas/e2e && go test -tags e2e -count=1 -v ./...
package e2e
