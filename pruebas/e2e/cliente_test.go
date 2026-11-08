//go:build e2e

package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

// Helpers de los tests de punta a punta: hablan solo con el gateway, como un cliente real.

const (
	plazoAsincrono = 15 * time.Second // lo que se espera un efecto asíncrono (ADR-028)
	sondeo         = time.Second      // más seguido chocaría con el límite de 1 pedido por segundo por token
	maxReintentos  = 5                // reintentos ante un 429
)

var gateway = valor("E2E_GATEWAY_URL", "http://localhost:8080")

var clienteHTTP = &http.Client{Timeout: 10 * time.Second}

// zona es la hora de Argentina, en la que se expresan las fechas de las franjas.
var zona = func() *time.Location {
	if z, err := time.LoadLocation("America/Argentina/Cordoba"); err == nil {
		return z
	}
	return time.FixedZone("-03", -3*60*60)
}()

func valor(nombre, porDefecto string) string {
	if v := os.Getenv(nombre); v != "" {
		return v
	}
	return porDefecto
}

func obligatoria(t *testing.T, nombre string) string {
	t.Helper()
	v := os.Getenv(nombre)
	if v == "" {
		t.Fatalf("falta la variable de entorno %s (sale de .env)", nombre)
	}
	return v
}

// respuesta es lo que devuelve hacer: estado y cuerpo ya leído.
type respuesta struct {
	estado int
	cuerpo []byte
}

// leer decodifica el cuerpo JSON en destino.
func (r respuesta) leer(t *testing.T, destino any) {
	t.Helper()
	if err := json.Unmarshal(r.cuerpo, destino); err != nil {
		t.Fatalf("el cuerpo no es el JSON esperado (%v): %s", err, r.cuerpo)
	}
}

// codigo devuelve el campo codigo de un Problem Details.
func (r respuesta) codigo() string {
	var p struct {
		Codigo string `json:"codigo"`
	}
	_ = json.Unmarshal(r.cuerpo, &p)
	return p.Codigo
}

// hacer manda un pedido al gateway. Respeta el límite de tráfico como un consumidor real
// (teórico 9): ante un 429 espera los segundos de Retry-After y repite el MISMO pedido (mismo
// cuerpo y mismos headers, incluida la Idempotency-Key), hasta maxReintentos veces.
func hacer(t *testing.T, metodo, ruta, token string, cuerpo any, headers map[string]string) respuesta {
	t.Helper()
	var datos []byte
	if cuerpo != nil {
		var err error
		if datos, err = json.Marshal(cuerpo); err != nil {
			t.Fatal(err)
		}
	}
	for intento := 0; ; intento++ {
		req, err := http.NewRequest(metodo, gateway+ruta, bytes.NewReader(datos))
		if err != nil {
			t.Fatal(err)
		}
		if cuerpo != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := clienteHTTP.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", metodo, ruta, err)
		}
		leido, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("%s %s: leer la respuesta: %v", metodo, ruta, err)
		}
		if resp.StatusCode != http.StatusTooManyRequests || intento == maxReintentos {
			return respuesta{estado: resp.StatusCode, cuerpo: leido}
		}
		espera, err := strconv.Atoi(resp.Header.Get("Retry-After"))
		if err != nil || espera < 1 {
			espera = 1
		}
		t.Logf("429 en %s %s: espero %d s (Retry-After) y repito el mismo pedido", metodo, ruta, espera)
		time.Sleep(time.Duration(espera) * time.Second)
	}
}

// esperar exige un estado HTTP y devuelve la respuesta.
func esperar(t *testing.T, r respuesta, estado int, que string) respuesta {
	t.Helper()
	if r.estado != estado {
		t.Fatalf("%s: estado %d, se esperaba %d: %s", que, r.estado, estado, r.cuerpo)
	}
	return r
}

// esperarHasta sondea la condición cada segundo hasta que se cumple o vence el plazo. Lo
// asíncrono (outbox, consumidores) es eventualmente consistente (teórico 2): no se puede pedir el
// resultado en el mismo instante, y un time.Sleep fijo sería lento o frágil. Si vence, falla con
// el último valor visto.
func esperarHasta(t *testing.T, plazo time.Duration, condicion func() (bool, string)) {
	t.Helper()
	limite := time.Now().Add(plazo)
	for {
		ok, ultimo := condicion()
		if ok {
			return
		}
		if time.Now().After(limite) {
			t.Fatalf("no se cumplió en %s; último valor: %s", plazo, ultimo)
		}
		time.Sleep(sondeo)
	}
}

// token pide un token y devuelve el access_token.
func token(t *testing.T, ruta string, credenciales map[string]string) string {
	t.Helper()
	r := esperar(t, hacer(t, http.MethodPost, ruta, "", credenciales, nil), http.StatusOK, "POST "+ruta)
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	r.leer(t, &tok)
	return tok.AccessToken
}

// pedirToken obtiene el token del sistema externo (client credentials del contrato público).
func pedirToken(t *testing.T) string {
	return token(t, "/auth/token", map[string]string{
		"client_id": obligatoria(t, "EXTERNO_CLIENT_ID"), "client_secret": obligatoria(t, "EXTERNO_CLIENT_SECRET")})
}

// login obtiene el token de un usuario de la web; prefijo es OPERADOR o CLIENTE (variables de .env).
func login(t *testing.T, prefijo string) string {
	return token(t, "/api/identidad/login", map[string]string{
		"email": obligatoria(t, prefijo+"_EMAIL"), "password": obligatoria(t, prefijo+"_PASSWORD")})
}

// nuevaClave genera una Idempotency-Key distinta por intención.
func nuevaClave() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "e2e-" + hex.EncodeToString(b)
}

// Tipos de las respuestas que leen los tests (solo los campos que se verifican).

type franjaDisponible struct {
	Inicio               time.Time `json:"inicio"`
	Disponible           bool      `json:"disponible"`
	TrasladosDisponibles int       `json:"traslados_disponibles"`
}

type franjaOperador struct {
	Inicio              time.Time `json:"inicio"`
	CupoTraslados       int       `json:"cupo_traslados"`
	TrasladosReservados int       `json:"traslados_reservados"`
}

type traslado struct {
	ID           string     `json:"id"`
	Estado       string     `json:"estado"`
	FinalizadoEn *time.Time `json:"finalizado_en"`
}

type aviso struct {
	Titulo  string `json:"titulo"`
	Entidad struct {
		ID string `json:"id"`
	} `json:"entidad"`
}

// franja es la franja elegida para un escenario.
type franja struct {
	fecha  string // AAAA-MM-DD
	inicio string // RFC 3339 con -03:00, tal como lo devuelve la disponibilidad
	cupo   int
}

// franjaLibre elige, para el día hoy + dias, una franja reservable con todo el cupo libre. Cada
// escenario usa un día distinto y toma una franja sin reservas, así los tests se pueden correr
// varias veces sobre el mismo sistema sin pisarse. La disponibilidad pública dice qué franjas se
// pueden reservar; la vista del operador dice cuáles no tienen ninguna reserva.
func franjaLibre(t *testing.T, tokenExterno, tokenOperador string, dias int) franja {
	t.Helper()
	fecha := time.Now().In(zona).AddDate(0, 0, dias).Format(time.DateOnly)
	var disp struct {
		Franjas []franjaDisponible `json:"franjas"`
	}
	esperar(t, hacer(t, http.MethodGet, "/disponibilidad?fecha="+fecha, tokenExterno, nil, nil), http.StatusOK, "disponibilidad").leer(t, &disp)
	var ocupacion struct {
		Franjas []franjaOperador `json:"franjas"`
	}
	esperar(t, hacer(t, http.MethodGet, "/api/operaciones/franjas?fecha="+fecha, tokenOperador, nil, nil), http.StatusOK, "franjas del operador").leer(t, &ocupacion)
	for _, d := range disp.Franjas {
		for _, o := range ocupacion.Franjas {
			if o.Inicio.Equal(d.Inicio) && o.TrasladosReservados == 0 && d.Disponible && d.TrasladosDisponibles == o.CupoTraslados {
				return franja{fecha: fecha, inicio: d.Inicio.In(zona).Format(time.RFC3339), cupo: d.TrasladosDisponibles}
			}
		}
	}
	t.Fatalf("no hay ninguna franja sin reservas el %s (¿se corrieron los tests muchas veces sobre el mismo sistema?)", fecha)
	return franja{}
}

// pedidoReserva arma el cuerpo de POST /traslados. Origen y destino no mencionan el aeropuerto:
// el e2e no depende del servicio de clima del otro grupo (ADR-025).
func pedidoReserva(f franja, descripcion string) map[string]any {
	return map[string]any{
		"referencia_externa": "e2e-" + descripcion,
		"cosa":               map[string]any{"descripcion": descripcion, "cantidad": 2, "peso_kg": 50},
		"origen":             map[string]any{"nombre": "Depósito central", "direccion": "Av. Colón 1000, Córdoba"},
		"destino":            map[string]any{"nombre": "Sucursal Nueva Córdoba", "direccion": "Obispo Trejo 300, Córdoba"},
		"franja_inicio":      f.inicio,
	}
}

// reservar hace POST /traslados con una clave nueva.
func reservar(t *testing.T, tokenExterno string, f franja, descripcion string) respuesta {
	t.Helper()
	return hacer(t, http.MethodPost, "/traslados", tokenExterno, pedidoReserva(f, descripcion),
		map[string]string{"Idempotency-Key": nuevaClave()})
}

// reservadosEn devuelve cuántos traslados tiene reservados la franja, según el operador.
func reservadosEn(t *testing.T, tokenOperador string, f franja) int {
	t.Helper()
	var ocupacion struct {
		Franjas []franjaOperador `json:"franjas"`
	}
	esperar(t, hacer(t, http.MethodGet, "/api/operaciones/franjas?fecha="+f.fecha, tokenOperador, nil, nil), http.StatusOK, "franjas del operador").leer(t, &ocupacion)
	inicio, _ := time.Parse(time.RFC3339, f.inicio)
	for _, o := range ocupacion.Franjas {
		if o.Inicio.Equal(inicio) {
			return o.TrasladosReservados
		}
	}
	t.Fatalf("la franja %s no aparece en las franjas del operador", f.inicio)
	return 0
}

// disponiblesEn devuelve los lugares libres de la franja según la disponibilidad pública (0 si ya
// no figura como reservable).
func disponiblesEn(t *testing.T, tokenExterno string, f franja) int {
	t.Helper()
	var disp struct {
		Franjas []franjaDisponible `json:"franjas"`
	}
	esperar(t, hacer(t, http.MethodGet, "/disponibilidad?fecha="+f.fecha, tokenExterno, nil, nil), http.StatusOK, "disponibilidad").leer(t, &disp)
	inicio, _ := time.Parse(time.RFC3339, f.inicio)
	for _, d := range disp.Franjas {
		if d.Inicio.Equal(inicio) {
			return d.TrasladosDisponibles
		}
	}
	return 0
}

// estadoPublico devuelve el traslado tal como lo ve el sistema externo (GET /traslados/{id}).
func estadoPublico(t *testing.T, tokenExterno, id string) traslado {
	t.Helper()
	var tr traslado
	esperar(t, hacer(t, http.MethodGet, "/traslados/"+id, tokenExterno, nil, nil), http.StatusOK, "GET /traslados/{id}").leer(t, &tr)
	return tr
}

// tieneAvisos indica si la bandeja tiene los avisos con esos títulos para la entidad id.
func tieneAvisos(t *testing.T, tokenUsuario, id string, titulos ...string) (bool, string) {
	t.Helper()
	var bandeja struct {
		Resultados []aviso `json:"resultados"`
	}
	esperar(t, hacer(t, http.MethodGet, "/api/notificaciones/avisos?tamanio=100", tokenUsuario, nil, nil), http.StatusOK, "bandeja").leer(t, &bandeja)
	vistos := map[string]bool{}
	for _, a := range bandeja.Resultados {
		if a.Entidad.ID == id {
			vistos[a.Titulo] = true
		}
	}
	for _, titulo := range titulos {
		if !vistos[titulo] {
			return false, fmt.Sprintf("avisos de %s: %v (falta %q)", id, vistos, titulo)
		}
	}
	return true, ""
}
