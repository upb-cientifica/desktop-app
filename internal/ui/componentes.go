package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Piezas de interfaz que Fyne no trae y que la web sí usa: el botón «Nuevo»,
// las entradas del menú lateral con fondo de píldora, los chips de filtro, el
// avatar con iniciales y la barra fina del almacenamiento.
//
// Todas leen sus colores del tema al refrescarse, así que cambian solas
// cuando se pasa de claro a oscuro: Fyne refresca cada widget al cambiar el
// tema, pero no los rectángulos sueltos.

// ---------- pastilla: botón con fondo redondeado ----------

type pastilla struct {
	widget.BaseWidget

	texto string
	icono fyne.Resource

	alto, radio float32
	izq, der    float32
	tam         float32 // tamaño del texto; 0 es el del tema

	fondo, fondoEncima fyne.ThemeColorName // "" es transparente
	colorTexto         fyne.ThemeColorName // "" es el color de texto normal
	colorIcono         fyne.ThemeColorName // "" es el gris secundario
	borde, negrita     bool

	activo, encima bool
	alTocar        func(*fyne.PointEvent)
}

func nuevaPastilla(texto string, icono fyne.Resource, alTocar func(*fyne.PointEvent)) *pastilla {
	p := &pastilla{texto: texto, icono: icono, alTocar: alTocar,
		alto: 40, radio: 20, izq: 16, der: 16, fondoEncima: theme.ColorNameHover}
	p.ExtendBaseWidget(p)
	return p
}

// elementoMenu es una entrada del menú lateral, como .nav-item en la web.
func elementoMenu(texto string, icono fyne.Resource, alTocar func()) *pastilla {
	return nuevaPastilla(texto, icono, func(*fyne.PointEvent) { alTocar() })
}

// chip es un filtro rápido, como .filter-chip en la web.
func chip(texto string, alTocar func()) *pastilla {
	p := nuevaPastilla(texto, nil, func(*fyne.PointEvent) { alTocar() })
	p.alto, p.radio, p.izq, p.der, p.borde = 32, 8, 12, 12, true
	return p
}

func (p *pastilla) SetActivo(activo bool) {
	p.activo = activo
	p.Refresh()
}

func (p *pastilla) Tapped(ev *fyne.PointEvent) {
	if p.alTocar != nil {
		p.alTocar(ev)
	}
}
func (p *pastilla) MouseIn(*desktop.MouseEvent)    { p.encima = true; p.Refresh() }
func (p *pastilla) MouseMoved(*desktop.MouseEvent) {}
func (p *pastilla) MouseOut()                      { p.encima = false; p.Refresh() }
func (p *pastilla) Cursor() desktop.Cursor         { return desktop.PointerCursor }

func (p *pastilla) CreateRenderer() fyne.WidgetRenderer {
	r := &rendererPastilla{p: p,
		fondo: canvas.NewRectangle(color.Transparent),
		texto: canvas.NewText(p.texto, nil),
		icono: widget.NewIcon(nil)}
	r.Refresh()
	return r
}

type rendererPastilla struct {
	p     *pastilla
	fondo *canvas.Rectangle
	texto *canvas.Text
	icono *widget.Icon
}

const ladoIcono = 20

func (r *rendererPastilla) estiloTexto() fyne.TextStyle {
	return fyne.TextStyle{Bold: r.p.negrita || r.p.activo}
}

func (r *rendererPastilla) tamTexto() float32 {
	if r.p.tam > 0 {
		return r.p.tam
	}
	return theme.SizeForWidget(theme.SizeNameText, r.p)
}

func (r *rendererPastilla) MinSize() fyne.Size {
	t := fyne.MeasureText(r.p.texto, r.tamTexto(), r.estiloTexto())
	w := r.p.izq + t.Width + r.p.der
	if r.p.icono != nil {
		w += ladoIcono + 12
	}
	return fyne.NewSize(w, fyne.Max(r.p.alto, t.Height))
}

func (r *rendererPastilla) Layout(s fyne.Size) {
	r.fondo.Resize(s)
	x := r.p.izq
	if r.p.icono != nil {
		r.icono.Resize(fyne.NewSquareSize(ladoIcono))
		r.icono.Move(fyne.NewPos(x, (s.Height-ladoIcono)/2))
		x += ladoIcono + 12
	}
	t := r.texto.MinSize()
	r.texto.Move(fyne.NewPos(x, (s.Height-t.Height)/2))
	r.texto.Resize(fyne.NewSize(s.Width-x-r.p.der, t.Height))
}

func (r *rendererPastilla) Refresh() {
	p := r.p
	color := func(n fyne.ThemeColorName) color.Color { return theme.ColorForWidget(n, p) }

	r.fondo.CornerRadius = p.radio
	switch {
	case p.activo:
		r.fondo.FillColor = color(theme.ColorNameSelection)
	case p.encima && p.fondoEncima != "":
		r.fondo.FillColor = color(p.fondoEncima)
	case p.fondo != "":
		r.fondo.FillColor = color(p.fondo)
	default:
		r.fondo.FillColor = colorTransparente
	}
	r.fondo.StrokeWidth = 0
	if p.borde && !p.activo {
		r.fondo.StrokeWidth, r.fondo.StrokeColor = 1, color(theme.ColorNameSeparator)
	}

	nombreTexto := p.colorTexto
	if nombreTexto == "" {
		nombreTexto = theme.ColorNameForeground
	}
	if p.activo {
		nombreTexto = theme.ColorNamePrimary
	}
	r.texto.Text = p.texto
	r.texto.Color = color(nombreTexto)
	r.texto.TextSize = r.tamTexto()
	r.texto.TextStyle = r.estiloTexto()

	if p.icono != nil {
		nombreIcono := p.colorIcono
		if nombreIcono == "" {
			nombreIcono = colorTexto2
		}
		if p.activo {
			nombreIcono = theme.ColorNamePrimary
		}
		r.icono.SetResource(theme.NewColoredResource(p.icono, nombreIcono))
		r.icono.Show()
	} else {
		r.icono.Hide()
	}

	r.Layout(p.Size())
	r.fondo.Refresh()
	r.texto.Refresh()
}

func (r *rendererPastilla) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.icono, r.texto}
}
func (r *rendererPastilla) Destroy() {}

var colorTransparente = color.Transparent

// ---------- avatar con las iniciales de la cuenta ----------

type avatar struct {
	widget.BaseWidget
	iniciales string
	alTocar   func()
}

func nuevoAvatar(nombre string, alTocar func()) *avatar {
	a := &avatar{iniciales: iniciales(nombre), alTocar: alTocar}
	a.ExtendBaseWidget(a)
	return a
}

func iniciales(nombre string) string {
	t := ""
	for _, parte := range strings.Fields(nombre) {
		t += strings.ToUpper(string([]rune(parte)[0]))
		if len([]rune(t)) == 2 {
			break
		}
	}
	if t == "" {
		return "?"
	}
	return t
}

func (a *avatar) Tapped(*fyne.PointEvent) {
	if a.alTocar != nil {
		a.alTocar()
	}
}
func (a *avatar) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (a *avatar) CreateRenderer() fyne.WidgetRenderer {
	r := &rendererAvatar{a: a, circulo: canvas.NewCircle(nil), texto: canvas.NewText(a.iniciales, nil)}
	r.texto.TextStyle = fyne.TextStyle{Bold: true}
	r.texto.Alignment = fyne.TextAlignCenter
	r.Refresh()
	return r
}

type rendererAvatar struct {
	a       *avatar
	circulo *canvas.Circle
	texto   *canvas.Text
}

func (r *rendererAvatar) MinSize() fyne.Size { return fyne.NewSquareSize(34) }
func (r *rendererAvatar) Layout(s fyne.Size) {
	lado := fyne.Min(s.Width, s.Height)
	origen := fyne.NewPos((s.Width-lado)/2, (s.Height-lado)/2)
	r.circulo.Resize(fyne.NewSquareSize(lado))
	r.circulo.Move(origen)
	t := r.texto.MinSize()
	r.texto.Resize(fyne.NewSize(lado, t.Height))
	r.texto.Move(fyne.NewPos(origen.X, origen.Y+(lado-t.Height)/2))
}
func (r *rendererAvatar) Refresh() {
	r.circulo.FillColor = theme.ColorForWidget(theme.ColorNamePrimary, r.a)
	r.texto.Color = theme.ColorForWidget(theme.ColorNameForegroundOnPrimary, r.a)
	r.texto.TextSize = 13
	r.circulo.Refresh()
	r.texto.Refresh()
}
func (r *rendererAvatar) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.circulo, r.texto}
}
func (r *rendererAvatar) Destroy() {}

// ---------- barra fina del almacenamiento ----------

type barraFina struct {
	widget.BaseWidget
	valor float32 // de 0 a 1
}

func nuevaBarraFina() *barraFina {
	b := &barraFina{}
	b.ExtendBaseWidget(b)
	return b
}

func (b *barraFina) SetValor(v float64) {
	b.valor = fyne.Max(0, fyne.Min(1, float32(v)))
	b.Refresh()
}

func (b *barraFina) CreateRenderer() fyne.WidgetRenderer {
	r := &rendererBarra{b: b, fondo: canvas.NewRectangle(nil), relleno: canvas.NewRectangle(nil)}
	r.fondo.CornerRadius, r.relleno.CornerRadius = 2, 2
	r.Refresh()
	return r
}

type rendererBarra struct {
	b              *barraFina
	fondo, relleno *canvas.Rectangle
}

func (r *rendererBarra) MinSize() fyne.Size { return fyne.NewSize(40, 4) }
func (r *rendererBarra) Layout(s fyne.Size) {
	r.fondo.Resize(s)
	r.relleno.Resize(fyne.NewSize(s.Width*r.b.valor, s.Height))
}
func (r *rendererBarra) Refresh() {
	r.fondo.FillColor = theme.ColorForWidget(colorSuperficie3, r.b)
	nombre := theme.ColorNamePrimary
	if r.b.valor >= 0.9 {
		nombre = theme.ColorNameError
	}
	r.relleno.FillColor = theme.ColorForWidget(nombre, r.b)
	r.Layout(r.b.Size())
	r.fondo.Refresh()
	r.relleno.Refresh()
}
func (r *rendererBarra) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.relleno}
}
func (r *rendererBarra) Destroy() {}

// ---------- fondo de color del tema (la píldora del buscador) ----------

type fondoTema struct {
	widget.BaseWidget
	color fyne.ThemeColorName
	radio float32
}

func nuevoFondo(color fyne.ThemeColorName, radio float32) *fondoTema {
	f := &fondoTema{color: color, radio: radio}
	f.ExtendBaseWidget(f)
	return f
}

func (f *fondoTema) CreateRenderer() fyne.WidgetRenderer {
	r := &rendererFondo{f: f, rect: canvas.NewRectangle(nil)}
	r.Refresh()
	return r
}

type rendererFondo struct {
	f    *fondoTema
	rect *canvas.Rectangle
}

func (r *rendererFondo) MinSize() fyne.Size { return fyne.NewSize(0, 0) }
func (r *rendererFondo) Layout(s fyne.Size) { r.rect.Resize(s) }
func (r *rendererFondo) Refresh() {
	r.rect.FillColor = theme.ColorForWidget(r.f.color, r.f)
	r.rect.CornerRadius = r.f.radio
	r.rect.Refresh()
}
func (r *rendererFondo) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.rect} }
func (r *rendererFondo) Destroy()                     {}

// temaSinMarco quita el fondo y el borde de un campo de texto para que se
// funda con la píldora que lo rodea. Lo demás lo toma del tema vigente.
type temaSinMarco struct{}

func (temaSinMarco) base() fyne.Theme { return fyne.CurrentApp().Settings().Theme() }

func (t temaSinMarco) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameInputBackground, theme.ColorNameInputBorder:
		return color.Transparent
	}
	return t.base().Color(n, v)
}
func (t temaSinMarco) Font(s fyne.TextStyle) fyne.Resource     { return t.base().Font(s) }
func (t temaSinMarco) Icon(n fyne.ThemeIconName) fyne.Resource { return t.base().Icon(n) }
func (t temaSinMarco) Size(n fyne.ThemeSizeName) float32       { return t.base().Size(n) }

// ---------- disposiciones ----------

// anchoFijo reserva un ancho exacto, como el menú lateral de 256 px de la web.
type anchoFijo struct{ ancho float32 }

func (l anchoFijo) MinSize(os []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range os {
		h = fyne.Max(h, o.MinSize().Height)
	}
	return fyne.NewSize(l.ancho, h)
}

func (l anchoFijo) Layout(os []fyne.CanvasObject, s fyne.Size) {
	for _, o := range os {
		o.Resize(s)
		o.Move(fyne.NewPos(0, 0))
	}
}

// hastaAncho deja crecer al hijo hasta un máximo, pegado a la izquierda: el
// buscador de la web no pasa de 720 px.
type hastaAncho struct{ max float32 }

func (l hastaAncho) MinSize(os []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range os {
		h = fyne.Max(h, o.MinSize().Height)
	}
	return fyne.NewSize(160, h)
}

func (l hastaAncho) Layout(os []fyne.CanvasObject, s fyne.Size) {
	for _, o := range os {
		h := o.MinSize().Height
		o.Resize(fyne.NewSize(fyne.Min(s.Width, l.max), h))
		o.Move(fyne.NewPos(0, (s.Height-h)/2))
	}
}

// columnas reparte una fila en columnas de ancho fijo; las de ancho 0 se
// quedan con lo que sobra. Así la cabecera y las filas de la lista quedan
// alineadas, como la tabla de archivos de la web.
type columnas struct{ anchos []float32 }

const huecoColumnas = 12

func (l columnas) MinSize(os []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for i, o := range os {
		if i >= len(l.anchos) {
			break
		}
		if l.anchos[i] == 0 {
			w += 120
		} else {
			w += l.anchos[i]
		}
		h = fyne.Max(h, o.MinSize().Height)
	}
	return fyne.NewSize(w+huecoColumnas*float32(len(l.anchos)-1), h)
}

func (l columnas) Layout(os []fyne.CanvasObject, s fyne.Size) {
	libre := s.Width - huecoColumnas*float32(len(l.anchos)-1)
	flexibles := 0
	for _, a := range l.anchos {
		if a == 0 {
			flexibles++
		} else {
			libre -= a
		}
	}
	var flexible float32
	if flexibles > 0 {
		flexible = fyne.Max(0, libre/float32(flexibles))
	}

	var x float32
	for i, o := range os {
		if i >= len(l.anchos) {
			o.Hide()
			continue
		}
		w := l.anchos[i]
		if w == 0 {
			w = flexible
		}
		h := fyne.Min(o.MinSize().Height, s.Height)
		o.Resize(fyne.NewSize(w, h))
		o.Move(fyne.NewPos(x, (s.Height-h)/2))
		x += w + huecoColumnas
	}
}
