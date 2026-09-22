# Aplicación de escritorio — UPB-CIENTÍFICA

Cliente de escritorio del sistema: **un solo ejecutable**, escrito en Go con
[Fyne](https://fyne.io) (BSD-3). No necesita instalador, ni Node, ni
contenedores, ni servicios externos: corre en macOS, Windows y Linux.

Cubre el requisito §3.3 del enunciado —copia sincronizada de un directorio
local, programable por horario— y además da acceso al resto de los servicios
desde el escritorio.

## Cómo habla con el sistema

Todo el tránsito entra por el **Service Bus**, que resuelve dónde vive cada
servicio, autoriza con el claim `servicios` del JWT y traduce el protocolo:
SOAP para el directorio de usuarios, Java RMI para el clúster, REST para el
resto. La única excepción es **File Sync**, que se habla por **gRPC directo**,
porque el bus no media ese protocolo.

```
  ventana (Fyne)
        │
  internal/ui ──► internal/bus ──► Service Bus ──► usuarios (SOAP)
        │                                     └──► shared_file, photo_album,
        │                                          streaming, monitoreo (REST)
        │                                     └──► hpc (Java RMI)
        └──────► internal/sincro ────────────────► File Sync (gRPC)
```

## Puesta en marcha

Requisitos: **Go ≥ 1.24**. En macOS bastan las Command Line Tools de Xcode; en
Linux hacen falta las cabeceras de OpenGL y X11 que pide Fyne.

```bash
go build -o bin/upb-escritorio ./cmd/upb-escritorio
./bin/upb-escritorio
```

Por defecto apunta al despliegue del CCA. Para probar contra otro entorno:

| Variable | Para qué | Valor por defecto |
|---|---|---|
| `UPB_BUS_URL` | dirección del bus | `http://10.154.12.210:8099/bus` |
| `UPB_SINCRO_ADDR` | servidor gRPC de File Sync | `10.154.12.212:8091` |
| `UPB_DIR_DATOS` | dónde guarda su estado | carpeta de configuración del sistema |

## Sesión

El token de acceso dura quince minutos y vive **solo en memoria**; se renueva
solo mientras la aplicación está abierta. Lo único que se escribe en disco es
el token de refresco (`sesion.json`, permisos 600), que dura siete días y el
servidor **rota en cada uso**: así, volver a abrir la aplicación no obliga a
escribir la contraseña, y un archivo copiado deja de servir en cuanto el
programa renueva. Cerrar sesión lo borra y revoca la sesión en el servidor.

## Estructura

```
cmd/upb-escritorio/    arranque
internal/
  config/    direcciones y carpeta de datos
  bus/       cliente del Service Bus (usuarios, archivos, …) y tipos del JSON
  sesion/    sesión, refresco silencioso y persistencia del refresco
  ui/        ventana: entrada, marco con menú y vistas
```

## Estado

| Sección | Estado |
|---|---|
| Entrada y sesión | ✔ |
| Mi unidad (listar, subir, descargar, carpetas, renombrar, destacar, papelera) | ✔ |
| Compartido conmigo · compartir con otras cuentas | ✔ |
| Versiones de un archivo | ✔ (lectura) |
| Sincronización (gRPC) | pendiente |
| Fotos · Videos · Trabajos MPI · Monitoreo · Administración | pendiente |

Las secciones que la cuenta no tenga en su claim `servicios` aparecen con el
aviso correspondiente en vez de fallar al abrirlas.

## Pruebas

```bash
go test ./...
```

Cubren la lectura del JSON que devuelve el bus (el XML traducido del directorio
manda un valor suelto donde el resto manda una lista, y números entre comillas)
y las utilidades de rutas del Home.
