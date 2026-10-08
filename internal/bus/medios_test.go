package bus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestActualizarImagenConsulta(t *testing.T) {
	for _, caso := range []struct {
		nombre, descripcion string
		etiquetas           []string
		consulta            string
	}{
		{"con detalles", "Río & montaña", []string{"viaje", "paisaje"}, "viaje,paisaje"},
		{"borrar detalles", "", nil, ""},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch || r.URL.Path != "/photo_album/imagenes/foto-1" {
					t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
				}
				consulta := r.URL.Query()
				for clave, esperado := range map[string]string{"titulo": "Foto + 1", "descripcion": caso.descripcion, "etiquetas": caso.consulta} {
					if !consulta.Has(clave) || consulta.Get(clave) != esperado {
						t.Errorf("%s: recibido %v, esperado %q (incluso vacío)", clave, consulta[clave], esperado)
					}
				}
				fmt.Fprint(w, `{"data":{"id":"foto-1","titulo":"Foto + 1"}}`)
			}))
			defer servidor.Close()
			imagen, err := NuevoCliente(servidor.URL).ActualizarImagen(context.Background(), "foto-1", "Foto + 1", caso.descripcion, caso.etiquetas)
			if err != nil || imagen.ID != "foto-1" {
				t.Fatalf("respuesta: %+v, error: %v", imagen, err)
			}
		})
	}
}

func TestAlbumsDecodificaUsoCompartido(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/photo_album/albums" {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":"1","publico":true,"urlPublica":"https://ejemplo.test/a/1","compartidoCon":["ana@upb.edu.co","luis@upb.edu.co"]},{"id":"2","publico":false,"compartidoCon":"ana@upb.edu.co"},{"id":"3"}]}`)
	}))
	defer servidor.Close()
	albums, err := NuevoCliente(servidor.URL).Albums(context.Background())
	if err != nil || len(albums) != 3 {
		t.Fatalf("álbumes: %+v, error: %v", albums, err)
	}
	if !albums[0].Publico || albums[0].URLPublica != "https://ejemplo.test/a/1" || !reflect.DeepEqual(albums[0].CompartidoCon, Lista{"ana@upb.edu.co", "luis@upb.edu.co"}) {
		t.Fatalf("álbum público: %+v", albums[0])
	}
	if albums[1].Publico || !reflect.DeepEqual(albums[1].CompartidoCon, Lista{"ana@upb.edu.co"}) {
		t.Fatalf("álbum compartido: %+v", albums[1])
	}
	if albums[2].Publico || albums[2].URLPublica != "" || len(albums[2].CompartidoCon) != 0 {
		t.Fatalf("álbum privado: %+v", albums[2])
	}
}

func TestOperacionesDeFotos(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		consulta := r.URL.Query()
		switch r.URL.Path {
		case "/photo_album/buscar":
			if r.Method != http.MethodGet || consulta.Get("q") != "río azul" || consulta.Get("etiqueta") != "viaje" {
				t.Errorf("búsqueda: %s %v", r.Method, consulta)
			}
			fmt.Fprint(w, `{"data":[]}`)
			return
		case "/photo_album/albums/1/compartir":
			if r.Method != http.MethodPost || consulta.Get("correo") != "ana@upb.edu.co" || (consulta.Get("accion") != "grant" && consulta.Get("accion") != "revoke") {
				t.Errorf("compartir: %s %v", r.Method, consulta)
			}
		case "/photo_album/albums/1/publico":
			if r.Method != http.MethodPost || (consulta.Get("accion") != "activar" && consulta.Get("accion") != "desactivar") {
				t.Errorf("publicar: %s %v", r.Method, consulta)
			}
		default:
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"data":{"id":"1"}}`)
	}))
	defer servidor.Close()
	cliente := NuevoCliente(servidor.URL)
	if _, err := cliente.BuscarFotos(context.Background(), "río azul", "viaje"); err != nil {
		t.Fatal(err)
	}
	for _, activo := range []bool{true, false} {
		if _, err := cliente.CompartirAlbum(context.Background(), "1", " ana ", activo); err != nil {
			t.Fatal(err)
		}
		if _, err := cliente.PublicarAlbum(context.Background(), "1", activo); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTrabajoYSlots(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hpc/trabajos/abc-123":
			fmt.Fprint(w, `{"data":{"id":"abc-123","estado":"EJECUTANDO","progreso":42,"mensaje":"ejecutando en el clúster","rutaHome":"/HPC/k"}}`)
		case "/hpc/slots":
			fmt.Fprint(w, `{"data":{"slotsDisponibles":16}}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer servidor.Close()
	cli := NuevoCliente(servidor.URL)
	trabajo, err := cli.Trabajo(context.Background(), "abc-123")
	if err != nil || trabajo.Estado != "EJECUTANDO" || trabajo.Progreso.Int64() != 42 || trabajo.RutaHome != "/HPC/k" {
		t.Fatalf("trabajo: %+v, error: %v", trabajo, err)
	}
	slots, err := cli.SlotsDisponibles(context.Background())
	if err != nil || slots != 16 {
		t.Fatalf("slots: %d, error: %v", slots, err)
	}
}
