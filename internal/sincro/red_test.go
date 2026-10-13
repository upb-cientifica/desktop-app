package sincro

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func esperar(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("no llegó la pasada")
	}
}
func sinPasada(t *testing.T, ch <-chan struct{}, plazo time.Duration) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("pasada inesperada")
	case <-time.After(plazo):
	}
}

func TestVigiaRedYAntirrebote(t *testing.T) {
	dir := t.TempDir()
	var conectado atomic.Bool
	pasadas := make(chan struct{}, 20)
	estados := make(chan bool, 20)
	marcar := func(ctx context.Context, red, direccion string) (net.Conn, error) {
		if red != "tcp" || direccion != "servidor:8091" {
			t.Error("destino incorrecto")
		}
		if limite, ok := ctx.Deadline(); !ok || time.Until(limite) > 2*time.Second {
			t.Error("falta timeout de dos segundos")
		}
		if !conectado.Load() {
			return nil, errors.New("sin red")
		}
		a, b := net.Pipe()
		b.Close()
		return a, nil
	}
	v, err := nuevoVigia(dir, "servidor:8091", marcar, func() { pasadas <- struct{}{} }, func(b bool) { estados <- b }, 10*time.Millisecond, time.Hour, 80*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Detener()
	escribir := func(nombre string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, nombre), []byte("dato"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	escribir("fuera.txt")
	sinPasada(t, pasadas, 150*time.Millisecond)
	if v.EnRed() {
		t.Fatal("no debe estar en red")
	}
	conectado.Store(true)
	esperar(t, pasadas)
	if !v.EnRed() {
		t.Fatal("debe estar en red")
	}
	sinPasada(t, pasadas, 150*time.Millisecond)
	for _, nombre := range []string{archivoEstado, "a.tmp", "a~", ".DS_Store", "a.swp", ".fs-descarga"} {
		escribir(nombre)
	}
	sinPasada(t, pasadas, 150*time.Millisecond)
	if err := os.MkdirAll(filepath.Join(dir, "nueva", "hija"), 0700); err != nil {
		t.Fatal(err)
	}
	esperar(t, pasadas)
	for i := 0; i < 5; i++ {
		escribir("nueva/hija/archivo.txt")
		time.Sleep(10 * time.Millisecond)
	}
	esperar(t, pasadas)
	sinPasada(t, pasadas, 150*time.Millisecond)
	conectado.Store(false)
	select {
	case <-estados:
	case <-time.After(time.Second):
		t.Fatal("falta cambio a conectado")
	}
	select {
	case dentro := <-estados:
		if dentro {
			t.Fatal("debe salir de red")
		}
	case <-time.After(time.Second):
		t.Fatal("falta cambio a desconectado")
	}
	escribir("fuera2.txt")
	sinPasada(t, pasadas, 150*time.Millisecond)
	conectado.Store(true)
	esperar(t, pasadas)
}

func TestVigiaEncolaSoloUnaYSeDetiene(t *testing.T) {
	dir := t.TempDir()
	pasadas := make(chan struct{}, 10)
	liberar := make(chan struct{})
	var n atomic.Int32
	marcar := func(context.Context, string, string) (net.Conn, error) { return nil, nil }
	v, err := nuevoVigia(dir, "servidor", marcar, func() {
		pasadas <- struct{}{}
		if n.Add(1) == 1 {
			<-liberar
		}
	}, nil, time.Hour, time.Hour, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Detener()
	esperar(t, pasadas)
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(dir, "archivo"), []byte{byte(i)}, 0600); err != nil {
			t.Fatal(err)
		}
		sinPasada(t, pasadas, 70*time.Millisecond)
	}
	close(liberar)
	esperar(t, pasadas)
	sinPasada(t, pasadas, 100*time.Millisecond)
	v.Detener()
	if err := os.WriteFile(filepath.Join(dir, "otro"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	sinPasada(t, pasadas, 100*time.Millisecond)
}

func TestVigiaTraeCambiosRemotos(t *testing.T) {
	pasadas := make(chan struct{}, 10)
	v, err := nuevoVigia(t.TempDir(), "servidor", func(context.Context, string, string) (net.Conn, error) { return nil, nil }, func() { pasadas <- struct{}{} }, nil, time.Hour, 30*time.Millisecond, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Detener()
	esperar(t, pasadas)
	esperar(t, pasadas)
}
