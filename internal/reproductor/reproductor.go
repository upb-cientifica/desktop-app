// Package reproductor reproduce un flujo HLS dentro de la propia ventana.
//
// Fyne no trae reproductor de video, así que aquí se arma uno: ffmpeg
// descodifica el flujo que entrega el Service Bus y lo parte en dos salidas,
// los cuadros en crudo por la salida estándar y el sonido por un socket local.
// La ventana pinta los cuadros según llegan y el sonido va a la tarjeta con
// oto. ffmpeg lee con `-re`, es decir al ritmo real del video, así que imagen
// y sonido llegan acompasados sin tener que sincronizarlos a mano.
//
// El video nunca se descarga entero: HLS lo sirve en trozos y ffmpeg los va
// pidiendo al bus a medida que avanza la reproducción.
package reproductor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

const (
	frecuencia = 44100 // Hz del sonido que se le pide a ffmpeg
	canales    = 2
)

// Disponible dice si el equipo tiene ffmpeg, que es lo único que hace falta.
func Disponible() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// Medidas de un video, según ffprobe.
type Medidas struct {
	Ancho, Alto int
	Duracion    time.Duration
	ConSonido   bool
}

// Sesion es una reproducción en curso.
type Sesion struct {
	Cuadros   <-chan image.Image // un cuadro listo para pintar
	Terminado <-chan error       // se cierra al acabar; con error si algo falló

	cmd      *exec.Cmd
	cancelar context.CancelFunc
	escucha  net.Listener

	mu       sync.Mutex
	pausado  bool
	reanudar chan struct{}

	inicio time.Time
	pausas time.Duration
	desde  time.Time
}

// contexto de sonido: oto solo admite uno por proceso.
var (
	unaVez    sync.Once
	ctxSonido *oto.Context
	errSonido error
)

func contextoDeSonido() (*oto.Context, error) {
	unaVez.Do(func() {
		c, listo, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:   frecuencia,
			ChannelCount: canales,
			Format:       oto.FormatSignedInt16LE,
		})
		if err != nil {
			errSonido = err
			return
		}
		<-listo
		ctxSonido = c
	})
	return ctxSonido, errSonido
}

// Medir pregunta a ffprobe el tamaño y la duración del video.
func Medir(ctx context.Context, url string) (Medidas, error) {
	salida, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "stream=codec_type,width,height:format=duration",
		"-of", "json", url).Output()
	if err != nil {
		return Medidas{}, fmt.Errorf("no se pudo leer el video: %w", err)
	}
	var d struct {
		Streams []struct {
			Tipo   string `json:"codec_type"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(salida, &d); err != nil {
		return Medidas{}, errors.New("no se entendió la respuesta de ffprobe")
	}
	var m Medidas
	for _, st := range d.Streams {
		switch st.Tipo {
		case "video":
			if m.Ancho == 0 {
				m.Ancho, m.Alto = st.Width, st.Height
			}
		case "audio":
			m.ConSonido = true
		}
	}
	if m.Ancho == 0 {
		return Medidas{}, errors.New("el flujo no trae imagen")
	}
	if seg, err := strconv.ParseFloat(d.Format.Duration, 64); err == nil {
		m.Duracion = time.Duration(seg * float64(time.Second))
	}
	return m, nil
}

// Abrir arranca la reproducción. `anchoDestino` es el ancho al que se escala la
// imagen; la altura sale de la proporción original.
func Abrir(padre context.Context, url string, anchoDestino int, m Medidas) (*Sesion, error) {
	if !Disponible() {
		return nil, errors.New("hace falta ffmpeg en el equipo para reproducir dentro de la aplicación")
	}
	alto := altoProporcional(m, anchoDestino)

	// El sonido viaja por un socket local: así las dos salidas salen del mismo
	// proceso de ffmpeg y no hay dos descodificaciones que se desfasen. Un
	// video sin pista de sonido no lleva esa salida: ffmpeg se niega a escribir
	// una salida vacía y no arrancaría.
	var escucha net.Listener
	argumentos := []string{
		"-nostdin", "-loglevel", "error",
		"-re", // al ritmo real del video, no lo más rápido posible
		"-i", url,
		"-map", "0:v:0",
		"-f", "rawvideo", "-pix_fmt", "rgba",
		"-vf", fmt.Sprintf("scale=%d:%d", anchoDestino, alto),
		"pipe:1",
	}
	if m.ConSonido {
		var err error
		escucha, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		puerto := escucha.Addr().(*net.TCPAddr).Port
		argumentos = append(argumentos,
			"-map", "0:a:0",
			"-f", "s16le", "-ar", strconv.Itoa(frecuencia), "-ac", strconv.Itoa(canales),
			fmt.Sprintf("tcp://127.0.0.1:%d", puerto))
	}

	ctx, cancelar := context.WithCancel(padre)
	cmd := exec.CommandContext(ctx, "ffmpeg", argumentos...)
	salidaVideo, err := cmd.StdoutPipe()
	if err != nil {
		cancelar()
		cerrar(escucha)
		return nil, err
	}
	var errores strings.Builder
	cmd.Stderr = &errores

	if err := cmd.Start(); err != nil {
		cancelar()
		cerrar(escucha)
		return nil, err
	}

	cuadros := make(chan image.Image, 2)
	terminado := make(chan error, 1)
	s := &Sesion{
		Cuadros: cuadros, Terminado: terminado,
		cmd: cmd, cancelar: cancelar, escucha: escucha,
		reanudar: make(chan struct{}), inicio: time.Now(),
	}

	if m.ConSonido {
		go s.recibirSonido()
	}
	go s.leerCuadros(salidaVideo, cuadros, terminado, anchoDestino, alto, &errores)
	return s, nil
}

func altoProporcional(m Medidas, ancho int) int {
	if m.Ancho <= 0 || m.Alto <= 0 {
		return ancho * 9 / 16
	}
	alto := m.Alto * ancho / m.Ancho
	if alto%2 != 0 { // ffmpeg quiere dimensiones pares
		alto++
	}
	return alto
}

// recibirSonido acepta la conexión de ffmpeg y entrega las muestras a la
// tarjeta de sonido. Si el equipo no tiene salida de audio, o el video no trae
// pista, la reproducción sigue sin sonido.
func (s *Sesion) recibirSonido() {
	conn, err := s.escucha.Accept()
	cerrar(s.escucha)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, err := contextoDeSonido()
	if err != nil {
		io.Copy(io.Discard, conn) // hay que vaciar el socket o ffmpeg se bloquea
		return
	}
	reproductor := ctx.NewPlayer(&lectorPausable{origen: bufio.NewReaderSize(conn, 64<<10), sesion: s})
	defer reproductor.Close()
	reproductor.Play()

	<-s.Terminado
}

// lectorPausable corta el suministro de sonido mientras la reproducción está
// en pausa; al no leer, ffmpeg se detiene solo y el video espera con él.
type lectorPausable struct {
	origen io.Reader
	sesion *Sesion
}

func (l *lectorPausable) Read(p []byte) (int, error) {
	l.sesion.esperarSiPausado()
	return l.origen.Read(p)
}

func (s *Sesion) leerCuadros(origen io.Reader, cuadros chan<- image.Image, terminado chan<- error,
	ancho, alto int, errores *strings.Builder) {

	defer close(cuadros)
	lector := bufio.NewReaderSize(origen, ancho*alto*4)
	tam := ancho * alto * 4

	for {
		s.esperarSiPausado()

		buf := make([]byte, tam)
		if _, err := io.ReadFull(lector, buf); err != nil {
			esperar := s.cmd.Wait()
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				terminado <- nil // el video se acabó
			} else if esperar != nil && errores.Len() > 0 {
				terminado <- errors.New(strings.TrimSpace(errores.String()))
			} else {
				terminado <- nil
			}
			close(terminado)
			return
		}
		img := &image.RGBA{Pix: buf, Stride: ancho * 4, Rect: image.Rect(0, 0, ancho, alto)}
		select {
		case cuadros <- img:
		case <-time.After(2 * time.Second):
			// La ventana no da abasto: se descarta el cuadro y se sigue.
		}
	}
}

func (s *Sesion) esperarSiPausado() {
	s.mu.Lock()
	if !s.pausado {
		s.mu.Unlock()
		return
	}
	espera := s.reanudar
	s.mu.Unlock()
	<-espera
}

// Pausar y Reanudar detienen y siguen la reproducción. Con la pausa se deja de
// leer, ffmpeg se bloquea al escribir y el video queda donde está.
func (s *Sesion) Pausar() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pausado {
		return
	}
	s.pausado = true
	s.desde = time.Now()
}

func (s *Sesion) Reanudar() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.pausado {
		return
	}
	s.pausado = false
	s.pausas += time.Since(s.desde)
	close(s.reanudar)
	s.reanudar = make(chan struct{})
}

func (s *Sesion) EnPausa() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pausado
}

// Transcurrido es cuánto se lleva reproducido, sin contar las pausas.
func (s *Sesion) Transcurrido() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := time.Since(s.inicio) - s.pausas
	if s.pausado {
		t -= time.Since(s.desde)
	}
	if t < 0 {
		return 0
	}
	return t
}

// Cerrar termina la reproducción y suelta todo.
func (s *Sesion) Cerrar() {
	s.Reanudar() // que ninguna gorutina quede esperando la pausa
	s.cancelar()
	cerrar(s.escucha)
}

func cerrar(l net.Listener) {
	if l != nil {
		_ = l.Close()
	}
}
