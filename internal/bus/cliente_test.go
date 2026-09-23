package bus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// Si el bus se reinicia, el cliente puede reutilizar una conexión que ya está
// muerta. Esta prueba imita ese caso: la primera petición se corta sin
// responder y la segunda contesta bien; el cliente debe reintentar solo.
func TestReintentaCuandoLaConexionEstaMuerta(t *testing.T) {
	var intentos int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&intentos, 1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close() // se corta sin responder, como una conexión muerta
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"correo":"ana@upb.edu.co"}}`))
	}))
	defer srv.Close()

	c := NuevoCliente(srv.URL)
	var u Usuario
	if err := c.Pedir(context.Background(), http.MethodGet, "shared_file", "/files", nil, &u); err != nil {
		t.Fatalf("debería haberse recuperado: %v", err)
	}
	if u.Correo != "ana@upb.edu.co" {
		t.Fatalf("respuesta inesperada: %+v", u)
	}
	if n := atomic.LoadInt32(&intentos); n != 2 {
		t.Fatalf("se esperaban dos intentos, hubo %d", n)
	}
}

// Un error del servicio no se reintenta: es una respuesta, no un fallo de red.
func TestNoReintentaUnErrorDelServicio(t *testing.T) {
	var intentos int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&intentos, 1)
		w.WriteHeader(422)
		w.Write([]byte(`{"error":{"codigo":"VALIDACION","mensaje":"falta la ruta"}}`))
	}))
	defer srv.Close()

	err := NuevoCliente(srv.URL).Pedir(context.Background(), http.MethodPost, "shared_file", "/files/carpeta", nil, nil)
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("se esperaba un error del bus, llegó %v", err)
	}
	if e.Estado != 422 || e.Mensaje != "falta la ruta" {
		t.Fatalf("error mal interpretado: %+v", e)
	}
	if n := atomic.LoadInt32(&intentos); n != 1 {
		t.Fatalf("no debía reintentarse, hubo %d intentos", n)
	}
}
