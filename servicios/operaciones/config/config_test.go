package config

import (
	"slices"
	"testing"
)

func TestAeropuertoPalabras(t *testing.T) {
	t.Setenv("AEROPUERTO_PALABRAS", " Aeropuerto , SACO,,Pajas Blancas ,Córdoba ")
	if got := lista("AEROPUERTO_PALABRAS", ""); !slices.Equal(got, []string{"aeropuerto", "saco", "pajas blancas", "cordoba"}) {
		t.Fatalf("palabras = %q", got)
	}
	t.Setenv("AEROPUERTO_PALABRAS", "")
	if got := lista("AEROPUERTO_PALABRAS", "aeropuerto,saco,pajas blancas"); !slices.Equal(got, []string{"aeropuerto", "saco", "pajas blancas"}) {
		t.Fatalf("por defecto = %q", got)
	}
}
