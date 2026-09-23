package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Tema de UPB-CIENTÍFICA: la misma paleta que usa la aplicación móvil, para
// que las tres —web, móvil y escritorio— se vean como un solo sistema.
//
// Los colores claros salen tal cual de lib/core/theme/app_colors.dart del
// repositorio móvil. Los oscuros son su equivalente: el azul se aclara, porque
// el #1A73E8 sobre fondo oscuro no tiene contraste suficiente para leerse.

var (
	// Claros (móvil: AppColors)
	azul        = rgb(0x1A73E8)
	azulClaro   = rgb(0xE8F0FE)
	blanco      = rgb(0xFFFFFF)
	fondoClaro  = rgb(0xF8F9FA)
	textoClaro1 = rgb(0x202124)
	textoClaro3 = rgb(0x9AA0A6)
	bordeClaro  = rgb(0xDADCE0)
	exito       = rgb(0x34A853)
	aviso       = rgb(0xFBBC04)
	fallo       = rgb(0xEA4335)

	// Oscuros
	azulOscuro    = rgb(0x8AB4F8)
	seleccionOsc  = rgb(0x1F3A5F)
	fondoOscuro   = rgb(0x202124)
	superficieOsc = rgb(0x292A2D)
	textoOscuro1  = rgb(0xE8EAED)
	textoOscuro3  = rgb(0x9AA0A6)
	bordeOscuro   = rgb(0x3C4043)
	exitoOscuro   = rgb(0x81C995)
	avisoOscuro   = rgb(0xFDD663)
	falloOscuro   = rgb(0xF28B82)
)

func rgb(v uint32) color.Color {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

func negro(alfa uint8) color.Color { return color.NRGBA{A: alfa} }
func blancoAlfa(alfa uint8) color.Color {
	return color.NRGBA{R: 255, G: 255, B: 255, A: alfa}
}

// Preferencia de tema del usuario.
type Preferencia string

const (
	TemaSistema Preferencia = "sistema"
	TemaClaro   Preferencia = "claro"
	TemaOscuro  Preferencia = "oscuro"
)

// temaUPB implementa fyne.Theme. Cuando el usuario fija claro u oscuro, se
// ignora la variante que trae el sistema.
type temaUPB struct {
	preferencia Preferencia
}

func (t temaUPB) variante(delSistema fyne.ThemeVariant) fyne.ThemeVariant {
	switch t.preferencia {
	case TemaClaro:
		return theme.VariantLight
	case TemaOscuro:
		return theme.VariantDark
	default:
		return delSistema
	}
}

func (t temaUPB) Color(nombre fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if t.variante(v) == theme.VariantLight {
		return t.colorClaro(nombre)
	}
	return t.colorOscuro(nombre)
}

func (t temaUPB) colorClaro(n fyne.ThemeColorName) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return fondoClaro
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground, theme.ColorNameInputBackground:
		return blanco
	case theme.ColorNameForeground, theme.ColorNameForegroundOnError:
		return textoClaro1
	case theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnWarning:
		return blanco
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return textoClaro3
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return azul
	case theme.ColorNameFocus:
		return azul
	case theme.ColorNameSelection:
		return azulClaro
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		return bordeClaro
	case theme.ColorNameButton, theme.ColorNameDisabledButton:
		return blanco
	case theme.ColorNameHover:
		return negro(10)
	case theme.ColorNamePressed:
		return negro(20)
	case theme.ColorNameScrollBar:
		return negro(60)
	case theme.ColorNameShadow:
		return negro(25)
	case theme.ColorNameSuccess:
		return exito
	case theme.ColorNameWarning:
		return aviso
	case theme.ColorNameError:
		return fallo
	default:
		return theme.DefaultTheme().Color(n, theme.VariantLight)
	}
}

func (t temaUPB) colorOscuro(n fyne.ThemeColorName) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return fondoOscuro
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground, theme.ColorNameInputBackground:
		return superficieOsc
	case theme.ColorNameForeground:
		return textoOscuro1
	case theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnSuccess,
		theme.ColorNameForegroundOnWarning, theme.ColorNameForegroundOnError:
		return fondoOscuro
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return textoOscuro3
	case theme.ColorNamePrimary, theme.ColorNameHyperlink, theme.ColorNameFocus:
		return azulOscuro
	case theme.ColorNameSelection:
		return seleccionOsc
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		return bordeOscuro
	case theme.ColorNameButton, theme.ColorNameDisabledButton:
		return superficieOsc
	case theme.ColorNameHover:
		return blancoAlfa(16)
	case theme.ColorNamePressed:
		return blancoAlfa(28)
	case theme.ColorNameScrollBar:
		return blancoAlfa(70)
	case theme.ColorNameShadow:
		return negro(120)
	case theme.ColorNameSuccess:
		return exitoOscuro
	case theme.ColorNameWarning:
		return avisoOscuro
	case theme.ColorNameError:
		return falloOscuro
	default:
		return theme.DefaultTheme().Color(n, theme.VariantDark)
	}
}

func (t temaUPB) Font(estilo fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(estilo)
}

func (t temaUPB) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

// Size sigue el sistema de espaciado del móvil: esquinas de 12 y un poco más
// de aire entre elementos que el de serie.
func (t temaUPB) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 10
	case theme.SizeNamePadding:
		return 5
	default:
		return theme.DefaultTheme().Size(n)
	}
}
