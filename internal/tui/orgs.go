package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
)

// OrgsView lists registered organizations.
type OrgsView struct {
	app         *App
	flex        *tview.Flex
	table       *tview.Table
	filterInput *tview.InputField
	filterFlex  *tview.Flex

	allOrgs []dbgen.Organization
}

func newOrgsView(a *App) *OrgsView {
	ov := &OrgsView{app: a}

	ov.table = tview.NewTable().
		SetBorders(false).
		SetFixed(1, 0).
		SetSelectable(true, false)
	ov.table.SetBackgroundColor(colorBg)
	ov.table.SetSelectedStyle(tcell.StyleDefault.
		Background(colorSelected).Foreground(colorText))

	ov.filterInput = tview.NewInputField().
		SetLabel("/").
		SetLabelColor(colorBrand).
		SetFieldBackgroundColor(colorBg).
		SetFieldTextColor(colorText).
		SetPlaceholder("filter…").
		SetPlaceholderTextColor(colorMuted)
	ov.filterInput.SetChangedFunc(func(text string) { ov.applyFilter(text) })
	ov.filterInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			ov.closeFilter()
			return nil
		}
		return event
	})

	ov.filterFlex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ov.table, 0, 1, true).
		AddItem(ov.filterInput, 0, 0, false)

	ov.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ov.filterFlex, 0, 1, true)

	ov.setupKeys()
	return ov
}

func (ov *OrgsView) root() tview.Primitive { return ov.flex }

func (ov *OrgsView) refresh() {
	orgs, err := ov.app.q.ListAllOrganizations(context.Background())
	if err != nil {
		return
	}
	ov.allOrgs = orgs
	ov.renderTable(orgs)
}

func (ov *OrgsView) renderTable(orgs []dbgen.Organization) {
	ov.table.Clear()
	headers := []string{"SLUG", "PROVIDER", "TYPE", "STATUS", "BASE URL", "PAT ENV"}
	for col, h := range headers {
		ov.table.SetCell(0, col, tview.NewTableCell(h).
			SetTextColor(colorText).SetBackgroundColor(colorHeader).
			SetSelectable(false).SetExpansion(1))
	}
	for i, o := range orgs {
		status := "active"
		statusColor := colorActive
		if o.IsActive == 0 {
			status = "inactive"
			statusColor = colorDead
		}
		data := []string{o.Slug, o.Provider, o.AccountType, status, o.BaseUrl, o.PatEnv}
		for col, val := range data {
			cell := tview.NewTableCell(val).SetExpansion(1)
			if col == 3 {
				cell.SetTextColor(statusColor)
			} else {
				cell.SetTextColor(colorText)
			}
			ov.table.SetCell(i+1, col, cell)
		}
	}
	ov.table.ScrollToBeginning()
}

func (ov *OrgsView) setupKeys() {
	ov.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'n':
				ov.app.showOrgForm()
				return nil
			case 'x':
				ov.deactivateSelected()
				return nil
			case '/':
				ov.openFilter()
				return nil
			}
		}
		return event
	})
}

func (ov *OrgsView) deactivateSelected() {
	row, _ := ov.table.GetSelection()
	if row <= 0 || row-1 >= len(ov.allOrgs) {
		return
	}
	slug := ov.allOrgs[row-1].Slug
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Deactivate org %q?", slug)).
		AddButtons([]string{"Cancel", "Deactivate"}).
		SetDoneFunc(func(_ int, buttonLabel string) {
			ov.app.pages.RemovePage("confirm")
			ov.app.tv.SetFocus(ov.table)
			if buttonLabel == "Deactivate" {
				_ = ov.app.q.DeactivateOrganization(context.Background(), slug)
				ov.refresh()
			}
		})
	ov.app.pages.AddPage("confirm", modal, true, true)
	ov.app.tv.SetFocus(modal)
}

func (ov *OrgsView) openFilter() {
	ov.filterFlex.ResizeItem(ov.filterInput, 1, 0)
	ov.app.tv.SetFocus(ov.filterInput)
}

func (ov *OrgsView) closeFilter() {
	ov.filterFlex.ResizeItem(ov.filterInput, 0, 0)
	ov.filterInput.SetText("")
	ov.renderTable(ov.allOrgs)
	ov.app.tv.SetFocus(ov.table)
}

func (ov *OrgsView) applyFilter(text string) {
	if text == "" {
		ov.renderTable(ov.allOrgs)
		return
	}
	lower := strings.ToLower(text)
	var filtered []dbgen.Organization
	for _, o := range ov.allOrgs {
		if strings.Contains(strings.ToLower(o.Slug), lower) ||
			strings.Contains(strings.ToLower(o.Provider), lower) {
			filtered = append(filtered, o)
		}
	}
	ov.renderTable(filtered)
}
