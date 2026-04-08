package tui

import "github.com/gdamore/tcell/v2"

var (
	colorBg       = tcell.GetColor("#0d1117")
	colorHeader   = tcell.GetColor("#1a1a2e")
	colorSelected = tcell.GetColor("#1f6feb")
	colorActive   = tcell.GetColor("#3fb950")
	colorDead     = tcell.GetColor("#f85149")
	colorScanning = tcell.GetColor("#f0883e")
	colorMuted    = tcell.GetColor("#888888")
	colorBrand    = tcell.GetColor("#7ef542")
	colorText     = tcell.ColorWhite
)

const (
	markupBrand    = "[#7ef542]"
	markupMuted    = "[#888888]"
	markupScanning = "[#f0883e]"
	markupActive   = "[#3fb950]"
	markupDead     = "[#f85149]"
	markupReset    = "[-]"
	markupWhite    = "[white]"
)
