package reproductor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Genera un video corto con ffmpeg y comprueba que el reproductor lo mide y
// entrega cuadros del tamaño pedido. Sin ffmpeg, la prueba se salta.
func crearVideo(t *testing.T, conSonido bool) string {
	t.Helper()
	if !Disponible() {
		t.Skip("sin ffmpeg en este equipo")
	}
	ruta := filepath.Join(t.TempDir(), "prueba.mp4")
	args := []string{"-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=10:duration=2"}
	if conSonido {
		args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:duration=2")
	}
	args = append(args, "-pix_fmt", "yuv420p", "-shortest", ruta)
	if err := exec.Command("ffmpeg", args...).Run(); err != nil {
		t.Fatalf("no se pudo crear el video de prueba: %v", err)
	}
	return ruta
}

func TestMedirLeeTamanoDuracionYSonido(t *testing.T) {
	ruta := crearVideo(t, true)
	m, err := Medir(context.Background(), ruta)
	if err != nil {
		t.Fatal(err)
	}
	if m.Ancho != 320 || m.Alto != 240 {
		t.Fatalf("tamaño: %dx%d", m.Ancho, m.Alto)
	}
	if !m.ConSonido {
		t.Fatal("el video tiene pista de sonido y no se detectó")
	}
	if m.Duracion < time.Second {
		t.Fatalf("duración: %v", m.Duracion)
	}

	mudo, err := Medir(context.Background(), crearVideo(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if mudo.ConSonido {
		t.Fatal("un video sin pista de sonido se detectó como si la tuviera")
	}
}

// Un video sin sonido no debe llevar la salida de audio: ffmpeg se niega a
// escribir una salida vacía y no arrancaría.
func TestReproduceConYSinSonido(t *testing.T) {
	for _, conSonido := range []bool{true, false} {
		ruta := crearVideo(t, conSonido)
		m, err := Medir(context.Background(), ruta)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)

		s, err := Abrir(ctx, ruta, 160, m)
		if err != nil {
			cancelar()
			t.Fatalf("sonido=%v: %v", conSonido, err)
		}
		cuadros := 0
		for img := range s.Cuadros {
			if cuadros == 0 {
				b := img.Bounds()
				if b.Dx() != 160 || b.Dy() != 120 {
					t.Fatalf("cuadro de %dx%d, se esperaba 160x120", b.Dx(), b.Dy())
				}
			}
			cuadros++
		}
		if err := <-s.Terminado; err != nil {
			t.Fatalf("sonido=%v: la reproducción falló: %v", conSonido, err)
		}
		if cuadros < 10 {
			t.Fatalf("sonido=%v: solo llegaron %d cuadros de un video de 2 s a 10 fps", conSonido, cuadros)
		}
		s.Cerrar()
		cancelar()
	}
}

func TestPausaDetieneLaEntregaDeCuadros(t *testing.T) {
	ruta := crearVideo(t, false)
	m, _ := Medir(context.Background(), ruta)
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()

	s, err := Abrir(ctx, ruta, 160, m)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Cerrar()

	<-s.Cuadros // el primero, para saber que ya está en marcha
	s.Pausar()
	if !s.EnPausa() {
		t.Fatal("debería estar en pausa")
	}
	// Se vacía lo que ya estaba en el canal y luego no debe llegar nada más.
	tiempo := time.After(1500 * time.Millisecond)
	vacio := false
	for !vacio {
		select {
		case <-s.Cuadros:
		case <-time.After(400 * time.Millisecond):
			vacio = true
		case <-tiempo:
			t.Fatal("en pausa siguen llegando cuadros")
		}
	}
	antes := s.Transcurrido()
	time.Sleep(300 * time.Millisecond)
	if d := s.Transcurrido() - antes; d > 50*time.Millisecond {
		t.Fatalf("el tiempo transcurrido avanzó %v estando en pausa", d)
	}

	s.Reanudar()
	select {
	case _, ok := <-s.Cuadros:
		if !ok {
			t.Fatal("la reproducción se cerró al reanudar")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("al reanudar no volvieron los cuadros")
	}
}

func TestSinArchivoDaError(t *testing.T) {
	if !Disponible() {
		t.Skip("sin ffmpeg en este equipo")
	}
	if _, err := Medir(context.Background(), filepath.Join(os.TempDir(), "no-existe-12345.mp4")); err == nil {
		t.Fatal("se esperaba un error")
	}
}
