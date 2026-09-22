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
make build && ./bin/upb-escritorio
```

Fyne dibuja con OpenGL a través de cgo, así que **cada sistema se compila en su
propio sistema**: el ejecutable de Windows se hace en Windows y el de Linux en
Linux. No hay contenedores de por medio.

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
  sincro/    cliente gRPC de File Sync, motor de sincronización y horarios
  ui/        ventana: entrada, marco con menú y vistas
proto/       copia del contrato de File Sync (make proto regenera gen/)
gen/         stubs de gRPC generados
```

## Sincronización

Una carpeta del equipo se mantiene igual que en el servidor. El equipo se
registra una vez (queda un `.filesync-state.json` dentro de la carpeta, el
mismo que usa el cliente de consola de `file-sync`, así que las dos
herramientas pueden turnarse sobre el mismo directorio).

Cuándo se sincroniza, según el §3.3 del enunciado:

- **Solo a mano**, con el botón.
- **Cada tantos minutos.**
- **Todos los días a una hora**, que es el "horario establecido" que pide el
  enunciado. Corre aunque la sección esté cerrada, mientras la aplicación siga
  abierta y la sesión viva.

Los conflictos (dos equipos tocaron el mismo archivo) se resuelven eligiendo la
versión del servidor, la del equipo, o conservando las dos: la copia local se
guarda aparte con su marca.

## Estado

| Sección | Estado |
|---|---|
| Entrada y sesión | ✔ |
| Mi unidad (listar, subir, descargar, carpetas, renombrar, destacar, papelera) | ✔ |
| Compartido conmigo · compartir con otras cuentas | ✔ |
| Versiones de un archivo | ✔ (lectura) |
| Sincronización: carpeta, registro del equipo, pasadas manuales y por horario, conflictos | ✔ |
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
