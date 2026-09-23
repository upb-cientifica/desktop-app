package bus

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// Claims es lo que el token de acceso afirma de quien lo presenta.
//
// Se leen del propio JWT y no de la respuesta del directorio, porque el token
// es lo que los servicios y el bus verifican de verdad: si el claim no trae
// `shared_file`, la llamada se rechaza por más que el perfil diga otra cosa.
// Aquí solo se descodifica la carga útil; la firma la comprueba el bus con la
// clave pública, y nada de lo que se lea aquí concede permisos por su cuenta.
type Claims struct {
	Correo    string `json:"correo"`
	Rol       string `json:"rol"`
	Servicios Lista  `json:"servicios"`
	Sujeto    string `json:"sub"`
}

func (c Claims) EsAdmin() bool { return c.Rol == "admin" }

// ClaimsDe descodifica la carga útil de un JWT.
func ClaimsDe(token string) (Claims, error) {
	var c Claims
	partes := strings.Split(token, ".")
	if len(partes) != 3 {
		return c, errors.New("el token no tiene la forma de un JWT")
	}
	crudo, err := base64.RawURLEncoding.DecodeString(partes[1])
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(crudo, &c)
}
