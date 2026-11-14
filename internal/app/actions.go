package app

import (
	"context"
	"strconv"
	"strings"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/winapi/apps"
	"fluentlauncher/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *App) openActions() {
	entry, ok := a.selected()
	if !ok {
		return
	}
	profile := a.profile(entry.ID)
	a.actions = search.CommandsFor(entry, profile.Pinned, a.anyHidden())
	a.actionsOpen = true
	a.actionIndex = 0
	a.input.CancelFocus()
	a.actionsFocus.Focus()
	a.panel.Update(func() {})
}

func (a *App) hideActions() {
	if !a.actionsOpen {
		return
	}
	a.actionsOpen = false
	a.actions = nil
	a.input.Focus()
	a.panel.Update(func() {})
}

func (a *App) runAction() {
	entry, ok := a.selected()
	if !ok || a.actionIndex < 0 || a.actionIndex >= len(a.actions) {
		return
	}
	command := a.actions[a.actionIndex]
	switch command.Command {
	case search.CommandOpen:
		a.hideActions()
		a.executeSelected(model.Open)
	case search.CommandRunAsAdministrator:
		a.hideActions()
		a.executeSelected(model.RunAsAdministrator)
	case search.CommandOpenLocation:
		a.hideActions()
		a.executeSelected(model.OpenLocation)
	case search.CommandCopyPath:
		a.hideActions()
		a.executeSelected(model.CopyPath)
	case search.CommandCopyValue:
		mygo.Clipboard.WriteText(command.Value)
		a.hideActions()
		a.hidePanel()
	case search.CommandRegenerate:
		a.hideActions()
		a.regenerateToolValue()
	case search.CommandPluginAction:
		for _, action := range entry.Actions {
			if action.ID == command.Value {
				a.executePluginAction(entry, action)
				return
			}
		}
	case search.CommandPluginPreview:
		a.actionsOpen = false
		a.promptOpen = true
		a.promptTitle = locales.PromptPreviewTitle()
		a.promptValue = entry.PreviewText
		a.promptEntry = ""
		a.promptReadOnly = true
		a.input.CancelFocus()
	case search.CommandSetAlias:
		a.actionsOpen = false
		a.promptOpen = true
		a.promptTitle = locales.PromptAliasTitle()
		a.promptValue = a.profile(entry.ID).Alias
		a.promptEntry = entry.ID
		a.promptReadOnly = false
		a.input.CancelFocus()
	case search.CommandTogglePinned:
		a.changeProfile(entry.ID, func(profile model.Profile) model.Profile {
			profile.Pinned = !profile.Pinned
			return profile
		})
		a.hideActions()
	case search.CommandHide:
		a.changeProfile(entry.ID, func(profile model.Profile) model.Profile {
			profile.Hidden = true
			return profile
		})
		a.hideActions()
	case search.CommandRestoreHidden:
		a.settingsPage = pageOther
		a.openSettings()
		a.hideActions()
	}
	a.panel.Update(func() {})
}

func (a *App) savePrompt() {
	if a.promptEntry != "" {
		id := a.promptEntry
		value := a.promptValue
		a.changeProfile(id, func(profile model.Profile) model.Profile {
			profile.Alias = strings.TrimSpace(value)
			return profile
		})
	}
	a.promptOpen = false
	a.promptEntry = ""
	a.promptValue = ""
	a.input.Focus()
	a.panel.Update(func() {})
}

// regenerateToolValue rerolls the selected random tool (uuid/rand/pass) in
// place, keeping the range or length the user asked for, which ride in the
// entry ID. A UUID rerolls in all its format rows at once.
func (a *App) regenerateToolValue() {
	entry, ok := a.selected()
	if !ok {
		return
	}
	var id, value string
	switch {
	case strings.HasPrefix(entry.ID, "uuid:"):
		// Every format row of the UUID takes the new one together.
		if search.RefreshUUIDs(a.results) {
			a.panel.Update(func() {})
		}
		return
	case strings.HasPrefix(entry.ID, "rand:"):
		parts := strings.SplitN(entry.ID, ":", 3)
		if len(parts) != 3 {
			return
		}
		bounds := strings.SplitN(parts[1], "-", 2)
		lo, err1 := strconv.Atoi(bounds[0])
		hi, err2 := strconv.Atoi(bounds[1])
		if len(bounds) != 2 || err1 != nil || err2 != nil {
			return
		}
		id, value = "rand:"+parts[1]+":", search.RandInRange(lo, hi)
	case strings.HasPrefix(entry.ID, "pass:"):
		parts := strings.SplitN(entry.ID, ":", 3)
		if len(parts) != 3 {
			return
		}
		length, err := strconv.Atoi(parts[1])
		if err != nil || length < 4 || length > 64 {
			return
		}
		id, value = "pass:"+parts[1]+":", search.RandomPassword(length)
	default:
		return
	}
	if value == "" {
		return
	}
	a.results[a.chosen].Entry.ID = id + value
	a.results[a.chosen].Entry.Title = value
	a.results[a.chosen].Entry.CopyText = value
	a.panel.Update(func() {})
}

func (a *App) anyHidden() bool {
	return a.hiddenCount() > 0
}

func (a *App) runWorkspace(workspaceID string) {
	snap := a.currentSettings()
	var workspace *settings.Workspace
	for i := range snap.Workspaces {
		if strings.EqualFold(snap.Workspaces[i].ID, workspaceID) {
			workspace = &snap.Workspaces[i]
			break
		}
	}
	if workspace == nil {
		a.notice = locales.WorkspaceDeleted()
		return
	}
	workspaceCopy := *workspace
	a.hidePanel()
	// The steps launch programs one after another and each can wait on a
	// UAC prompt; the UI thread only hears the outcome.
	go func() {
		steps := settings.RunWorkspace(workspaceCopy, snap.CustomItems, func(item settings.CustomLaunchItem) model.Outcome {
			return apps.Launch(context.Background(), item.ToEntry(), model.Open)
		})
		failed := 0
		for _, step := range steps {
			if !step.Succeeded && !step.Skipped() {
				failed++
			}
		}
		if a.panel == nil {
			return
		}
		a.panel.Update(func() {
			if failed == 0 {
				a.recordUse("workspaces:" + workspaceCopy.ID)
				return
			}
			var details strings.Builder
			for _, step := range steps {
				if !step.Succeeded {
					details.WriteString("\n" + locales.WorkspaceStepFailed(step.Name, step.Error))
				}
			}
			a.notice = locales.WorkspaceRunFailed(workspaceCopy.Name, failed, details.String())
			a.showWorkspaceResult(workspaceCopy.Name, steps)
		})
	}()
}

func (a *App) showWorkspaceResult(name string, steps []settings.WorkspaceStep) {
	a.workspaceName = name
	a.workspaceSteps = steps
	old := a.workspaceWin
	a.workspaceWin = mygo.NewWindow(mygo.WindowOptions{
		Hidden:            true,
		Title:             locales.WorkspaceResultTitle(),
		Width:             520,
		Height:            420,
		MinWidth:          420,
		Transparent:       true,
		Vibrancy:          mygo.VibrancyMica,
		TitleBarStyle:     mygo.TitleBarHidden,
		HideWindowButtons: true,
		DisableMaximize:   true,
		Content:           ui.View(a.workspaceView),
	})
	win := a.workspaceWin
	// The close button and the user both destroy the window without telling
	// the app; without this the theme and locale passes would keep poking a
	// dead handle.
	win.OnClose(func(*mygo.CloseEvent) {
		if a.workspaceWin == win {
			a.workspaceWin = nil
		}
	})
	if old != nil {
		old.Close()
	}
	a.applyBackdrop(a.displayTheme())
	win.Show()
}

func (a *App) workspaceView(c *ui.Context) {
	theme := *c.Theme()
	a.applyUITheme(&theme)
	c.SetTheme(&theme)
	a.drawShell(c, &theme, func() {
		a.captionBar(c, &theme, locales.WorkspaceResultTitle(), a.workspaceWin)
		ui.Column(c).Grow(1).FillWidth().Padding(16).Gap(10).Children(func() {
			ui.Text(c, a.workspaceName).FontSize(18).Bold().TextColor(theme.Text)
			for i := range a.workspaceSteps {
				step := &a.workspaceSteps[i]
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					status := "✓"
					colorValue := theme.Success
					if !step.Succeeded {
						status = "×"
						colorValue = theme.Danger
					}
					ui.Text(c, status).TextColor(colorValue).Width(20)
					ui.Text(c, step.Name).Grow(1).SingleLine()
					if !step.Succeeded {
						ui.Text(c, step.Error).FontSize(11).TextColor(theme.TextMuted).SingleLine()
						if compactButton(c, locales.CommonRetry()).Clicked() {
							a.retryWorkspaceItem(step.ItemID)
						}
					}
				})
			}
			if compactButton(c, locales.CommonClose()).Clicked() {
				a.workspaceWin.Close()
			}
		})
	})
}

func (a *App) retryWorkspaceItem(itemID string) {
	snap := a.currentSettings()
	var item *settings.CustomLaunchItem
	for i := range snap.CustomItems {
		if strings.EqualFold(snap.CustomItems[i].ID, itemID) {
			item = &snap.CustomItems[i]
			break
		}
	}
	if item == nil {
		a.notice = locales.WorkspaceItemDeleted()
		return
	}
	itemCopy := *item
	go func() {
		outcome := apps.Launch(context.Background(), itemCopy.ToEntry(), model.Open)
		if a.panel == nil {
			return
		}
		a.panel.Update(func() {
			if outcome.Succeeded {
				a.status = locales.WorkspaceRetryStatus(itemCopy.Name)
			} else {
				a.notice = locales.WorkspaceRetryFailed(outcome.Error)
			}
		})
	}()
}

func (a *App) executePluginAction(entry model.Entry, action model.Action) {
	// Launches and plugin RPCs wait on processes; only their outcomes come
	// back to the UI thread.
	switch action.Kind {
	case "copyText":
		mygo.Clipboard.WriteText(action.Value)
		a.hidePanel()
		a.panel.Update(func() {})
		return
	case "openUrl", "openPath":
		kind := model.File
		if action.Kind == "openUrl" {
			kind = model.URL
		}
		target := model.Entry{ID: "plugin-action", Title: action.Title, LaunchTarget: action.Value, Kind: kind}
		a.hidePanel()
		go func() {
			outcome := apps.Launch(context.Background(), target, model.Open)
			if a.panel == nil {
				return
			}
			a.panel.Update(func() {
				if !outcome.Succeeded {
					a.notice = locales.PanelPluginActionFailed(outcome.Error)
				}
			})
		}()
		return
	case "invoke":
		if a.plugins == nil {
			a.notice = locales.PanelPluginRuntimeDisabled()
			a.panel.Update(func() {})
			return
		}
		a.hidePanel()
		go func() {
			err := a.plugins.Execute(context.Background(), entry, action)
			if a.panel == nil {
				return
			}
			a.panel.Update(func() {
				if err != nil {
					a.notice = locales.PanelPluginActionFailed(err.Error())
					return
				}
				a.status = locales.PanelPluginActionDone(action.Title)
			})
		}()
		return
	}
	a.notice = locales.PanelPluginActionUnsupported()
	a.panel.Update(func() {})
}
