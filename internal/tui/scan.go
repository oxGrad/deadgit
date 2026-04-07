package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oxGrad/deadgit/internal/cache"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/output"
	"github.com/oxGrad/deadgit/internal/scanner"
)

// ScanView displays the repo scan results table with an optional detail panel.
type ScanView struct {
	app         *App
	flex        *tview.Flex
	table       *tview.Table
	detail      *tview.TextView
	filterInput *tview.InputField
	filterFlex  *tview.Flex

	allRows     []output.RepoRow
	detailShown bool
}

func newScanView(a *App) *ScanView {
	sv := &ScanView{app: a}

	sv.table = tview.NewTable().
		SetBorders(false).
		SetFixed(1, 0).
		SetSelectable(true, false)
	sv.table.SetBackgroundColor(colorBg)
	sv.table.SetSelectedStyle(tcell.StyleDefault.
		Background(colorSelected).
		Foreground(colorText))

	sv.detail = tview.NewTextView().SetDynamicColors(true)
	sv.detail.SetBackgroundColor(colorBg)
	sv.detail.SetBorder(true).SetBorderColor(colorSelected)

	sv.filterInput = tview.NewInputField().
		SetLabel("/").
		SetLabelColor(colorBrand).
		SetFieldBackgroundColor(colorBg).
		SetFieldTextColor(colorText).
		SetPlaceholder("filter…").
		SetPlaceholderTextColor(colorMuted)
	sv.filterInput.SetChangedFunc(func(text string) {
		sv.applyFilter(text)
	})
	sv.filterInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			sv.closeFilter()
			return nil
		}
		return event
	})

	sv.filterFlex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(sv.table, 0, 1, true).
		AddItem(sv.filterInput, 0, 0, false)

	sv.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(sv.filterFlex, 0, 1, true).
		AddItem(sv.detail, 0, 0, false)

	sv.setupKeys()
	return sv
}

func (sv *ScanView) root() tview.Primitive { return sv.flex }

func (sv *ScanView) refresh() {
	ctx := context.Background()
	orgs, err := sv.app.q.ListOrganizations(ctx)
	if err != nil {
		return
	}
	var rows []output.RepoRow
	for _, org := range orgs {
		repos, err := sv.app.q.ListRepositoriesByOrg(ctx, org.Slug)
		if err != nil {
			continue
		}
		for _, r := range repos {
			rows = append(rows, output.RepoRow{
				OrgSlug: org.Slug,
				Project: r.ProjectName,
				Repo:    r.Name,
				Cached:  !cache.IsStale(r.LastFetched, 24),
			})
		}
	}
	sv.allRows = rows
	sv.renderTable(rows)
}

func (sv *ScanView) renderTable(rows []output.RepoRow) {
	sv.table.Clear()
	headers := []string{"ORG", "PROJECT", "REPO", "SCORE", "STATUS", "CACHED"}
	for col, h := range headers {
		sv.table.SetCell(0, col, tview.NewTableCell(h).
			SetTextColor(colorText).
			SetBackgroundColor(colorHeader).
			SetSelectable(false).
			SetExpansion(1))
	}
	for i, r := range rows {
		status := "ACTIVE"
		statusColor := colorActive
		if r.IsInactive {
			status = "DEAD"
			statusColor = colorDead
		}
		cached := ""
		if r.Cached {
			cached = "✓"
		}
		data := []string{r.OrgSlug, r.Project, r.Repo, fmt.Sprintf("%.2f", r.Score), status, cached}
		for col, val := range data {
			cell := tview.NewTableCell(val).SetExpansion(1)
			if col == 4 {
				cell.SetTextColor(statusColor)
			} else {
				cell.SetTextColor(colorText)
			}
			sv.table.SetCell(i+1, col, cell)
		}
	}
	sv.table.ScrollToBeginning()
}

func (sv *ScanView) setupKeys() {
	sv.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'd':
				sv.toggleDetail()
				return nil
			case 's':
				sv.triggerScan()
				return nil
			case '/':
				sv.openFilter()
				return nil
			}
		}
		return event
	})
	sv.table.SetSelectionChangedFunc(func(row, col int) {
		if sv.detailShown {
			sv.updateDetail(row)
		}
	})
}

func (sv *ScanView) toggleDetail() {
	sv.detailShown = !sv.detailShown
	if sv.detailShown {
		sv.flex.ResizeItem(sv.detail, 8, 0)
		row, _ := sv.table.GetSelection()
		sv.updateDetail(row)
	} else {
		sv.flex.ResizeItem(sv.detail, 0, 0)
	}
}

func (sv *ScanView) updateDetail(row int) {
	if row <= 0 || row-1 >= len(sv.allRows) {
		sv.detail.SetText("")
		return
	}
	r := sv.allRows[row-1]
	reasons := strings.Join(r.Reasons, ", ")
	if reasons == "" {
		reasons = "—"
	}
	sv.detail.SetText(fmt.Sprintf(
		"%s%s[-] — Score: [white]%.2f[-]\nOrg: [white]%s[-]  Project: [white]%s[-]\nReasons: %s%s[-]",
		markupBrand, r.Repo, r.Score, r.OrgSlug, r.Project, markupMuted, reasons,
	))
}

func (sv *ScanView) openFilter() {
	sv.filterFlex.ResizeItem(sv.filterInput, 1, 0)
	sv.app.tv.SetFocus(sv.filterInput)
}

func (sv *ScanView) closeFilter() {
	sv.filterFlex.ResizeItem(sv.filterInput, 0, 0)
	sv.filterInput.SetText("")
	sv.renderTable(sv.allRows)
	sv.app.tv.SetFocus(sv.table)
}

func (sv *ScanView) applyFilter(text string) {
	if text == "" {
		sv.renderTable(sv.allRows)
		return
	}
	lower := strings.ToLower(text)
	var filtered []output.RepoRow
	for _, r := range sv.allRows {
		if strings.Contains(strings.ToLower(r.OrgSlug), lower) ||
			strings.Contains(strings.ToLower(r.Project), lower) ||
			strings.Contains(strings.ToLower(r.Repo), lower) {
			filtered = append(filtered, r)
		}
	}
	sv.renderTable(filtered)
}

func (sv *ScanView) triggerScan() {
	if sv.app.scanning {
		return
	}
	sv.app.showScanConfigForm()
}

func (sv *ScanView) startScan(orgs []dbgen.Organization, prof dbgen.ScoringProfile) {
	if sv.app.scanning {
		return
	}
	sv.app.scanning = true
	sv.app.switchTo("scan")

	ctx, cancel := context.WithCancel(context.Background())
	sv.app.cancelScan = cancel

	profile := scanner.DBProfileToScoringProfile(prof)
	cfg := scanner.Config{
		Orgs:    orgs,
		Profile: profile,
		Workers: 5,
		TTL:     24,
	}

	progressFn := func(done, total int, repoName string) {
		bar := buildProgressBar(done, total)
		sv.app.setProgress(
			fmt.Sprintf("● scanning %s · %s  %s  %d/%d", orgSlugs(orgs), prof.Name, bar, done, total),
			fmt.Sprintf("%s  %s<ctrl+x>[-] cancel", repoName, markupMuted),
		)
	}

	go func() {
		rows, _, _ := scanner.Run(ctx, sv.app.q, cfg, sv.app.provFn, progressFn)
		sv.app.tv.QueueUpdateDraw(func() {
			sv.allRows = rows
			sv.renderTable(rows)
			sv.app.clearProgress()
		})
	}()
}

func buildProgressBar(done, total int) string {
	const width = 16
	if total == 0 {
		return "[" + strings.Repeat("░", width) + "]"
	}
	filled := done * width / total
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func orgSlugs(orgs []dbgen.Organization) string {
	slugs := make([]string, len(orgs))
	for i, o := range orgs {
		slugs[i] = o.Slug
	}
	return strings.Join(slugs, ",")
}
