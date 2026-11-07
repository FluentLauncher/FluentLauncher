// Package app wires the launcher's sources, search, views and Windows
// integrations together.
package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mistquay/fluentlauncher/internal/core/layout"
	"github.com/mistquay/fluentlauncher/internal/core/model"
	"github.com/mistquay/fluentlauncher/internal/core/search"
	"github.com/mistquay/fluentlauncher/internal/core/settings"
	"github.com/mistquay/fluentlauncher/internal/diag"
	"github.com/mistquay/fluentlauncher/internal/plugin"
	"github.com/mistquay/fluentlauncher/internal/store"
	"github.com/mistquay/fluentlauncher/internal/winapi/apps"
	"github.com/mistquay/fluentlauncher/internal/winapi/everything"
	"github.com/mistquay/fluentlauncher/internal/winapi/volume"
	"github.com/mistquay/fluentlauncher/internal/winapi/windowlist"
)

type App struct {
	dataDir string

	config     *store.ConfigStore
	themes     *store.ThemeStore
	db         *store.DB
	settings   settings.LauncherSettings
	profiles   map[string]model.Profile
	profilesMu sync.Mutex

	appEngine       *search.Engine
	fileEngine      *search.Engine
	customEngine    *search.Engine
	workspaceEngine *search.Engine

	catalog    *apps.Catalog
	fileIndex  *store.FileIndex
	everything *everything.Provider
	pluginRepo *plugin.Repository
	plugins    *plugin.Manager
	providers  []search.Provider

	panel          *mygo.Window
	settingsWin    *mygo.Window
	workspaceWin   *mygo.Window
	workspaceName  string
	workspaceSteps []settings.WorkspaceStep
	tray           *mygo.Tray
	volumeWin      *mygo.Window
	volume         *volume.Controller
	volumePct      int

	input ui.Handle
	list  ui.ListState

	query         string
	results       []model.Result
	chosen        int
	selectedID    string
	resultsReady  bool
	notice        string
	status        string
	composing     bool
	settingsError string
	loading       bool
	lastDiscovery time.Time

	icons       map[string]*ui.Bitmap
	iconLoading map[string]bool
	iconSize    int
	mem         string
	background  *ui.Bitmap

	settingsPage  int
	settingsDraft settings.LauncherSettings

	hotkeyActive string
	hotkeyNotice string
	pendingOpen  bool

	settingsHover int
	listHover     int
	listHoverNext int
	brandIcon     *ui.Bitmap

	selectOpen    string
	lastPreview   settings.ThemeSettings
	blurSlider    float64
	blurSliderInt int

	actionsOpen      bool
	actions          []search.CommandItem
	actionIndex      int
	actionsFocus     ui.Handle
	actionsHover     int
	actionsHoverNext int

	promptOpen     bool
	promptTitle    string
	promptValue    string
	promptEntry    string
	promptReadOnly bool

	quitting bool

	queryCancel     context.CancelFunc
	queryGeneration int64
	catalogTimer    *time.Timer
	volumeTimer     *time.Timer
}

// Run starts the single-instance launcher.
func Run() error {
	diag.Init()
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	a := &App{}
	data := filepath.Join(os.Getenv("LOCALAPPDATA"), "FluentLauncher")
	mygo.App.SetName("FluentLauncher")
	mygo.App.SetVersion("0.1.0")
	mygo.App.SetPath(mygo.PathUserData, data)
	mygo.App.SetPath(mygo.PathCache, filepath.Join(data, "cache"))
	mygo.App.SetPath(mygo.PathLogs, filepath.Join(data, "logs"))
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnSecondInstance(func([]string, string) {
		if a.panel != nil {
			a.show()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.close() })
	mygo.App.WhenReady(a.ready)
	return mygo.App.Run()
}

func (a *App) ready() {
	diag.Mark("ready")
	a.dataDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "FluentLauncher")
	a.icons = map[string]*ui.Bitmap{}
	a.iconLoading = map[string]bool{}
	a.profiles = map[string]model.Profile{}
	a.settings = settings.Default()
	a.appEngine = search.NewEngine()
	a.fileEngine = search.NewEngine()
	a.customEngine = search.NewEngine()
	a.workspaceEngine = search.NewEngine()

	a.config = store.NewConfigStore(a.dataDir)
	a.themes = store.NewThemeStore(filepath.Join(a.dataDir, "themes"))
	if value, err := a.config.Load(); err != nil {
		a.notice = "无法读取设置，使用默认配置：" + err.Error()
	} else {
		a.settings = value
	}
	a.settingsDraft = a.settings

	a.loadDatabase()
	a.catalog = apps.NewCatalog(a.settings.Applications)
	a.catalog.SetChanged(a.catalogChanged)
	a.fileIndex = store.NewFileIndex(a.db)
	a.fileIndex.SetOnUpdate(func(entries []model.Entry) {
		if a.panel != nil {
			a.panel.Update(func() {
				a.fileEngine.ReplaceEntries(entries, a.profileMap())
				a.updateResults()
			})
		}
	})
	a.everything = &everything.Provider{Enabled: func() bool { return a.settings.Files.EnableEverything }}
	a.pluginRepo = plugin.NewRepository(filepath.Join(a.dataDir, "plugins"))
	_ = a.pluginRepo.Refresh()
	a.plugins = plugin.NewManager()
	a.plugins.Configure(a.pluginRepo.Installed(), a.settings.Plugins)
	log.Printf("plugins installed=%d enabled=%v", len(a.pluginRepo.Installed()), a.settings.Plugins.EnabledIDs)

	a.iconSize = 28
	for _, display := range mygo.Screen.Displays() {
		a.iconSize = max(a.iconSize, int(float64(28)*display.ScaleFactor))
	}
	a.rebuildCustom()
	a.providers = []search.Provider{
		indexedProvider{id: "applications", engine: a.appEngine, scope: search.ScopeApplications, bonus: 500},
		indexedProvider{id: "files", engine: a.fileEngine, scope: search.ScopeFiles},
		indexedProvider{id: settings.CustomSourceID, engine: a.customEngine, scope: search.ScopeApplications},
		indexedProvider{id: settings.WorkspaceSourceID, engine: a.workspaceEngine, scope: search.ScopeApplications},
		search.UtilityProvider{Settings: a.currentSettings},
		search.SiteSearchProvider{Settings: a.currentSettings},
		search.LauncherCommands{},
		windowProvider{},
		a.everything,
		a.plugins,
	}

	a.panel = mygo.NewWindow(mygo.WindowOptions{
		Panel:         true,
		Hidden:        true,
		Frameless:     true,
		Transparent:   true,
		Vibrancy:      a.vibrancy(),
		CornerRadius:  16,
		Width:         a.settings.Width,
		Height:        a.settings.Height,
		DisableResize: true,
		Content:       ui.View(a.view),
	})
	a.panel.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		a.hidePanel()
	})
	a.panel.OnBlur(a.hidePanel)
	a.panel.OnShow(func() { log.Printf("panel shown") })

	a.createVolumeOverlay()
	a.setUpTray()
	a.applyTheme()
	a.registerHotkey()
	a.applyVolumeSettings()
	a.applyOpenAtLogin()
	a.applyWindowSize()

	diag.Mark("windows ready")
	a.loadFileCache()
	a.refreshApps()
	a.catalog.Watch(context.Background())
	if !a.settings.HideOnStartup && !mygo.App.WasOpenedAtLogin() {
		a.show()
	}
	if query := os.Getenv("FL_DEBUG_QUERY"); query != "" {
		a.query = query
		a.updateResults()
		a.panel.Update(func() {})
	}
	if os.Getenv("FL_DEBUG_SHOW") == "1" {
		a.show()
	}
	if os.Getenv("FL_DEBUG_OPEN_SETTINGS") == "1" {
		a.openSettings()
	}
	if os.Getenv("FL_DEBUG_TRAY_MENU") == "1" {
		time.AfterFunc(2*time.Second, func() {
			a.panel.Update(func() {
				if a.tray != nil {
					a.tray.PopUpMenu()
				}
			})
		})
	}
	if value := os.Getenv("FL_DEBUG_EXIT_MS"); value != "" {
		if milliseconds, err := strconv.Atoi(value); err == nil && milliseconds > 0 {
			time.AfterFunc(time.Duration(milliseconds)*time.Millisecond, mygo.App.Quit)
		}
	}
	go a.memLoop()
}

func (a *App) loadDatabase() {
	path := filepath.Join(a.dataDir, "data", "launcher.db")
	db, err := store.OpenDB(context.Background(), path)
	if err != nil {
		a.notice = "无法打开数据库：" + err.Error()
		return
	}
	a.db = db
	profiles, err := db.LoadProfiles(context.Background())
	if err != nil {
		a.notice = "无法读取个人规则：" + err.Error()
		return
	}
	a.profiles = profiles
}

func (a *App) currentSettings() settings.LauncherSettings {
	return a.settings
}

func (a *App) profileMap() map[string]model.Profile {
	a.profilesMu.Lock()
	defer a.profilesMu.Unlock()
	out := make(map[string]model.Profile, len(a.profiles))
	for key, profile := range a.profiles {
		out[key] = profile
	}
	return out
}

func (a *App) profileSlice() []model.Profile {
	a.profilesMu.Lock()
	defer a.profilesMu.Unlock()
	out := make([]model.Profile, 0, len(a.profiles))
	for _, profile := range a.profiles {
		out = append(out, profile)
	}
	return out
}

func (a *App) rebuildCustom() {
	profiles := a.profileMap()
	items := make([]model.Entry, 0, len(a.settings.CustomItems))
	for _, item := range a.settings.CustomItems {
		if item.Enabled {
			items = append(items, item.ToEntry())
		}
	}
	workspaces := make([]model.Entry, 0, len(a.settings.Workspaces))
	for _, workspace := range a.settings.Workspaces {
		if workspace.Enabled {
			workspaces = append(workspaces, workspace.ToEntry(len(workspace.ItemIDs)))
		}
	}
	a.customEngine.ReplaceEntries(items, profiles)
	a.workspaceEngine.ReplaceEntries(workspaces, profiles)
}

func (a *App) loadFileCache() {
	if a.db == nil {
		return
	}
	cached, err := a.fileIndex.LoadCached(context.Background(), a.settings.Files)
	if err != nil {
		a.notice = "无法读取文件索引：" + err.Error()
		return
	}
	a.fileEngine.ReplaceEntries(cached, a.profileMap())
	go func() {
		if err := a.fileIndex.Configure(context.Background(), a.settings.Files); err != nil {
			log.Printf("file index: %v", err)
		}
	}()
}

func (a *App) refreshApps() {
	if a.loading {
		return
	}
	a.loading = true
	start := time.Now()
	go func() {
		entries, err := a.catalog.Discover(context.Background())
		ms := time.Since(start).Milliseconds()
		if a.panel == nil {
			return
		}
		a.panel.Update(func() {
			a.loading = false
			a.lastDiscovery = time.Now()
			if err != nil {
				a.notice = "应用发现失败：" + err.Error()
				return
			}
			a.appEngine.ReplaceEntries(entries, a.profileMap())
			a.status = fmt.Sprintf("已发现 %d 个应用", len(entries))
			log.Printf("apps: %d in %dms", len(entries), ms)
			a.updateResults()
		})
	}()
}

func (a *App) catalogChanged() {
	if a.catalogTimer != nil {
		a.catalogTimer.Stop()
	}
	a.catalogTimer = time.AfterFunc(2*time.Second, a.refreshApps)
}

func (a *App) refreshIfStale() {
	if time.Since(a.lastDiscovery) > time.Hour {
		a.refreshApps()
	}
}

func (a *App) show() {
	a.positionPanel()
	a.query = ""
	a.notice = ""
	a.pendingOpen = false
	a.hideActions()
	a.input.Focus()
	a.panel.Show()
	a.refreshIfStale()
	if len(a.results) == 0 {
		a.updateResults()
	}
}

func (a *App) hidePanel() {
	a.panel.Hide()
}

func (a *App) toggle() {
	if a.panel.IsVisible() {
		a.hidePanel()
	} else {
		a.show()
	}
}

func (a *App) positionPanel() {
	display := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	switch a.settings.Placement {
	case settings.PlacementPrimaryScreen:
		display = mygo.Screen.PrimaryDisplay()
	case settings.PlacementMouseScreen:
		display = mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	}
	rect := layout.Calculate(layout.WorkArea{
		X: display.WorkArea.X, Y: display.WorkArea.Y,
		Width: display.WorkArea.Width, Height: display.WorkArea.Height,
	}, 1, float64(a.settings.Width), float64(a.settings.Height), nil)
	a.panel.SetBounds(mygo.Rectangle{X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height})
}

func (a *App) applyWindowSize() {
	a.panel.SetSize(a.settings.Width, a.settings.Height)
}

func (a *App) setUpTray() {
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: trayIconPNG(), ToolTip: "FluentLauncher"})
	if err != nil {
		log.Printf("tray: %v", err)
		return
	}
	a.tray = tray
	a.tray.SetMenu(a.trayMenu())
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.stopVolume()
		tray.Destroy()
	})
}

// trayMenu builds the notification area menu. Checkable items are plain
// rows whose label states the value: a check box would reserve a mark
// column that leaves an empty gutter beside every item.
func (a *App) trayMenu() *mygo.Menu {
	loginLabel := "开机启动：关"
	if a.settings.RunAtLogin {
		loginLabel = "开机启动：开"
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "显示搜索", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
		{Label: "设置", Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
		mygo.Separator(),
		{Label: loginLabel, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.settings.RunAtLogin = !a.settings.RunAtLogin
			_ = a.config.Save(a.settings)
			a.settingsDraft = a.settings
			a.applyOpenAtLogin()
			if a.tray != nil {
				a.tray.SetMenu(a.trayMenu())
			}
		}},
		mygo.Separator(),
		{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

func (a *App) registerHotkey() {
	mygo.GlobalShortcut.UnregisterAll()
	a.hotkeyActive = ""
	for _, shortcut := range []string{a.settings.Hotkey, "Alt+Shift+Space", "Ctrl+Alt+Space"} {
		if shortcut == "" {
			continue
		}
		err := mygo.GlobalShortcut.Register(shortcut, a.toggle)
		if err == nil {
			log.Printf("hotkey: %s", shortcut)
			a.hotkeyActive = shortcut
			if shortcut != a.settings.Hotkey {
				a.hotkeyNotice = fmt.Sprintf("热键 %s 被其他程序占用，已改用 %s。", a.settings.Hotkey, shortcut)
			} else {
				a.hotkeyNotice = ""
			}
			return
		}
		log.Printf("hotkey %s: %v", shortcut, err)
	}
	a.hotkeyActive = ""
	a.hotkeyNotice = "全局热键全部被占用，请在设置中更换。"
}

func (a *App) applyOpenAtLogin() {
	if err := mygo.App.SetOpenAtLogin(a.settings.RunAtLogin); err != nil {
		log.Printf("open at login: %v", err)
	}
}

func (a *App) memLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if a.panel == nil {
			return
		}
		m := diag.Mem()
		a.panel.Update(func() { a.mem = m })
	}
}

// profileMapFor is used by callers that need a stable copy.

// applySettings persists and applies the whole configuration.
func (a *App) applySettings(value settings.LauncherSettings) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if err := a.config.Save(value); err != nil {
		return err
	}
	a.settings = value
	a.settingsDraft = value
	a.lastPreview = value.Theme
	a.catalog.SetOptions(value.Applications)
	if a.plugins != nil {
		a.plugins.Configure(a.pluginRepo.Installed(), value.Plugins)
	}
	a.rebuildCustom()
	a.applyTheme()
	a.applyWindowSize()
	a.positionPanel()
	a.registerHotkey()
	a.applyVolumeSettings()
	a.applyOpenAtLogin()
	if a.db != nil {
		go func() {
			if err := a.fileIndex.Configure(context.Background(), value.Files); err != nil {
				log.Printf("file index: %v", err)
			}
		}()
	}
	a.updateResults()
	return nil
}

func (a *App) saveTheme(value settings.ThemeSettings) error {
	if err := a.config.SaveTheme(value); err != nil {
		return err
	}
	a.settings.Theme = value
	a.settingsDraft.Theme = value
	a.applyTheme()
	a.panel.Update(func() {})
	return nil
}

// applyTheme switches the system appearance, window material and image.
func (a *App) applyTheme() {
	a.applyThemeFrom(a.settings.Theme)
}

// displayTheme is what the views draw: while the settings window shows,
// the draft, so appearance edits preview live; otherwise the saved theme.
func (a *App) displayTheme() settings.ThemeSettings {
	if a.settingsWin != nil && a.settingsWin.IsVisible() {
		return a.settingsDraft.Theme
	}
	return a.settings.Theme
}

// applyThemeFrom applies a theme value to both windows without persisting
// it, which the appearance page uses to preview the draft.
func (a *App) applyThemeFrom(value settings.ThemeSettings) {
	switch value.Mode {
	case settings.ThemeLight:
		mygo.Theme.SetSource(mygo.ThemeLight)
	case settings.ThemeDark:
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
	vibrancy := a.vibrancyOf(value)
	if a.panel != nil {
		a.panel.SetVibrancy(vibrancy)
	}
	if a.settingsWin != nil {
		a.settingsWin.SetVibrancy(vibrancy)
	}
	if value.Background == settings.BackgroundImage && value.ImagePath != "" {
		if data, err := os.ReadFile(value.ImagePath); err == nil {
			if bitmap, err := ui.DecodeBitmap(data); err == nil {
				a.background = bitmap
			}
		}
	} else {
		a.background = nil
	}
	if a.panel != nil {
		a.panel.Update(func() {})
	}
	if a.settingsWin != nil {
		a.settingsWin.Update(func() {})
	}
}

func (a *App) vibrancy() mygo.Vibrancy {
	return a.vibrancyOf(a.displayTheme())
}

func (a *App) vibrancyOf(value settings.ThemeSettings) mygo.Vibrancy {
	switch value.Background {
	case settings.BackgroundMica:
		return mygo.VibrancyMica
	case settings.BackgroundAcrylic:
		return mygo.VibrancyAcrylic
	default:
		return mygo.VibrancyNone
	}
}

// recordUse updates the personal profile and the search snapshots.
func (a *App) recordUse(id string) {
	a.profilesMu.Lock()
	profile := a.profiles[id]
	profile.EntryID = id
	profile.UseCount++
	now := time.Now()
	profile.LastUsedAt = &now
	a.profiles[id] = profile
	a.profilesMu.Unlock()
	if a.db != nil {
		_ = a.db.SaveProfile(context.Background(), profile)
	}
	a.appEngine.UpdateProfile(profile)
	a.fileEngine.UpdateProfile(profile)
	a.customEngine.UpdateProfile(profile)
	a.workspaceEngine.UpdateProfile(profile)
}

func (a *App) changeProfile(id string, update func(model.Profile) model.Profile) {
	a.profilesMu.Lock()
	profile := update(a.profiles[id])
	profile.EntryID = id
	a.profiles[id] = profile
	a.profilesMu.Unlock()
	if a.db != nil {
		_ = a.db.SaveProfile(context.Background(), profile)
	}
	a.appEngine.UpdateProfile(profile)
	a.fileEngine.UpdateProfile(profile)
	a.customEngine.UpdateProfile(profile)
	a.workspaceEngine.UpdateProfile(profile)
	a.updateResults()
}

func (a *App) profile(id string) model.Profile {
	a.profilesMu.Lock()
	defer a.profilesMu.Unlock()
	if profile, ok := a.profiles[id]; ok {
		return profile
	}
	return model.Profile{EntryID: id}
}

func (a *App) findEntry(id string) (model.Entry, bool) {
	for _, engine := range []*search.Engine{a.appEngine, a.customEngine, a.workspaceEngine, a.fileEngine} {
		if entry, ok := engine.Find(id); ok {
			return entry, true
		}
	}
	return model.Entry{}, false
}

func (a *App) updateResults() {
	if a.panel == nil {
		return
	}
	if a.queryCancel != nil {
		a.queryCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.queryCancel = cancel
	a.queryGeneration++
	generation := a.queryGeneration
	a.resultsReady = false
	preferred := a.selectedID

	if strings.TrimSpace(a.query) == "" {
		results := search.RecentEntries(a.settings.ShowRecentEntries, a.profileSlice(), a.findEntry, a.settings.MaxResults)
		a.setResults(results, preferred)
		a.resultsReady = true
		a.panel.Update(func() {})
		return
	}
	q := search.ParseQuery(a.query)
	log.Printf("query %q scope=%d", q.Text, q.Scope)
	go func() {
		batches := make([]search.Batch, 0, len(a.providers))
		for batch := range search.Run(ctx, a.providers, q, a.settings.MaxResults) {
			if generation != a.queryGeneration || ctx.Err() != nil {
				return
			}
			batches = append(batches, batch)
			matches := search.Merge(batches, a.settings.MaxResults)
			if len(matches) == 0 {
				continue
			}
			a.panel.Update(func() {
				a.setResults(matches, preferred)
				a.resultsReady = true
				a.status = a.statusLine(batches)
			})
		}
		a.panel.Update(func() {
			if generation != a.queryGeneration {
				return
			}
			if len(batches) == 0 {
				a.setResults(nil, preferred)
			}
			a.resultsReady = true
			a.status = a.statusLine(batches)
		})
	}()
}

func (a *App) setResults(results []model.Result, preferred string) {
	a.results = results
	a.chosen = 0
	if preferred != "" {
		for i, result := range results {
			if result.Entry.ID == preferred {
				a.chosen = i
				break
			}
		}
	}
	if len(results) == 0 {
		a.selectedID = ""
	} else {
		a.selectedID = results[a.chosen].Entry.ID
		a.list.ScrollIntoView(a.chosen)
	}
}

func (a *App) statusLine(batches []search.Batch) string {
	parts := []string{}
	if a.status != "" {
		parts = append(parts, a.status)
	}
	parts = append(parts, fmt.Sprintf("当前显示 %d 条", len(a.results)))
	for _, batch := range batches {
		if batch.Error != "" {
			parts = append(parts, batch.SourceID+": "+batch.Error)
		}
	}
	if warning := a.catalog.Warning(); warning != "" {
		parts = append(parts, warning)
	}
	if a.fileIndex != nil {
		if warning := a.fileIndex.Warning(); warning != "" {
			parts = append(parts, warning)
		}
	}
	return strings.Join(parts, " · ")
}

func (a *App) choose(offset int) {
	if len(a.results) == 0 {
		return
	}
	a.chosen = min(max(a.chosen+offset, 0), len(a.results)-1)
	a.selectedID = a.results[a.chosen].Entry.ID
	a.list.ScrollIntoView(a.chosen)
}

func (a *App) selected() (model.Entry, bool) {
	if a.chosen < 0 || a.chosen >= len(a.results) {
		return model.Entry{}, false
	}
	return a.results[a.chosen].Entry, true
}

func (a *App) executeSelected(action model.EntryAction) {
	if !a.resultsReady {
		return
	}
	entry, ok := a.selected()
	if !ok {
		return
	}
	if action == model.Open && entry.ID == search.SettingsID {
		a.openSettings()
		return
	}
	if action == model.Open && entry.Kind == model.Workspace {
		a.runWorkspace(entry.LaunchTarget)
		return
	}
	if action == model.CopyPath {
		if entry.LaunchTarget == "" {
			a.notice = "此条目没有可复制的路径"
			return
		}
		mygo.Clipboard.WriteText(entry.LaunchTarget)
		a.hidePanel()
		return
	}
	if action == model.CopyText || (action == model.Open && entry.Kind == model.Calculation) {
		text := entry.CopyText
		if text == "" {
			text = entry.LaunchTarget
		}
		mygo.Clipboard.WriteText(text)
		a.hidePanel()
		return
	}
	if entry.Kind == model.Plugin {
		if len(entry.Actions) == 0 {
			a.notice = "该插件条目没有可执行动作"
			return
		}
		a.executePluginAction(entry, entry.Actions[0])
		return
	}
	outcome := apps.Launch(context.Background(), entry, action)
	if outcome.Succeeded {
		if action == model.Open || action == model.RunAsAdministrator {
			a.recordUse(entry.ID)
		}
		a.hidePanel()
		return
	}
	if outcome.Canceled {
		a.notice = "已取消"
		return
	}
	a.notice = fmt.Sprintf("无法打开“%s”：%s", entry.Title, outcome.Error)
	a.updateResults()
}

func (a *App) openSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	a.settingsDraft = cloneSettings(a.settings)
	a.lastPreview = a.settingsDraft.Theme
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:     "FluentLauncher 设置",
		Width:     900,
		Height:    660,
		MinWidth:  720,
		MinHeight: 520,
		Vibrancy:  a.vibrancy(),
		StateKey:  "settings",
		Content:   ui.View(a.settingsView),
	})
	a.settingsWin.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		a.settingsWin.Hide()
	})
	if icon := windowIconPNG(); icon != nil {
		if err := a.settingsWin.SetIcon(icon); err != nil {
			log.Printf("settings icon: %v", err)
		}
	}
	a.settingsWin.Show()
}

func (a *App) close() {
	if a.queryCancel != nil {
		a.queryCancel()
	}
	if a.db != nil {
		a.db.Close()
	}
	if a.fileIndex != nil {
		a.fileIndex.Close()
	}
	if a.catalog != nil {
		a.catalog.Close()
	}
	if a.plugins != nil {
		a.plugins.Dispose()
	}
}

// indexedProvider adapts one in-memory engine to the provider protocol.
type indexedProvider struct {
	id     string
	engine *search.Engine
	scope  search.Scope
	bonus  int
}

func (p indexedProvider) ID() string { return p.id }

func (p indexedProvider) Supports(q search.Query) bool {
	return q.Scope == p.scope || q.Scope == search.ScopeAll
}

func (p indexedProvider) Search(ctx context.Context, q search.Query, limit int) ([]model.Result, error) {
	results := p.engine.Search(ctx, q.Text, limit)
	if p.scope == search.ScopeApplications && p.bonus != 0 {
		for i := range results {
			results[i].Score += p.bonus
		}
	}
	return results, nil
}

type windowProvider struct{}

func (windowProvider) ID() string { return "windows" }

func (windowProvider) Supports(q search.Query) bool { return q.Scope == search.ScopeWindows }

func (windowProvider) Search(ctx context.Context, q search.Query, limit int) ([]model.Result, error) {
	entries, err := windowlist.List(ctx)
	if err != nil {
		return nil, err
	}
	engine := search.NewEngine()
	engine.ReplaceEntries(entries, nil)
	results := engine.Search(ctx, q.Text, limit)
	for i := range results {
		results[i].Score++
	}
	return results, nil
}
