// Package sesion guarda la sesión de la aplicación y la mantiene viva.
//
// El token de acceso dura quince minutos y vive solo en memoria. Lo único que
// se escribe en disco es el token de refresco, que dura siete días y el
// servidor rota en cada uso: así, abrir la aplicación no obliga a escribir la
// contraseña otra vez, y un archivo robado deja de servir en cuanto el
// programa renueva. El archivo se escribe con permisos 600.
package sesion

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/upb-cientifica/desktop-app/internal/bus"
)

type Sesion struct {
	cli     *bus.Cliente
	archivo string

	mu       sync.RWMutex
	refresco string
	usuario  bus.Usuario
	abierta  bool

	cancelar context.CancelFunc
	// AlCerrarse se llama cuando la sesión se cae sola (refresco vencido o
	// revocado). Lo usa la interfaz para volver a la pantalla de entrada.
	AlCerrarse func()
	// AlActualizarse avisa de que los datos del usuario ya están completos.
	AlActualizarse func()
}

type guardado struct {
	RefreshToken string `json:"refreshToken"`
	Correo       string `json:"correo"`
}

func Nueva(cli *bus.Cliente, dirDatos string) *Sesion {
	return &Sesion{cli: cli, archivo: filepath.Join(dirDatos, "sesion.json")}
}

func (s *Sesion) Abierta() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.abierta
}

func (s *Sesion) Usuario() bus.Usuario {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.usuario
}

// Entrar inicia sesión con cuenta y contraseña.
func (s *Sesion) Entrar(ctx context.Context, cuenta, password string) error {
	d, err := s.cli.Login(ctx, cuenta, password)
	if err != nil {
		return err
	}
	s.instalar(d)
	return nil
}

// Reanudar intenta seguir la sesión guardada. Devuelve false si no había
// ninguna o si ya no sirve.
func (s *Sesion) Reanudar(ctx context.Context) bool {
	g, err := s.leer()
	if err != nil || g.RefreshToken == "" {
		return false
	}
	d, err := s.cli.Renovar(ctx, g.RefreshToken)
	if err != nil {
		s.borrar()
		return false
	}
	s.instalar(d)
	return true
}

// Salir revoca la sesión en el servidor y olvida lo guardado.
func (s *Sesion) Salir(ctx context.Context) {
	_ = s.cli.CerrarSesion(ctx)
	s.cerrar()
}

func (s *Sesion) instalar(d bus.Sesion) {
	usuario := d.Usuario
	// Los permisos salen del token, que es lo que el bus verifica. La
	// operación de renovar, por ejemplo, no devuelve el usuario completo, y
	// sin esto la ventana creía que la cuenta no tenía ningún servicio.
	if c, err := bus.ClaimsDe(d.AccessToken); err == nil {
		usuario.Servicios = c.Servicios
		if usuario.Rol == "" {
			usuario.Rol = c.Rol
		}
		if usuario.Correo == "" {
			usuario.Correo = c.Correo
		}
		if usuario.ID == "" {
			usuario.ID = c.Sujeto
		}
	}

	s.mu.Lock()
	s.refresco = d.RefreshToken
	s.usuario = usuario
	s.abierta = true
	if s.cancelar != nil {
		s.cancelar()
	}
	ctx, cancelar := context.WithCancel(context.Background())
	s.cancelar = cancelar
	s.mu.Unlock()

	s.cli.PonerToken(d.AccessToken)
	s.escribir(guardado{RefreshToken: d.RefreshToken, Correo: usuario.Correo})
	go s.renovarAntesDeQueExpire(ctx, d.ExpiraEn.Int64())

	// El nombre y la cuota los completa el perfil, que sí los trae siempre.
	if usuario.Nombre == "" {
		go s.completarPerfil()
	}
}

// completarPerfil rellena lo que el token no dice (nombre, cuota, grupo).
func (s *Sesion) completarPerfil() {
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	u, err := s.cli.MiPerfil(ctx)
	if err != nil || u.Correo == "" {
		return
	}
	s.mu.Lock()
	servicios, rol := s.usuario.Servicios, s.usuario.Rol
	s.usuario = u
	s.usuario.Servicios = servicios // los del token mandan
	if rol != "" {
		s.usuario.Rol = rol
	}
	s.mu.Unlock()
	if s.AlActualizarse != nil {
		s.AlActualizarse()
	}
}

// renovarAntesDeQueExpire pide un token nuevo un minuto antes del vencimiento.
func (s *Sesion) renovarAntesDeQueExpire(ctx context.Context, expiraEnSeg int64) {
	for {
		espera := time.Duration(max64(expiraEnSeg-60, 30)) * time.Second
		select {
		case <-ctx.Done():
			return
		case <-time.After(espera):
		}

		s.mu.RLock()
		refresco := s.refresco
		s.mu.RUnlock()

		pedir, cancelar := context.WithTimeout(ctx, 30*time.Second)
		d, err := s.cli.Renovar(pedir, refresco)
		cancelar()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.cerrar()
			if s.AlCerrarse != nil {
				s.AlCerrarse()
			}
			return
		}

		s.cli.PonerToken(d.AccessToken)
		s.mu.Lock()
		s.refresco = d.RefreshToken
		s.mu.Unlock()
		s.escribir(guardado{RefreshToken: d.RefreshToken, Correo: d.Usuario.Correo})
		expiraEnSeg = d.ExpiraEn.Int64()
		if expiraEnSeg == 0 {
			expiraEnSeg = 900
		}
	}
}

func (s *Sesion) cerrar() {
	s.mu.Lock()
	s.abierta = false
	s.refresco = ""
	s.usuario = bus.Usuario{}
	if s.cancelar != nil {
		s.cancelar()
		s.cancelar = nil
	}
	s.mu.Unlock()
	s.cli.PonerToken("")
	s.borrar()
}

func (s *Sesion) leer() (guardado, error) {
	var g guardado
	b, err := os.ReadFile(s.archivo)
	if err != nil {
		return g, err
	}
	return g, json.Unmarshal(b, &g)
}

func (s *Sesion) escribir(g guardado) {
	b, err := json.Marshal(g)
	if err != nil {
		return
	}
	_ = os.WriteFile(s.archivo, b, 0o600)
}

func (s *Sesion) borrar() { _ = os.Remove(s.archivo) }

// CorreoRecordado sirve para rellenar el campo de la pantalla de entrada.
func (s *Sesion) CorreoRecordado() string {
	g, err := s.leer()
	if err != nil {
		return ""
	}
	return g.Correo
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
