package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/scanner"
)

// App is the root TUI application.
type App struct {
	tv          *tview.Application
	pages       *tview.Pages
	bottomPages *tview.Pages
	root        *tview.Flex

	header      *tview.TextView
	progressBar *tview.TextView
	helpBar     *tview.TextView
	cmdInput    *tview.InputField

	q *dbgen.Queries

	scanView     *ScanView
	profilesView *ProfilesView
	orgsView     *OrgsView

	currentView string
	scanning    bool
	cancelScan  context.CancelFunc
	provFn      scanner.ProviderFunc
}

// Run initialises and starts the TUI application.
func Run(q *dbgen.Queries) error {
	a := &App{
		q:      q,
		provFn: scanner.DefaultProviderFor,
	}
	a.build()
	return a.tv.SetRoot(a.root, true).EnableMouse(false).Run()
}

func (a *App) build() {
	a.tv = tview.NewApplication()

	a.header = tview.NewTextView().SetDynamicColors(true)
	a.header.SetBackgroundColor(colorHeader)

	a.pages = tview.NewPages()

	a.progressBar = tview.NewTextView().SetDynamicColors(true)
	a.progressBar.SetBackgroundColor(colorHeader)

	a.helpBar = tview.NewTextView().SetDynamicColors(true)
	a.helpBar.SetBackgroundColor(colorHeader)

	a.cmdInput = tview.NewInputField().
		SetLabel(":").
		SetLabelColor(colorBrand).
		SetFieldBackgroundColor(colorBg).
		SetFieldTextColor(colorText)
	a.cmdInput.SetDoneFunc(func(key tcell.Key) {
		cmd := strings.TrimSpace(a.cmdInput.GetText())
		a.closeCommandBar()
		switch cmd {
		case "scan":
			a.switchTo("scan")
		case "profiles":
			a.switchTo("profiles")
		case "orgs":
			a.switchTo("orgs")
		}
	})
	a.cmdInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			a.closeCommandBar()
			return nil
		}
		return event
	})

	a.bottomPages = tview.NewPages()
	a.bottomPages.AddPage("help", a.helpBar, true, true)
	a.bottomPages.AddPage("cmd", a.cmdInput, true, false)

	a.scanView = newScanView(a)
	a.profilesView = newProfilesView(a)
	a.orgsView = newOrgsView(a)

	a.pages.AddPage("scan", a.scanView.root(), true, true)
	a.pages.AddPage("profiles", a.profilesView.root(), true, false)
	a.pages.AddPage("orgs", a.orgsView.root(), true, false)

	a.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header, 1, 0, false).
		AddItem(a.pages, 0, 1, true).
		AddItem(a.progressBar, 2, 0, false).
		AddItem(a.bottomPages, 1, 0, false)

	a.tv.SetInputCapture(a.handleGlobalKey)
	a.switchTo("scan")
}

func (a *App) switchTo(name string) {
	a.currentView = name
	a.pages.SwitchToPage(name)
	a.header.SetText(a.headerText(name))
	a.updateHelp()

	switch name {
	case "scan":
		a.scanView.refresh()
		a.tv.SetFocus(a.scanView.table)
	case "profiles":
		a.profilesView.refresh()
		a.tv.SetFocus(a.profilesView.table)
	case "orgs":
		a.orgsView.refresh()
		a.tv.SetFocus(a.orgsView.table)
	}
}

func (a *App) showPage(name string, focusPrimitive tview.Primitive) {
	a.pages.SwitchToPage(name)
	a.tv.SetFocus(focusPrimitive)
}

func (a *App) headerText(view string) string {
	order := []string{"scan", "profiles", "orgs"}
	nums := map[string]int{"scan": 1, "profiles": 2, "orgs": 3}
	var hints []string
	for _, v := range order {
		if v == view {
			hints = append(hints, fmt.Sprintf("%s<[white]%d[#7ef542]>[-][white]%s[-]", markupBrand, nums[v], v))
		} else {
			hints = append(hints, fmt.Sprintf("%s<%d>%s[-]", markupMuted, nums[v], v))
		}
	}
	return fmt.Sprintf("%sdeadgit [-]%s›[-] [white]%s[-]          %s",
		markupBrand, markupMuted, view, strings.Join(hints, "  "))
}

func (a *App) updateHelp() {
	switch a.currentView {
	case "scan":
		a.helpBar.SetText(markupMuted + "<s>scan  <d>detail  </>filter  <:>cmd  <q>quit" + markupReset)
	case "profiles":
		a.helpBar.SetText(markupMuted + "<n>new  <e>edit  <enter>default  <x>delete  </>filter  <:>cmd  <q>quit" + markupReset)
	case "orgs":
		a.helpBar.SetText(markupMuted + "<n>new  <x>deactivate  </>filter  <:>cmd  <q>quit" + markupReset)
	}
}

func (a *App) setProgress(line1, line2 string) {
	a.tv.QueueUpdateDraw(func() {
		a.progressBar.SetText(markupScanning + line1 + "\n" + markupMuted + line2 + markupReset)
	})
}

func (a *App) clearProgress() {
	a.tv.QueueUpdateDraw(func() {
		a.progressBar.SetText("")
		a.scanning = false
		a.cancelScan = nil
	})
}

func (a *App) openCommandBar() {
	a.cmdInput.SetText("")
	a.bottomPages.SwitchToPage("cmd")
	a.tv.SetFocus(a.cmdInput)
}

func (a *App) closeCommandBar() {
	a.bottomPages.SwitchToPage("help")
	switch a.currentView {
	case "scan":
		a.tv.SetFocus(a.scanView.table)
	case "profiles":
		a.tv.SetFocus(a.profilesView.table)
	case "orgs":
		a.tv.SetFocus(a.orgsView.table)
	}
}

func (a *App) handleGlobalKey(event *tcell.EventKey) *tcell.EventKey {
	if a.tv.GetFocus() == a.cmdInput {
		return event
	}
	switch event.Key() {
	case tcell.KeyCtrlX:
		if a.scanning && a.cancelScan != nil {
			a.cancelScan()
		}
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case '1':
			a.switchTo("scan")
			return nil
		case '2':
			a.switchTo("profiles")
			return nil
		case '3':
			a.switchTo("orgs")
			return nil
		case ':':
			a.openCommandBar()
			return nil
		case 'q':
			a.tv.Stop()
			return nil
		}
	}
	return event
}
