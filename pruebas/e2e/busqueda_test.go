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

// TestBusquedaTodasLasPalabras: "bulto rojo" trae solo el bulto rojo, aunque se ordene por fecha
// (el orden por defecto, el de la web): tienen que aparecer todas las palabras (mm=100%, ADR-033).
// Usa palabras que no aparecen en la demo ("caja grafana"), porque las reservas del e2e quedan en
// la base: así una corrida sobre un sistema que dura no le suma resultados a esa búsqueda.
func TestBusquedaTodasLasPalabras(t *testing.T) {
	externo, operador := pedirToken(t), login(t, "OPERADOR")
	suf := sufijoLetras(8)
	// Una franja para cada reserva: no depende de que el cupo por franja sea 2 o más.
	f1, f2 := franjaLibre(t, externo, operador, 5), franjaLibre(t, externo, operador, 6)

	var rojo traslado
	esperar(t, reservar(t, externo, f1, "Bulto rojo "+suf), http.StatusCreated, "reserva del bulto rojo").leer(t, &rojo)
	esperar(t, reservar(t, externo, f2, "Bulto azul "+suf), http.StatusCreated, "reserva del bulto azul")

	// El índice es asincrónico (ADR-011 promete ≤ 5 s).
	esperarHasta(t, plazoAsincrono, func() (bool, string) {
		p := buscarTexto(t, operador, "bulto "+suf)
		return p.Total == 2, fmt.Sprintf("total %d", p.Total)
	})

	// Sin orden: por fecha, el orden por defecto (el de la web).
	p := buscarTexto(t, operador, "bulto rojo "+suf)
	if p.Total != 1 || len(p.Resultados) != 1 || p.Resultados[0].SolicitudID != rojo.ID {
		t.Fatalf("\"bulto rojo %s\": total %d, resultados %+v; se esperaba solo %s", suf, p.Total, p.Resultados, rojo.ID)
	}
}
