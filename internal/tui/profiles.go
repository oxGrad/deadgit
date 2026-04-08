package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
)

// ProfilesView lists scoring profiles with CRUD key bindings.
type ProfilesView struct {
	app         *App
	flex        *tview.Flex
	table       *tview.Table
	filterInput *tview.InputField
	filterFlex  *tview.Flex

	allProfiles []dbgen.ScoringProfile
}

func newProfilesView(a *App) *ProfilesView {
	pv := &ProfilesView{app: a}

	pv.table = tview.NewTable().
		SetBorders(false).
		SetFixed(1, 0).
		SetSelectable(true, false)
	pv.table.SetBackgroundColor(colorBg)
	pv.table.SetSelectedStyle(tcell.StyleDefault.
		Background(colorSelected).Foreground(colorText))

	pv.filterInput = tview.NewInputField().
		SetLabel("/").
		SetLabelColor(colorBrand).
		SetFieldBackgroundColor(colorBg).
		SetFieldTextColor(colorText).
		SetPlaceholder("filter…").
		SetPlaceholderTextColor(colorMuted)
	pv.filterInput.SetChangedFunc(func(text string) { pv.applyFilter(text) })
	pv.filterInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			pv.closeFilter()
			return nil
		}
		return event
	})

	pv.filterFlex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pv.table, 0, 1, true).
		AddItem(pv.filterInput, 0, 0, false)

	pv.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pv.filterFlex, 0, 1, true)

	pv.setupKeys()
	return pv
}

func (pv *ProfilesView) root() tview.Primitive { return pv.flex }

func (pv *ProfilesView) refresh() {
	profiles, err := pv.app.q.ListProfiles(context.Background())
	if err != nil {
		return
	}
	pv.allProfiles = profiles
	pv.renderTable(profiles)
}

func (pv *ProfilesView) renderTable(profiles []dbgen.ScoringProfile) {
	pv.table.Clear()
	headers := []string{"NAME", "VER", "COMMIT", "PR", "FREQ", "BRANCH", "RELEASE", "THRESHOLD", "SCORE MIN", "DEFAULT"}
	for col, h := range headers {
		pv.table.SetCell(0, col, tview.NewTableCell(h).
			SetTextColor(colorText).SetBackgroundColor(colorHeader).
			SetSelectable(false).SetExpansion(1))
	}
	for i, p := range profiles {
		def := ""
		if p.IsDefault == 1 {
			def = "✓"
		}
		data := []string{
			p.Name,
			strconv.FormatInt(p.Version, 10),
			fmt.Sprintf("%.2f", p.WLastCommit),
			fmt.Sprintf("%.2f", p.WLastPr),
			fmt.Sprintf("%.2f", p.WCommitFrequency),
			fmt.Sprintf("%.2f", p.WBranchStaleness),
			fmt.Sprintf("%.2f", p.WNoReleases),
			fmt.Sprintf("%dd", p.InactiveDaysThreshold),
			fmt.Sprintf("%.2f", p.InactiveScoreThreshold),
			def,
		}
		for col, val := range data {
			cell := tview.NewTableCell(val).SetExpansion(1)
			if col == 9 && val == "✓" {
				cell.SetTextColor(colorActive)
			} else {
				cell.SetTextColor(colorText)
			}
			pv.table.SetCell(i+1, col, cell)
		}
	}
	pv.table.ScrollToBeginning()
}

func (pv *ProfilesView) setupKeys() {
	pv.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			pv.setSelectedAsDefault()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 'n':
				pv.app.showProfileForm(nil)
				return nil
			case 'e':
				row, _ := pv.table.GetSelection()
				if row > 0 && row-1 < len(pv.allProfiles) {
					p := pv.allProfiles[row-1]
					pv.app.showProfileForm(&p)
				}
				return nil
			case 'x':
				pv.deleteSelected()
				return nil
			case '/':
				pv.openFilter()
				return nil
			}
		}
		return event
	})
}

func (pv *ProfilesView) setSelectedAsDefault() {
	row, _ := pv.table.GetSelection()
	if row <= 0 || row-1 >= len(pv.allProfiles) {
		return
	}
	_ = pv.app.q.SetDefaultProfile(context.Background(), pv.allProfiles[row-1].Name)
	pv.refresh()
}

func (pv *ProfilesView) deleteSelected() {
	row, _ := pv.table.GetSelection()
	if row <= 0 || row-1 >= len(pv.allProfiles) {
		return
	}
	name := pv.allProfiles[row-1].Name
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Delete profile %q?", name)).
		AddButtons([]string{"Cancel", "Delete"}).
		SetDoneFunc(func(_ int, buttonLabel string) {
			pv.app.pages.RemovePage("confirm")
			pv.app.tv.SetFocus(pv.table)
			if buttonLabel == "Delete" {
				_ = pv.app.q.DeleteProfile(context.Background(), name)
				pv.refresh()
			}
		})
	pv.app.pages.AddPage("confirm", modal, true, true)
	pv.app.tv.SetFocus(modal)
}

func (pv *ProfilesView) openFilter() {
	pv.filterFlex.ResizeItem(pv.filterInput, 1, 0)
	pv.app.tv.SetFocus(pv.filterInput)
}

func (pv *ProfilesView) closeFilter() {
	pv.filterFlex.ResizeItem(pv.filterInput, 0, 0)
	pv.filterInput.SetText("")
	pv.renderTable(pv.allProfiles)
	pv.app.tv.SetFocus(pv.table)
}

func (pv *ProfilesView) applyFilter(text string) {
	if text == "" {
		pv.renderTable(pv.allProfiles)
		return
	}
	lower := strings.ToLower(text)
	var filtered []dbgen.ScoringProfile
	for _, p := range pv.allProfiles {
		if strings.Contains(strings.ToLower(p.Name), lower) {
			filtered = append(filtered, p)
		}
	}
	pv.renderTable(filtered)
}
