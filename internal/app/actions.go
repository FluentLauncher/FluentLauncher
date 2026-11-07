package app

import (
	"context"
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
		a.settingsPage = 7
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

func (a *App) anyHidden() bool {
	for _, profile := range a.profileMap() {
		if profile.Hidden {
			return true
		}
	}
	return false
}

func (a *App) runWorkspace(workspaceID string) {
	var workspace *settings.Workspace
	for i := range a.settings.Workspaces {
		if strings.EqualFold(a.settings.Workspaces[i].ID, workspaceID) {
			workspace = &a.settings.Workspaces[i]
			break
		}
	}
	if workspace == nil {
		a.notice = locales.WorkspaceDeleted()
		return
	}
	steps := settings.RunWorkspace(*workspace, a.settings.CustomItems, func(item settings.CustomLaunchItem) model.Outcome {
		return apps.Launch(context.Background(), item.ToEntry(), model.Open)
	})
	failed := 0
	for _, step := range steps {
		if !step.Succeeded && !step.Skipped() {
			failed++
		}
	}
	if failed == 0 {
		a.recordUse("workspaces:" + workspace.ID)
		a.hidePanel()
		return
	}
	var details strings.Builder
	for _, step := range steps {
		if !step.Succeeded {
			details.WriteString("\n" + locales.WorkspaceStepFailed(step.Name, step.Error))
		}
	}
	a.notice = locales.WorkspaceRunFailed(workspace.Name, failed, details.String())
	a.hidePanel()
	a.showWorkspaceResult(workspace.Name, steps)
}

func (a *App) showWorkspaceResult(name string, steps []settings.WorkspaceStep) {
	a.workspaceName = name
	a.workspaceSteps = steps
	if a.workspaceWin != nil {
		a.workspaceWin.Close()
	}
	a.workspaceWin = mygo.NewWindow(mygo.WindowOptions{
		Title:             locales.WorkspaceResultTitle(),
		Width:             520,
		Height:            420,
		MinWidth:          420,
		Transparent:       true,
		TitleBarStyle:     mygo.TitleBarHidden,
		HideWindowButtons: true,
		DisableMaximize:   true,
		Content:           ui.View(a.workspaceView),
	})
	a.applyBackdrop(a.displayTheme())
	a.workspaceWin.Show()
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
	for _, item := range a.settings.CustomItems {
		if strings.EqualFold(item.ID, itemID) {
			outcome := apps.Launch(context.Background(), item.ToEntry(), model.Open)
			if outcome.Succeeded {
				a.status = locales.WorkspaceRetryStatus(item.Name)
			} else {
				a.notice = locales.WorkspaceRetryFailed(outcome.Error)
			}
			return
		}
	}
	a.notice = locales.WorkspaceItemDeleted()
}

func (a *App) executePluginAction(entry model.Entry, action model.Action) {
	defer a.panel.Update(func() {})
	switch action.Kind {
	case "copyText":
		mygo.Clipboard.WriteText(action.Value)
		a.hidePanel()
		return
	case "openUrl", "openPath":
		kind := model.File
		if action.Kind == "openUrl" {
			kind = model.URL
		}
		outcome := apps.Launch(context.Background(), model.Entry{
			ID: "plugin-action", Title: action.Title, LaunchTarget: action.Value, Kind: kind,
		}, model.Open)
		if !outcome.Succeeded {
			a.notice = locales.PanelPluginActionFailed(outcome.Error)
			return
		}
		a.hidePanel()
		return
	case "invoke":
		if a.plugins == nil {
			a.notice = locales.PanelPluginRuntimeDisabled()
			return
		}
		if err := a.plugins.Execute(context.Background(), entry, action); err != nil {
			a.notice = locales.PanelPluginActionFailed(err.Error())
			return
		}
		a.status = locales.PanelPluginActionDone(action.Title)
		a.hidePanel()
		return
	}
	a.notice = locales.PanelPluginActionUnsupported()
}
