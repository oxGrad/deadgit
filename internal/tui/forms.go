package tui

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
	"github.com/oxGrad/deadgit/internal/providers"
	"github.com/oxGrad/deadgit/internal/providers/azure"
	"github.com/oxGrad/deadgit/internal/providers/github"
)

// showProfileForm opens the profile create (existing==nil) or edit (existing!=nil) form.
func (a *App) showProfileForm(existing *dbgen.ScoringProfile) {
	title := "Create Profile"
	if existing != nil {
		title = fmt.Sprintf("Edit Profile: %s", existing.Name)
	}

	name := ""
	desc := ""
	wc, wpr, wf, wb, wr := "0.50", "0.20", "0.20", "0.10", "0.00"
	thresh, scoreMin := "90", "0.65"
	setDefault := false

	if existing != nil {
		name = existing.Name
		if existing.Description.Valid {
			desc = existing.Description.String
		}
		wc = fmt.Sprintf("%.2f", existing.WLastCommit)
		wpr = fmt.Sprintf("%.2f", existing.WLastPr)
		wf = fmt.Sprintf("%.2f", existing.WCommitFrequency)
		wb = fmt.Sprintf("%.2f", existing.WBranchStaleness)
		wr = fmt.Sprintf("%.2f", existing.WNoReleases)
		thresh = strconv.FormatInt(existing.InactiveDaysThreshold, 10)
		scoreMin = fmt.Sprintf("%.2f", existing.InactiveScoreThreshold)
	}

	form := tview.NewForm()
	form.SetBackgroundColor(colorBg)
	form.SetBorderPadding(1, 1, 2, 2)

	if existing == nil {
		form.AddInputField("Name", name, 40, nil, func(t string) { name = t })
	}
	form.AddInputField("Description", desc, 60, nil, func(t string) { desc = t })
	form.AddInputField("Weight: Last Commit", wc, 10, acceptFloat, func(t string) { wc = t })
	form.AddInputField("Weight: Last PR", wpr, 10, acceptFloat, func(t string) { wpr = t })
	form.AddInputField("Weight: Commit Freq", wf, 10, acceptFloat, func(t string) { wf = t })
	form.AddInputField("Weight: Branch Staleness", wb, 10, acceptFloat, func(t string) { wb = t })
	form.AddInputField("Weight: No Releases", wr, 10, acceptFloat, func(t string) { wr = t })
	form.AddInputField("Inactive Threshold (days)", thresh, 10, acceptInt, func(t string) { thresh = t })
	form.AddInputField("Score Min (0–1)", scoreMin, 10, acceptFloat, func(t string) { scoreMin = t })
	if existing == nil {
		form.AddCheckbox("Set as Default", false, func(checked bool) { setDefault = checked })
	}

	form.SetTitle(" " + title + " ").SetBorder(true).SetBorderColor(colorSelected)

	helpText := tview.NewTextView().SetDynamicColors(true).
		SetText(markupMuted + "<ctrl+s> save   <esc> cancel   <tab> next field" + markupReset)
	helpText.SetBackgroundColor(colorHeader)

	formFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(helpText, 1, 0, false)

	saveAndClose := func() {
		ctx := context.Background()
		wcF, _ := strconv.ParseFloat(wc, 64)
		wprF, _ := strconv.ParseFloat(wpr, 64)
		wfF, _ := strconv.ParseFloat(wf, 64)
		wbF, _ := strconv.ParseFloat(wb, 64)
		wrF, _ := strconv.ParseFloat(wr, 64)
		threshI, _ := strconv.ParseInt(thresh, 10, 64)
		scoreMinF, _ := strconv.ParseFloat(scoreMin, 64)
		descNull := sql.NullString{String: desc, Valid: desc != ""}

		if existing == nil {
			isDefault := int64(0)
			if setDefault {
				isDefault = 1
			}
			_, err := a.q.CreateScoringProfile(ctx, dbgen.CreateScoringProfileParams{
				Name:                   name,
				Description:            descNull,
				IsDefault:              isDefault,
				WLastCommit:            wcF,
				WLastPr:                wprF,
				WCommitFrequency:       wfF,
				WBranchStaleness:       wbF,
				WNoReleases:            wrF,
				InactiveDaysThreshold:  threshI,
				InactiveScoreThreshold: scoreMinF,
			})
			if err == nil && setDefault {
				_ = a.q.SetDefaultProfile(ctx, name)
			}
		} else {
			_, _ = a.q.UpdateProfile(ctx, dbgen.UpdateProfileParams{
				Name:                   existing.Name,
				Description:            descNull,
				WLastCommit:            wcF,
				WLastPr:                wprF,
				WCommitFrequency:       wfF,
				WBranchStaleness:       wbF,
				WNoReleases:            wrF,
				InactiveDaysThreshold:  threshI,
				InactiveScoreThreshold: scoreMinF,
			})
		}
		a.pages.RemovePage("profile-form")
		a.switchTo("profiles")
	}

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlS {
			saveAndClose()
			return nil
		}
		if event.Key() == tcell.KeyEsc {
			a.pages.RemovePage("profile-form")
			a.switchTo("profiles")
			return nil
		}
		return event
	})

	a.pages.AddPage("profile-form", formFlex, true, true)
	a.tv.SetFocus(form)
}

// showOrgForm opens the add org form.
func (a *App) showOrgForm() {
	slug, displayName, patEnv := "", "", ""
	provider := "github"
	accountType := "org"

	form := tview.NewForm()
	form.SetBackgroundColor(colorBg)
	form.SetBorderPadding(1, 1, 2, 2)

	form.AddInputField("Slug", "", 40, nil, func(t string) { slug = t })
	form.AddInputField("Display Name (optional)", "", 40, nil, func(t string) { displayName = t })
	form.AddDropDown("Provider", []string{"github", "azure"}, 0, func(opt string, _ int) { provider = opt })
	form.AddDropDown("Account Type", []string{"org", "personal"}, 0, func(opt string, _ int) { accountType = opt })
	form.AddInputField("PAT Env Var (e.g. GITHUB_PAT)", "", 40, nil, func(t string) { patEnv = t })

	form.SetTitle(" Add Organization ").SetBorder(true).SetBorderColor(colorSelected)

	statusText := tview.NewTextView().SetDynamicColors(true)
	statusText.SetBackgroundColor(colorHeader)

	helpText := tview.NewTextView().SetDynamicColors(true).
		SetText(markupMuted + "<ctrl+s> save   <esc> cancel   <tab> next field" + markupReset)
	helpText.SetBackgroundColor(colorHeader)

	formFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(statusText, 1, 0, false).
		AddItem(helpText, 1, 0, false)

	saveAndClose := func() {
		ctx := context.Background()
		if slug == "" || patEnv == "" {
			statusText.SetText(markupDead + "slug and PAT env var are required" + markupReset)
			return
		}
		if displayName == "" {
			displayName = slug
		}
		baseURL := "https://api.github.com"
		if provider == "azure" {
			baseURL = "https://dev.azure.com"
		}
		pat := os.Getenv(patEnv)
		statusText.SetText(markupScanning + "Validating connectivity…" + markupReset)
		a.tv.Draw()

		var prov providers.Provider
		switch provider {
		case "azure":
			prov = azure.New(baseURL, pat)
		case "github":
			prov = github.New(baseURL, pat, accountType)
		}
		newOrg := providers.Organization{
			Slug: slug, Name: displayName, Provider: provider,
			AccountType: accountType, BaseURL: baseURL, PatEnv: patEnv,
		}
		if _, err := prov.ListProjects(newOrg); err != nil {
			statusText.SetText(markupDead + fmt.Sprintf("connectivity failed: %v", err) + markupReset)
			return
		}
		_, err := a.q.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
			Name:        displayName,
			Slug:        slug,
			Provider:    provider,
			AccountType: accountType,
			BaseUrl:     baseURL,
			PatEnv:      patEnv,
		})
		if err != nil {
			statusText.SetText(markupDead + fmt.Sprintf("save failed: %v", err) + markupReset)
			return
		}
		a.pages.RemovePage("org-form")
		a.switchTo("orgs")
	}

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlS {
			saveAndClose()
			return nil
		}
		if event.Key() == tcell.KeyEsc {
			a.pages.RemovePage("org-form")
			a.switchTo("orgs")
			return nil
		}
		return event
	})

	a.pages.AddPage("org-form", formFlex, true, true)
	a.tv.SetFocus(form)
}

// showScanConfigForm shows a form to pick org(s) and profile before running a scan.
func (a *App) showScanConfigForm() {
	ctx := context.Background()
	orgs, err := a.q.ListOrganizations(ctx)
	if err != nil || len(orgs) == 0 {
		return
	}
	profiles, err := a.q.ListProfiles(ctx)
	if err != nil || len(profiles) == 0 {
		return
	}

	selectedOrgSlugs := map[string]bool{}
	selectedProfileName := profiles[0].Name
	for _, p := range profiles {
		if p.IsDefault == 1 {
			selectedProfileName = p.Name
			break
		}
	}

	form := tview.NewForm()
	form.SetBackgroundColor(colorBg)
	form.SetBorderPadding(1, 1, 2, 2)

	for _, o := range orgs {
		slug := o.Slug
		form.AddCheckbox(fmt.Sprintf("Org: %s (%s)", slug, o.Provider), false,
			func(checked bool) {
				if checked {
					selectedOrgSlugs[slug] = true
				} else {
					delete(selectedOrgSlugs, slug)
				}
			})
	}

	profileOpts := make([]string, len(profiles))
	defaultIdx := 0
	for i, p := range profiles {
		label := fmt.Sprintf("%s v%d", p.Name, p.Version)
		if p.IsDefault == 1 {
			label += " [default]"
			defaultIdx = i
		}
		profileOpts[i] = label
	}
	form.AddDropDown("Profile", profileOpts, defaultIdx, func(_ string, idx int) {
		if idx < len(profiles) {
			selectedProfileName = profiles[idx].Name
		}
	})

	form.SetTitle(" New Scan ").SetBorder(true).SetBorderColor(colorSelected)

	helpText := tview.NewTextView().SetDynamicColors(true).
		SetText(markupMuted + "<ctrl+s> start scan   <esc> cancel   <tab> next" + markupReset)
	helpText.SetBackgroundColor(colorHeader)

	formFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(helpText, 1, 0, false)

	startScan := func() {
		var selectedOrgs []dbgen.Organization
		for _, o := range orgs {
			if selectedOrgSlugs[o.Slug] {
				selectedOrgs = append(selectedOrgs, o)
			}
		}
		if len(selectedOrgs) == 0 {
			selectedOrgs = orgs
		}
		var selectedProfile dbgen.ScoringProfile
		for _, p := range profiles {
			if p.Name == selectedProfileName {
				selectedProfile = p
				break
			}
		}
		a.pages.RemovePage("scan-config")
		a.scanView.startScan(selectedOrgs, selectedProfile)
	}

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlS {
			startScan()
			return nil
		}
		if event.Key() == tcell.KeyEsc {
			a.pages.RemovePage("scan-config")
			a.switchTo("scan")
			return nil
		}
		return event
	})

	a.pages.AddPage("scan-config", formFlex, true, true)
	a.tv.SetFocus(form)
}

func acceptFloat(textToCheck string, lastChar rune) bool {
	if lastChar == '.' {
		dots := 0
		for _, c := range textToCheck {
			if c == '.' {
				dots++
			}
		}
		return dots == 1
	}
	return lastChar >= '0' && lastChar <= '9'
}

func acceptInt(_ string, lastChar rune) bool {
	return lastChar >= '0' && lastChar <= '9'
}
