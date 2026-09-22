package sincro

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	pb "github.com/upb-cientifica/desktop-app/gen/filesync/v1"
)

// Este motor es el mismo algoritmo del cliente de línea de comandos de
// file-sync, sin los mensajes por consola: en vez de imprimir, avisa por un
// canal de eventos que la ventana pinta.
//
// Concurrencia optimista: cada archivo lleva una versión en el servidor y el
// cliente recuerda la última que sincronizó. Si al subir la versión base ya
// quedó vieja y además el contenido difiere, el servidor declara CONFLICTO y
// nadie pisa a nadie.

const archivoEstado = ".filesync-state.json"

type entradaEstado struct {
	Version int64  `json:"version"`
	Hash    string `json:"hash"`
	Size    int64  `json:"size"`
	MTime   int64  `json:"mtime"`
}

// Estado local de una carpeta sincronizada. Vive dentro de la propia carpeta,
// igual que el del cliente de consola, para que las dos herramientas puedan
// turnarse sobre el mismo directorio.
type EstadoLocal struct {
	Endpoint      string                   `json:"endpoint"`
	DispositivoID string                   `json:"dispositivoId"`
	Entries       map[string]entradaEstado `json:"entries"`
	ruta          string
}

func CargarEstado(dir string) *EstadoLocal {
	p := filepath.Join(dir, archivoEstado)
	st := &EstadoLocal{Entries: map[string]entradaEstado{}, ruta: p}
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, st)
		if st.Entries == nil {
			st.Entries = map[string]entradaEstado{}
		}
		st.ruta = p
	}
	return st
}

func (st *EstadoLocal) Guardar() error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := st.ruta + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.ruta)
}

func (st *EstadoLocal) Registrado() bool { return st.DispositivoID != "" }

// Olvidar hace que la siguiente pasada vuelva a mirar esa ruta como si no la
// conociera; se usa tras resolver un conflicto.
func (st *EstadoLocal) Olvidar(rel string) { delete(st.Entries, rel) }

// Resumen es lo que dejó una pasada de sincronización.
type Resumen struct {
	Subidos     int
	Descargados int
	Borrados    int
	SinCambios  int
	Conflictos  int
	Fallos      []string
}

func (r Resumen) String() string {
	return fmt.Sprintf("%d subido(s), %d descargado(s), %d borrado(s), %d sin cambios, %d conflicto(s)",
		r.Subidos, r.Descargados, r.Borrados, r.SinCambios, r.Conflictos)
}

// Opciones de una pasada.
type Opciones struct {
	Subir      bool // false = solo traer del servidor
	Borrar     bool // propagar los borrados locales
	Comentario string
	// Aviso recibe una línea por archivo tratado, para la bitácora de la
	// ventana. Puede ser nil.
	Aviso func(string)
}

func (o Opciones) avisar(formato string, args ...any) {
	if o.Aviso != nil {
		o.Aviso(fmt.Sprintf(formato, args...))
	}
}

// Sincronizar hace una pasada completa sobre la carpeta.
func (c *Cliente) Sincronizar(ctx context.Context, st *EstadoLocal, dir string, op Opciones) Resumen {
	var r Resumen
	locales := escanear(dir)

	if op.Subir {
		for rel, li := range locales {
			previo, conocido := st.Entries[rel]
			if conocido && previo.Hash == li.hash {
				continue // sin cambios locales
			}
			base := int64(0)
			if conocido {
				base = previo.Version
			}
			res, entrada, idConflicto, err := c.subir(ctx,
				filepath.Join(dir, filepath.FromSlash(rel)), rel, base, li, st.DispositivoID, op.Comentario)
			if err != nil {
				r.Fallos = append(r.Fallos, rel+": "+err.Error())
				op.avisar("✗ %s: %v", rel, err)
				continue
			}
			switch res {
			case pb.Resultado_APLICADO:
				r.Subidos++
				st.Entries[rel] = entradaEstado{Version: entrada.GetVersion(), Hash: li.hash, Size: li.size, MTime: li.mtime}
				op.avisar("↑ %s (v%d)", rel, entrada.GetVersion())
			case pb.Resultado_SIN_CAMBIOS:
				r.SinCambios++
				st.Entries[rel] = entradaEstado{Version: entrada.GetVersion(), Hash: li.hash, Size: li.size, MTime: li.mtime}
			case pb.Resultado_CONFLICTO:
				r.Conflictos++
				op.avisar("⚠ conflicto #%d en %s (servidor v%d)", idConflicto, rel, entrada.GetVersion())
			}
		}

		if op.Borrar {
			for rel, previo := range st.Entries {
				if _, existe := locales[rel]; existe || previo.Hash == "" {
					continue
				}
				res, err := c.rpc.Delete(ctx, &pb.DeleteRequest{
					Ruta: rel, BaseVersion: previo.Version, Dispositivo: st.DispositivoID,
				})
				if err != nil {
					r.Fallos = append(r.Fallos, "borrar "+rel+": "+err.Error())
					continue
				}
				switch res.GetResultado() {
				case pb.Resultado_APLICADO, pb.Resultado_SIN_CAMBIOS:
					r.Borrados++
					delete(st.Entries, rel)
					op.avisar("␡ %s (borrado en el servidor)", rel)
				case pb.Resultado_CONFLICTO:
					r.Conflictos++
					op.avisar("⚠ conflicto #%d al borrar %s", res.GetConflictoId(), rel)
				}
			}
		}
	}

	// Lo que el servidor tenga más nuevo baja al equipo.
	lista, err := c.rpc.ListEntries(ctx, &pb.ListEntriesRequest{})
	if err != nil {
		r.Fallos = append(r.Fallos, "listar: "+err.Error())
		return r
	}
	for _, e := range lista.GetEntradas() {
		rel := e.GetRuta()
		previo, conocido := st.Entries[rel]
		li, hayLocal := locales[rel]

		if e.GetBorrado() {
			// Solo se borra lo que no tenga cambios locales sin subir.
			if hayLocal && (!conocido || li.hash == previo.Hash) {
				_ = os.Remove(filepath.Join(dir, filepath.FromSlash(rel)))
				delete(st.Entries, rel)
				r.Borrados++
				op.avisar("␡ %s (borrado en el equipo)", rel)
			} else if conocido {
				delete(st.Entries, rel)
			}
			continue
		}
		if conocido && previo.Version >= e.GetVersion() {
			continue
		}
		if hayLocal && conocido && li.hash != previo.Hash && li.hash != e.GetContentHash() {
			continue // hay cambio local sin subir: no se pisa
		}
		if hayLocal && li.hash == e.GetContentHash() {
			st.Entries[rel] = entradaEstado{Version: e.GetVersion(), Hash: e.GetContentHash(), Size: e.GetTamanoBytes(), MTime: li.mtime}
			continue
		}
		if err := c.descargar(ctx, dir, rel, e); err != nil {
			r.Fallos = append(r.Fallos, "descargar "+rel+": "+err.Error())
			op.avisar("✗ %s: %v", rel, err)
			continue
		}
		st.Entries[rel] = entradaEstado{Version: e.GetVersion(), Hash: e.GetContentHash(), Size: e.GetTamanoBytes(), MTime: time.Now().Unix()}
		r.Descargados++
		op.avisar("↓ %s (v%d)", rel, e.GetVersion())
	}
	return r
}

func (c *Cliente) subir(ctx context.Context, rutaAbs, rel string, base int64, li infoLocal,
	dispositivo, comentario string) (pb.Resultado, *pb.Entry, int64, error) {

	f, err := os.Open(rutaAbs)
	if err != nil {
		return 0, nil, 0, err
	}
	defer f.Close()

	flujo, err := c.rpc.Upload(ctx)
	if err != nil {
		return 0, nil, 0, err
	}
	if err := flujo.Send(&pb.UploadRequest{Payload: &pb.UploadRequest_Meta_{Meta: &pb.UploadRequest_Meta{
		Ruta: rel, BaseVersion: base, ContentHash: li.hash, TamanoBytes: li.size,
		Modo: li.modo, Dispositivo: dispositivo, Comentario: comentario,
	}}}); err != nil {
		return 0, nil, 0, err
	}
	buf := make([]byte, 64<<10)
	for {
		n, errLeer := f.Read(buf)
		if n > 0 {
			if err := flujo.Send(&pb.UploadRequest{Payload: &pb.UploadRequest_Datos{Datos: buf[:n]}}); err != nil {
				return 0, nil, 0, err
			}
		}
		if errLeer == io.EOF {
			break
		}
		if errLeer != nil {
			return 0, nil, 0, errLeer
		}
	}
	resp, err := flujo.CloseAndRecv()
	if err != nil {
		return 0, nil, 0, err
	}
	return resp.GetResultado(), resp.GetEntrada(), resp.GetConflictoId(), nil
}

func (c *Cliente) descargar(ctx context.Context, dir, rel string, e *pb.Entry) error {
	flujo, err := c.rpc.Download(ctx, &pb.DownloadRequest{Ruta: rel})
	if err != nil {
		return err
	}
	destino := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(destino), 0o755); err != nil {
		return err
	}
	// Se escribe a un temporal y se renombra: si la descarga se corta, el
	// archivo bueno del equipo no queda a medias.
	tmp, err := os.CreateTemp(filepath.Dir(destino), ".fs-*")
	if err != nil {
		return err
	}
	nombreTmp := tmp.Name()
	defer os.Remove(nombreTmp)

	h := sha256.New()
	for {
		trozo, errRecibir := flujo.Recv()
		if errRecibir == io.EOF {
			break
		}
		if errRecibir != nil {
			tmp.Close()
			return errRecibir
		}
		if d := trozo.GetDatos(); d != nil {
			if _, err := tmp.Write(d); err != nil {
				tmp.Close()
				return err
			}
			h.Write(d)
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if llego := hex.EncodeToString(h.Sum(nil)); e.GetContentHash() != "" && llego != e.GetContentHash() {
		return fmt.Errorf("el contenido llegó dañado (hash distinto)")
	}
	modo := os.FileMode(e.GetModo())
	if modo == 0 {
		modo = 0o644
	}
	_ = os.Chmod(nombreTmp, modo.Perm())
	return os.Rename(nombreTmp, destino)
}

// ---------- lectura de la carpeta local ----------

type infoLocal struct {
	hash  string
	size  int64
	mtime int64
	modo  uint32
}

func escanear(dir string) map[string]infoLocal {
	out := map[string]infoLocal{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".filesync" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == archivoEstado || d.Name() == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		hash, tam, err := hashDe(p)
		if err != nil {
			return nil
		}
		out[filepath.ToSlash(rel)] = infoLocal{
			hash: hash, size: tam, mtime: info.ModTime().Unix(), modo: uint32(info.Mode().Perm()),
		}
		return nil
	})
	return out
}

func hashDe(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
