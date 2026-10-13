package mensajeria

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
)

func TestNuevoSobreCompletaLosCamposDelContrato(t *testing.T) {
	ctx := observabilidad.ConCorrelacion(context.Background(), "corr-1")
	s, err := NuevoSobre(ctx, "operaciones.solicitud.aceptada", "sol-1", 4, map[string]string{"estado": "ACEPTADA"})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if s.ID == "" || s.Version != 1 || s.CorrelationID != "corr-1" || s.AgregadoID != "sol-1" || s.AgregadoVersion != 4 {
		t.Fatalf("sobre incompleto: %+v", s)
	}
	var datos map[string]string
	if err := s.LeerDatos(&datos); err != nil || datos["estado"] != "ACEPTADA" {
		t.Fatalf("datos mal leídos: %v %v", datos, err)
	}
}

func TestLeerDatosInvalidosEsPermanente(t *testing.T) {
	s := Sobre{Tipo: "x", Datos: []byte(`"no es un objeto"`)}
	var destino struct{ A int }
	if err := s.LeerDatos(&destino); !EsPermanente(err) {
		t.Fatalf("se esperaba error permanente, se obtuvo %v", err)
	}
}

func TestPermanenteSeReconoceEnvuelto(t *testing.T) {
	err := errors.Join(errors.New("contexto"), Permanente(errors.New("payload inválido")))
	if !EsPermanente(err) {
		t.Fatal("un error permanente envuelto tiene que seguir siendo permanente")
	}
	if EsPermanente(errors.New("timeout")) {
		t.Fatal("un error común no es permanente")
	}
}

func TestEsperaCreceExponencialmenteConJitter(t *testing.T) {
	esperados := []time.Duration{time.Second, 4 * time.Second, 16 * time.Second}
	for n, base := range esperados {
		for i := 0; i < 50; i++ {
			e := Espera(n)
			if e < base*8/10 || e > base*12/10 {
				t.Fatalf("Espera(%d) = %v, fuera de ±20%% de %v", n, e, base)
			}
		}
	}
}

func TestEncabezadosPropaganClaves(t *testing.T) {
	h := encabezados{}
	h.Set("traceparent", "00-abc-def-01")
	if h.Get("traceparent") != "00-abc-def-01" || len(h.Keys()) != 1 || h.Get("otra") != "" {
		t.Fatalf("carrier mal implementado: %+v", h)
	}
}

func TestLeerReintentos(t *testing.T) {
	if leerReintentos(map[string]any{headerReintentos: int32(2)}) != 2 || leerReintentos(map[string]any{}) != 0 {
		t.Fatal("conteo de reintentos incorrecto")
	}
}
