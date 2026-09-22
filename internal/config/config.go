// Package config resuelve las rutas y direcciones que la aplicación necesita
// antes de arrancar: dónde vive el bus y dónde guarda ella sus propios datos.
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// BusPorDefecto es el bus del despliegue del CCA. Se puede cambiar con la
// variable UPB_BUS_URL, que es lo que se hace al probar contra un bus local.
const BusPorDefecto = "http://10.154.12.210:8099/bus"

// SincroPorDefecto es el servidor gRPC de File Sync (no pasa por el bus: el
// bus media REST, SOAP y RMI, y gRPC se habla directo).
const SincroPorDefecto = "10.154.12.212:8091"

type Config struct {
	BusURL     string // http://host:8099/bus
	SincroAddr string // host:puerto del servidor gRPC
	DirDatos   string // estado de la aplicación (sesión, dispositivo, preferencias)
}

// Cargar devuelve la configuración efectiva y crea el directorio de datos.
func Cargar() (Config, error) {
	c := Config{
		BusURL:     valor("UPB_BUS_URL", BusPorDefecto),
		SincroAddr: valor("UPB_SINCRO_ADDR", SincroPorDefecto),
	}
	dir, err := dirDatos()
	if err != nil {
		return c, err
	}
	c.DirDatos = dir
	return c, os.MkdirAll(dir, 0o700)
}

func valor(clave, pordefecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return pordefecto
}

// dirDatos sigue la convención de cada sistema: Application Support en macOS,
// AppData en Windows y ~/.config (o XDG_CONFIG_HOME) en Linux.
func dirDatos() (string, error) {
	if d := os.Getenv("UPB_DIR_DATOS"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		inicio, err2 := os.UserHomeDir()
		if err2 != nil {
			return "", err
		}
		base = filepath.Join(inicio, ".config")
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(base, "UPB-CIENTIFICA"), nil
	}
	return filepath.Join(base, "upb-cientifica"), nil
}

// CarpetaSincroPorDefecto es la que se propone al configurar la sincronización.
func CarpetaSincroPorDefecto() string {
	inicio, err := os.UserHomeDir()
	if err != nil {
		return "UPB-CIENTIFICA"
	}
	return filepath.Join(inicio, "UPB-CIENTIFICA")
}
