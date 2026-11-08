//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"
)

// TestReservaIdempotente: el mismo POST /traslados dos veces con la misma Idempotency-Key
// devuelve la misma respuesta y ocupa un solo lugar (teórico 7: clave idempotente por intención).
func TestReservaIdempotente(t *testing.T) {
	externo, operador := pedirToken(t), login(t, "OPERADOR")
	f := franjaLibre(t, externo, operador, 2)
	cuerpo := pedidoReserva(f, "Reserva idempotente")
	clave := map[string]string{"Idempotency-Key": nuevaClave()}

	var primera, segunda traslado
	esperar(t, hacer(t, http.MethodPost, "/traslados", externo, cuerpo, clave), http.StatusCreated, "primera reserva").leer(t, &primera)
	esperar(t, hacer(t, http.MethodPost, "/traslados", externo, cuerpo, clave), http.StatusCreated, "reintento").leer(t, &segunda)
	if primera.ID == "" || segunda.ID != primera.ID {
		t.Fatalf("el reintento con la misma clave devuelve la misma reserva: %s y %s", primera.ID, segunda.ID)
	}
	if n := reservadosEn(t, operador, f); n != 1 {
		t.Fatalf("la franja tiene %d reservados, se esperaba 1", n)
	}
}

// TestFranjaLlena: reservar hasta agotar el cupo de una franja y una más → 409 SIN_DISPONIBILIDAD.
func TestFranjaLlena(t *testing.T) {
	externo, operador := pedirToken(t), login(t, "OPERADOR")
	f := franjaLibre(t, externo, operador, 3)
	for i := 1; i <= f.cupo; i++ {
		esperar(t, reservar(t, externo, f, fmt.Sprintf("Franja llena %d", i)), http.StatusCreated, fmt.Sprintf("reserva %d de %d", i, f.cupo))
	}
	r := esperar(t, reservar(t, externo, f, "Franja llena, una más"), http.StatusConflict, "reserva con la franja llena")
	if c := r.codigo(); c != "SIN_DISPONIBILIDAD" {
		t.Fatalf("codigo %q, se esperaba SIN_DISPONIBILIDAD", c)
	}
}

// TestCancelacionDelCliente: el cliente cancela desde la web; la franja recupera el lugar y el
// sistema externo ve CANCELADO (R-45).
func TestCancelacionDelCliente(t *testing.T) {
	externo, operador, cliente := pedirToken(t), login(t, "OPERADOR"), login(t, "CLIENTE")
	f := franjaLibre(t, externo, operador, 4)
	var reserva traslado
	esperar(t, reservar(t, externo, f, "Cancelación del cliente"), http.StatusCreated, "POST /traslados").leer(t, &reserva)

	esperar(t, hacer(t, http.MethodPost, "/api/operaciones/solicitudes/"+reserva.ID+"/cancelacion", cliente,
		map[string]string{"motivo": "Prueba de punta a punta"}, map[string]string{"Idempotency-Key": nuevaClave()}),
		http.StatusOK, "cancelación del cliente")

	esperarHasta(t, plazoAsincrono, func() (bool, string) {
		n := disponiblesEn(t, externo, f)
		return n == f.cupo, fmt.Sprintf("%d lugares libres de %d", n, f.cupo)
	})
	esperarHasta(t, plazoAsincrono, func() (bool, string) {
		tr := estadoPublico(t, externo, reserva.ID)
		return tr.Estado == "CANCELADO", fmt.Sprintf("%+v", tr)
	})
}

// TestSinToken: sin Authorization, el gateway responde 401.
func TestSinToken(t *testing.T) {
	r := esperar(t, hacer(t, http.MethodPost, "/traslados", "", map[string]string{}, map[string]string{"Idempotency-Key": nuevaClave()}),
		http.StatusUnauthorized, "POST /traslados sin token")
	if c := r.codigo(); c != "NO_AUTENTICADO" {
		t.Fatalf("codigo %q, se esperaba NO_AUTENTICADO", c)
	}
}
