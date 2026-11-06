// Package despertar despierta, al arrancar el gateway, los servicios del camino de la capacidad
// publicada (ADR-026). En Render gratuito un servicio se duerme a los 15 minutos sin tráfico y
// tarda ~1 minuto en volver: si el gateway esperara a que el pedido de un consumidor despierte a
// cada servicio de a uno, ese pedido superaría el deadline de 5 s. Pidiendo /salud a todos a la
// vez, apenas despierta el gateway, el resto despierta en paralelo.
//
// No agranda el deadline (teórico 7: el deadline es una decisión de producto) ni bloquea el
// arranque: corre en segundo plano.
package despertar

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Intervalo y plazo por defecto (docs/interfaces-internas.md §11.4).
const (
	Intervalo = 5 * time.Second
	Plazo     = 120 * time.Second
)

// Cadena le pide GET <url>/salud a cada destino cada intervalo, hasta recibir 200 o hasta que
// pase el plazo, y loguea el resultado de cada uno. Vuelve cuando terminaron todos.
func Cadena(ctx context.Context, cliente *http.Client, destinos []string, intervalo, plazo time.Duration) {
	var espera sync.WaitGroup
	for _, destino := range destinos {
		espera.Go(func() { uno(ctx, cliente, destino, intervalo, plazo) })
	}
	espera.Wait()
}

func uno(ctx context.Context, cliente *http.Client, destino string, intervalo, plazo time.Duration) {
	inicio := time.Now()
	ctx, cancelar := context.WithTimeout(ctx, plazo)
	defer cancelar()
	for {
		if responde(ctx, cliente, destino) {
			slog.InfoContext(ctx, "servicio despierto", slog.String("destino", destino),
				slog.Int64("espera_ms", time.Since(inicio).Milliseconds()))
			return
		}
		select {
		case <-ctx.Done():
			slog.WarnContext(ctx, "servicio no respondió al despertar", slog.String("destino", destino),
				slog.Int64("espera_ms", time.Since(inicio).Milliseconds()))
			return
		case <-time.After(intervalo):
		}
	}
}

func responde(ctx context.Context, cliente *http.Client, destino string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, destino+"/salud", nil)
	if err != nil {
		return false
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
