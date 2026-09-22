// Package sincro habla con el servidor File Sync por gRPC.
//
// Es el único servicio que no pasa por el Service Bus: el bus media REST, SOAP
// y RMI, y gRPC se habla directo, que es justamente lo que la Figura 1 pone en
// el recuadro "gRPC Server Go (Sync)".
//
// La identidad viaja en el metadato `authorization: Bearer <JWT>`. El token de
// acceso se renueva cada quince minutos, así que no se fija al conectar: se
// pide en cada llamada a la función que entrega el vigente.
package sincro

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "github.com/upb-cientifica/desktop-app/gen/filesync/v1"
)

type Cliente struct {
	rpc  pb.FileSyncClient
	conn *grpc.ClientConn
}

// Conectar abre la conexión gRPC. `token` se consulta en cada llamada.
func Conectar(addr string, token func() string) (*Cliente, error) {
	inyectar := func(ctx context.Context) context.Context {
		if t := token(); t != "" {
			return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+t)
		}
		return ctx
	}
	unario := func(ctx context.Context, metodo string, req, resp any, cc *grpc.ClientConn,
		invocar grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invocar(inyectar(ctx), metodo, req, resp, cc, opts...)
	}
	flujo := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, metodo string,
		abrir grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return abrir(inyectar(ctx), desc, cc, metodo, opts...)
	}

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(unario),
		grpc.WithStreamInterceptor(flujo),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16<<20)),
	)
	if err != nil {
		return nil, err
	}
	return &Cliente{rpc: pb.NewFileSyncClient(conn), conn: conn}, nil
}

func (c *Cliente) Cerrar() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

// RegistrarDispositivo da de alta este equipo y devuelve su identificador, que
// queda escrito en el estado local de la carpeta.
func (c *Cliente) RegistrarDispositivo(ctx context.Context, nombre, plataforma, carpeta string) (string, error) {
	r, err := c.rpc.RegistrarDispositivo(ctx, &pb.RegistrarDispositivoRequest{
		Nombre: nombre, Plataforma: plataforma, CarpetaLocal: carpeta,
	})
	if err != nil {
		return "", err
	}
	return r.GetDispositivoId(), nil
}

// Estado resume lo que el servidor guarda de este usuario.
type Estado struct {
	Archivos             int64
	Bytes                int64
	ConflictosPendientes int64
	UltimaSync           string
	Recientes            []Reciente
}

type Reciente struct {
	Ruta    string
	Version int64
}

func (c *Cliente) Estado(ctx context.Context) (Estado, error) {
	e, err := c.rpc.Estado(ctx, &pb.EstadoRequest{})
	if err != nil {
		return Estado{}, err
	}
	est := Estado{
		Archivos:             e.GetArchivos(),
		Bytes:                e.GetBytes(),
		ConflictosPendientes: e.GetConflictosPendientes(),
		UltimaSync:           e.GetUltimaSync(),
	}
	for _, r := range e.GetRecientes() {
		est.Recientes = append(est.Recientes, Reciente{Ruta: r.GetRuta(), Version: r.GetVersion()})
	}
	return est, nil
}

// Conflicto es una edición concurrente que el servidor no pudo decidir.
type Conflicto struct {
	ID              int64
	Ruta            string
	VersionServidor int64
	Resuelto        bool
	Estrategia      string
	DetectadoEn     string
}

func (c *Cliente) Conflictos(ctx context.Context, incluirResueltos bool) ([]Conflicto, error) {
	r, err := c.rpc.ListarConflictos(ctx, &pb.ListarConflictosRequest{IncluirResueltos: incluirResueltos})
	if err != nil {
		return nil, err
	}
	var out []Conflicto
	for _, x := range r.GetConflictos() {
		out = append(out, Conflicto{
			ID: x.GetId(), Ruta: x.GetRuta(), VersionServidor: x.GetVersionServidor(),
			Resuelto: x.GetResuelto(), Estrategia: x.GetEstrategia(), DetectadoEn: x.GetDetectadoEn(),
		})
	}
	return out, nil
}

// Estrategia de resolución de un conflicto.
type Estrategia string

const (
	MantenerServidor Estrategia = "server"
	MantenerCliente  Estrategia = "client"
	MantenerAmbos    Estrategia = "both"
)

var estrategias = map[Estrategia]pb.Estrategia{
	MantenerServidor: pb.Estrategia_MANTENER_SERVIDOR,
	MantenerCliente:  pb.Estrategia_MANTENER_CLIENTE,
	MantenerAmbos:    pb.Estrategia_MANTENER_AMBOS,
}

// Resolver aplica la estrategia y devuelve la ruta y la copia que resultó.
func (c *Cliente) Resolver(ctx context.Context, id int64, e Estrategia) (ruta, copia string, err error) {
	r, err := c.rpc.ResolverConflicto(ctx, &pb.ResolverConflictoRequest{
		ConflictoId: id, Estrategia: estrategias[e],
	})
	if err != nil {
		return "", "", err
	}
	return r.GetEntrada().GetRuta(), r.GetRutaCopia(), nil
}
