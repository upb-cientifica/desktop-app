package bus

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// El directorio de usuarios es SOAP y el bus traduce su XML: un elemento que
// aparece una vez llega como valor suelto y no como lista de uno, y los
// números pueden llegar entrecomillados. Estas pruebas fijan que las dos
// formas se lean igual, que es donde la app móvil ya se había tropezado.
func TestListaAceptaUnoOVarios(t *testing.T) {
	casos := map[string][]string{
		`"shared_file"`:               {"shared_file"},
		`["shared_file","streaming"]`: {"shared_file", "streaming"},
		`""`:                          nil,
		`null`:                        nil,
	}
	for entrada, esperado := range casos {
		var l Lista
		if err := json.Unmarshal([]byte(entrada), &l); err != nil {
			t.Fatalf("%s: %v", entrada, err)
		}
		if len(l) != len(esperado) {
			t.Fatalf("%s: se esperaban %d elementos, llegaron %d", entrada, len(esperado), len(l))
		}
		for i := range esperado {
			if l[i] != esperado[i] {
				t.Fatalf("%s: elemento %d es %q", entrada, i, l[i])
			}
		}
	}
}

func TestNumeroAceptaCadenaYNumero(t *testing.T) {
	casos := map[string]int64{
		`1073741824`:   1073741824,
		`"1073741824"`: 1073741824,
		`""`:           0,
		`null`:         0,
		`900.0`:        900,
	}
	for entrada, esperado := range casos {
		var n Numero
		if err := json.Unmarshal([]byte(entrada), &n); err != nil {
			t.Fatalf("%s: %v", entrada, err)
		}
		if n.Int64() != esperado {
			t.Fatalf("%s: se esperaba %d, llegó %d", entrada, esperado, n.Int64())
		}
	}
}

func TestListaDeAceptaObjetoSuelto(t *testing.T) {
	var l ListaDe[Catalogo]
	if err := json.Unmarshal([]byte(`{"codigo":"admin","nombre":"Administrador"}`), &l); err != nil {
		t.Fatal(err)
	}
	if len(l) != 1 || l[0].Codigo != "admin" {
		t.Fatalf("no se leyó el objeto suelto: %+v", l)
	}
}

func TestACorreoCompletaElDominio(t *testing.T) {
	if got := ACorreo("ana.torres"); got != "ana.torres@"+DominioCorreo {
		t.Fatalf("se esperaba el dominio institucional, llegó %q", got)
	}
	if got := ACorreo("ana@otra.edu"); got != "ana@otra.edu" {
		t.Fatalf("un correo completo no se debe tocar, llegó %q", got)
	}
}

func TestRutasDelHome(t *testing.T) {
	if got := Unir("/", "datos"); got != "/datos" {
		t.Fatalf("Unir en la raíz dio %q", got)
	}
	if got := Unir("/tesis", "datos"); got != "/tesis/datos" {
		t.Fatalf("Unir dio %q", got)
	}
	if got := Padre("/tesis/datos"); got != "/tesis" {
		t.Fatalf("Padre dio %q", got)
	}
	if got := Padre("/tesis"); got != "/" {
		t.Fatalf("Padre de un hijo de la raíz dio %q", got)
	}
}

func TestLegible(t *testing.T) {
	casos := map[int64]string{
		0:          "0 B",
		512:        "512 B",
		1024:       "1.0 KB",
		1536:       "1.5 KB",
		1073741824: "1.0 GB",
	}
	for bytes, esperado := range casos {
		if got := Legible(bytes); got != esperado {
			t.Fatalf("%d bytes: se esperaba %q, llegó %q", bytes, esperado, got)
		}
	}
}

// El claim `servicios` del token es lo que el bus verifica; la respuesta de
// renovar no trae el usuario completo, así que los permisos se leen de aquí.
func TestClaimsDelToken(t *testing.T) {
	// Carga útil de ejemplo, sin firma válida: aquí solo se descodifica.
	carga := base64.RawURLEncoding.EncodeToString([]byte(
		`{"correo":"ana@upb.edu.co","rol":"investigador","servicios":["shared_file","file_sync"],"sub":"7"}`))
	c, err := ClaimsDe("encabezado." + carga + ".firma")
	if err != nil {
		t.Fatal(err)
	}
	if c.Correo != "ana@upb.edu.co" || c.Sujeto != "7" || c.EsAdmin() {
		t.Fatalf("claims mal leídos: %+v", c)
	}
	if !c.Servicios.Contiene("shared_file") || c.Servicios.Contiene("streaming") {
		t.Fatalf("servicios mal leídos: %v", c.Servicios)
	}
}

func TestClaimsDeUnTokenQueNoLoEs(t *testing.T) {
	if _, err := ClaimsDe("esto-no-es-un-jwt"); err == nil {
		t.Fatal("se esperaba un error")
	}
}
