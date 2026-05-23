package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type customTheme struct{}

func (m customTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	// Custom dark mode theme palette
	switch name {
	case theme.ColorNameBackground:
		return color.RGBA{R: 10, G: 15, B: 29, A: 255} // #0A0F1D
	case theme.ColorNameForeground:
		return color.RGBA{R: 248, G: 250, B: 252, A: 255} // #F8FAFC
	case theme.ColorNamePrimary:
		return color.RGBA{R: 6, G: 182, B: 212, A: 255} // #06B6D4 Cyan Accent
	case theme.ColorNameInputBackground:
		return color.RGBA{R: 22, G: 34, B: 63, A: 255} // #16223F
	case theme.ColorNameButton:
		return color.RGBA{R: 29, G: 45, B: 80, A: 255} // #1D2D50
	case theme.ColorNameHover:
		return color.RGBA{R: 45, G: 65, B: 110, A: 255} // Hover
	case theme.ColorNamePlaceHolder:
		return color.RGBA{R: 100, G: 116, B: 139, A: 255} // #64748B
	case theme.ColorNameMenuBackground:
		return color.RGBA{R: 15, G: 23, B: 42, A: 255} // #0F172A
	case theme.ColorNameHeaderBackground:
		return color.RGBA{R: 30, G: 41, B: 59, A: 255} // #1E293B
	case theme.ColorNameSeparator:
		return color.RGBA{R: 51, G: 65, B: 85, A: 255} // #334155
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (m customTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m customTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m customTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 14
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNamePadding:
		return 12
	case theme.SizeNameScrollBar:
		return 8
	case theme.SizeNameInputRadius:
		return 8
	case theme.SizeNameSelectionRadius:
		return 8
	}
	return theme.DefaultTheme().Size(name)
}
