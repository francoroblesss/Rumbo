package dominio

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// ExtremoAeropuerto detecta qué punta del traslado es el aeropuerto (R-46, ADR-025): ORIGEN si
// el nombre o la dirección del origen contienen alguna de las palabras; si no, DESTINO con la
// misma regla sobre el destino; si no, nil (el traslado no consulta el clima). Es una regla pura:
// no llama a nadie, por eso corre dentro de la transacción de la reserva.
func ExtremoAeropuerto(origen, destino Ubicacion, palabras []string) *Extremo {
	for _, punta := range []struct {
		ubicacion Ubicacion
		extremo   Extremo
	}{{origen, ExtremoOrigen}, {destino, ExtremoDestino}} {
		texto := Normalizar(punta.ubicacion.Nombre + " " + punta.ubicacion.Direccion)
		for _, palabra := range palabras {
			if p := Normalizar(palabra); p != "" && strings.Contains(texto, p) {
				extremo := punta.extremo
				return &extremo
			}
		}
	}
	return nil
}

// Normalizar pasa el texto a minúsculas y le quita las tildes, para comparar sin distinguirlas
// ("Córdoba" y "CORDOBA" quedan iguales). Usa golang.org/x/text: descompone cada letra (NFD),
// borra las marcas diacríticas y vuelve a componer (NFC).
func Normalizar(texto string) string {
	sinTildes, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), texto)
	if err != nil {
		sinTildes = texto
	}
	return strings.ToLower(strings.TrimSpace(sinTildes))
}
