// Aplicación de escritorio de UPB-CIENTÍFICA.
//
// Todo el tránsito entra por el Service Bus, salvo File Sync, que se habla por
// gRPC directo porque el bus no media ese protocolo. La aplicación es un solo
// ejecutable: no necesita instalador, ni contenedores, ni nada externo.
package main

import (
	"log"

	"github.com/upb-cientifica/desktop-app/internal/bus"
	"github.com/upb-cientifica/desktop-app/internal/config"
	"github.com/upb-cientifica/desktop-app/internal/sesion"
	"github.com/upb-cientifica/desktop-app/internal/ui"
)

func main() {
	cfg, err := config.Cargar()
	if err != nil {
		log.Fatalf("no se pudo preparar la carpeta de datos: %v", err)
	}
	cli := bus.NuevoCliente(cfg.BusURL)
	ses := sesion.Nueva(cli, cfg.DirDatos)
	ui.Nueva(cfg, cli, ses).Ejecutar()
}
