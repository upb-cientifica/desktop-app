// Package bus habla con el Service Bus de UPB-CIENTÍFICA.
//
// La aplicación de escritorio no llama a ningún servicio por su cuenta: todo
// entra por el bus, que resuelve dónde vive cada servicio, autoriza con el
// claim del JWT y traduce el protocolo (SOAP para el directorio de usuarios,
// RMI para el clúster, REST para el resto). La única excepción es File Sync,
// que se habla por gRPC directo porque el bus no media ese protocolo.
//
// El bus responde siempre con la misma envoltura:
//
//	éxito  {"data": …}
//	error  {"error": {"codigo": "...", "mensaje": "..."}}
package bus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Error es la respuesta de error del bus, ya desenvuelta.
type Error struct {
	Estado  int
	Codigo  string
	Mensaje string
}

func (e *Error) Error() string { return e.Mensaje }

// EsNoAutorizado indica que el token falta, venció o no cubre ese servicio.
func EsNoAutorizado(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Estado == 401 || e.Estado == 403
	}
	return false
}

// Cliente es seguro para usarse desde varias gorutinas: la interfaz lanza
// cada llamada en la suya para no congelar la ventana.
type Cliente struct {
	base       string
	http       *http.Client
	transporte *http.Transport
	mu         sync.RWMutex
	token      string
}

func NuevoCliente(baseURL string) *Cliente {
	// El transporte se configura a mano por lo que pasó en pruebas: si el bus
	// se reinicia, las conexiones que el cliente tenía guardadas quedan
	// muertas, y al reutilizar una, la petición se queda esperando una
	// respuesta que no va a llegar. Con un plazo para la primera línea de la
	// respuesta el fallo se nota en segundos, y las conexiones ociosas se
	// sueltan pronto en vez de guardarse indefinidamente.
	transporte := http.DefaultTransport.(*http.Transport).Clone()
	transporte.IdleConnTimeout = 30 * time.Second
	transporte.ResponseHeaderTimeout = 20 * time.Second
	transporte.MaxIdleConnsPerHost = 4

	return &Cliente{
		base: strings.TrimRight(baseURL, "/"),
		// Sin plazo global: subir o bajar un archivo grande puede tardar,
		// y cada llamada trae su propio contexto.
		http:       &http.Client{Transport: transporte},
		transporte: transporte,
	}
}

func (c *Cliente) PonerToken(t string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = t
}

func (c *Cliente) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// URL arma la dirección de una operación: URL("shared_file", "/files", …).
func (c *Cliente) URL(servicio, ruta string, params map[string]string) string {
	u := c.base + "/" + servicio + ruta
	if q := consulta(params); q != "" {
		u += "?" + q
	}
	return u
}

func consulta(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	v := url.Values{}
	for k, val := range params {
		if val != "" {
			v.Set(k, val)
		}
	}
	return v.Encode()
}

// Pedir hace una llamada y deja el `data` de la respuesta en destino, que
// puede ser nil si no interesa el cuerpo.
func (c *Cliente) Pedir(ctx context.Context, metodo, servicio, ruta string, params map[string]string, destino any) error {
	return c.PedirCuerpo(ctx, metodo, servicio, ruta, params, nil, "", destino)
}

// PedirCuerpo es lo mismo, con un cuerpo que se envía tal cual (los bytes de
// un archivo al subirlo, por ejemplo).
func (c *Cliente) PedirCuerpo(ctx context.Context, metodo, servicio, ruta string, params map[string]string,
	cuerpo io.Reader, tipoContenido string, destino any) error {

	req, err := http.NewRequestWithContext(ctx, metodo, c.URL(servicio, ruta, params), cuerpo)
	if err != nil {
		return err
	}
	if t := c.Token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	if cuerpo != nil {
		if tipoContenido == "" {
			tipoContenido = "application/octet-stream"
		}
		req.Header.Set("Content-Type", tipoContenido)
	}

	res, err := c.hacer(req, cuerpo == nil)
	if err != nil {
		return fmt.Errorf("no se pudo hablar con el bus: %w", err)
	}
	defer res.Body.Close()

	crudo, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var sobre struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Codigo  string `json:"codigo"`
			Mensaje string `json:"mensaje"`
		} `json:"error"`
	}
	_ = json.Unmarshal(crudo, &sobre) // un cuerpo vacío o no-JSON no es fatal

	if res.StatusCode >= 400 {
		e := &Error{Estado: res.StatusCode, Mensaje: fmt.Sprintf("error %d en %s", res.StatusCode, ruta)}
		if sobre.Error != nil {
			e.Codigo, e.Mensaje = sobre.Error.Codigo, sobre.Error.Mensaje
		}
		return e
	}
	if destino == nil || len(sobre.Data) == 0 {
		return nil
	}
	return json.Unmarshal(sobre.Data, destino)
}

// Descargar entrega el cuerpo crudo de una operación (archivos, imágenes).
// Quien llama cierra el io.ReadCloser.
func (c *Cliente) Descargar(ctx context.Context, servicio, ruta string, params map[string]string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL(servicio, ruta, params), nil)
	if err != nil {
		return nil, err
	}
	if t := c.Token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	res, err := c.hacer(req, true)
	if err != nil {
		return nil, fmt.Errorf("no se pudo hablar con el bus: %w", err)
	}
	if res.StatusCode >= 400 {
		crudo, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		res.Body.Close()
		e := &Error{Estado: res.StatusCode, Mensaje: fmt.Sprintf("error %d al descargar", res.StatusCode)}
		var sobre struct {
			Error *struct {
				Codigo  string `json:"codigo"`
				Mensaje string `json:"mensaje"`
			} `json:"error"`
		}
		if json.Unmarshal(crudo, &sobre) == nil && sobre.Error != nil {
			e.Codigo, e.Mensaje = sobre.Error.Codigo, sobre.Error.Mensaje
		}
		return nil, e
	}
	return res.Body, nil
}

// JSON envía un cuerpo JSON (lo usan las operaciones que no pasan todo por la
// cadena de consulta).
func (c *Cliente) JSON(ctx context.Context, metodo, servicio, ruta string, params map[string]string, cuerpo, destino any) error {
	b, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	return c.PedirCuerpo(ctx, metodo, servicio, ruta, params, bytes.NewReader(b), "application/json", destino)
}

// hacer envía la petición y, si falla por red, lo intenta una segunda vez con
// una conexión nueva: el caso típico es una conexión guardada que murió porque
// el bus se reinició. Solo se reintenta cuando no hay cuerpo que reenviar.
func (c *Cliente) hacer(req *http.Request, reintentable bool) (*http.Response, error) {
	res, err := c.http.Do(req)
	if err == nil || !reintentable || req.Context().Err() != nil {
		return res, err
	}
	c.transporte.CloseIdleConnections()
	return c.http.Do(req.Clone(req.Context()))
}

// ConPlazo devuelve un contexto con el plazo corriente de las operaciones de
// consulta; las de archivo usan el suyo.
func ConPlazo(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}
