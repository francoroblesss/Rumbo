//go:build e2e

package e2e

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// sufijoLetras genera n letras minúsculas al azar: con números o guiones el análisis de texto
// partiría la palabra, y así la búsqueda no choca con datos de corridas anteriores.
func sufijoLetras(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = 'a' + b[i]%26
	}
	return string(b)
}

type paginaBusqueda struct {
	Total      int `json:"total"`
	Resultados []struct {
		SolicitudID string `json:"solicitud_id"`
		Descripcion string `json:"descripcion"`
	} `json:"resultados"`
}

func buscarTexto(t *testing.T, tokenOperador, q string) paginaBusqueda {
	t.Helper()
	var p paginaBusqueda
	esperar(t, hacer(t, http.MethodGet, "/api/operaciones/traslados/busqueda?q="+url.QueryEscape(q), tokenOperador, nil, nil),
		http.StatusOK, "búsqueda "+q).leer(t, &p)
	return p
}

// TestBusquedaTodasLasPalabras: "caja grafana" trae solo la caja Grafana, aunque se ordene por
// fecha (el orden que usaba la web): tienen que aparecer todas las palabras (mm=100%, ADR-033).
func TestBusquedaTodasLasPalabras(t *testing.T) {
	externo, operador := pedirToken(t), login(t, "OPERADOR")
	suf := sufijoLetras(8)
	f := franjaLibre(t, externo, operador, 5)

	var grafana traslado
	esperar(t, reservar(t, externo, f, "Caja Grafana "+suf), http.StatusCreated, "reserva de la caja Grafana").leer(t, &grafana)
	esperar(t, reservar(t, externo, f, "Caja de herramientas "+suf), http.StatusCreated, "reserva de la caja de herramientas")

	// El índice es asincrónico (ADR-011 promete ≤ 5 s).
	esperarHasta(t, plazoAsincrono, func() (bool, string) {
		p := buscarTexto(t, operador, "caja "+suf)
		return p.Total == 2, fmt.Sprintf("total %d", p.Total)
	})

	// Sin orden: por fecha, como pedía la web antes del cambio.
	p := buscarTexto(t, operador, "caja grafana "+suf)
	if p.Total != 1 || len(p.Resultados) != 1 || p.Resultados[0].SolicitudID != grafana.ID {
		t.Fatalf("\"caja grafana %s\": total %d, resultados %+v; se esperaba solo %s", suf, p.Total, p.Resultados, grafana.ID)
	}
}
