package bus

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Los tipos de este archivo existen por cómo llega el JSON del bus.
//
// El directorio de usuarios es SOAP: el bus traduce su XML a JSON y, en XML,
// un elemento que aparece una vez no se distingue de una lista de un solo
// elemento, ni un número de la cadena que lo representa. Los servicios REST sí
// mandan tipos, así que estos tipos aceptan las dos formas y el resto del
// programa trabaja con valores normales.

// Numero acepta 12, "12" y "" (que vale 0).
type Numero int64

func (n *Numero) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		f, err2 := strconv.ParseFloat(s, 64)
		if err2 != nil {
			return err
		}
		v = int64(f)
	}
	*n = Numero(v)
	return nil
}

func (n Numero) Int64() int64 { return int64(n) }

// Booleano acepta true y "true".
type Booleano bool

func (v *Booleano) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	*v = s == "true"
	return nil
}

func (v Booleano) Bool() bool { return bool(v) }

// Lista acepta "a", ["a","b"] y ausencia.
type Lista []string

func (l *Lista) UnmarshalJSON(b []byte) error {
	var varios []string
	if err := json.Unmarshal(b, &varios); err == nil {
		*l = varios
		return nil
	}
	var uno string
	if err := json.Unmarshal(b, &uno); err != nil {
		*l = nil
		return nil // un tipo inesperado se trata como lista vacía
	}
	if uno == "" {
		*l = nil
	} else {
		*l = []string{uno}
	}
	return nil
}

func (l Lista) Contiene(v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// ListaDe es lo mismo para listas de objetos: {…} o [{…}].
type ListaDe[T any] []T

func (l *ListaDe[T]) UnmarshalJSON(b []byte) error {
	var varios []T
	if err := json.Unmarshal(b, &varios); err == nil {
		*l = varios
		return nil
	}
	var uno T
	if err := json.Unmarshal(b, &uno); err != nil {
		*l = nil
		return nil
	}
	*l = []T{uno}
	return nil
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
