// Package app wires the launcher's sources, search, views and Windows
// integrations together.
package app

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/bundled"
	"fluentlauncher/internal/core/layout"
	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/core/shortcuts"
	"fluentlauncher/internal/diag"
	"fluentlauncher/internal/plugin"
	"fluentlauncher/internal/store"
	"fluentlauncher/internal/winapi/apps"
	"fluentlauncher/internal/winapi/everything"
	"fluentlauncher/internal/winapi/glass"
	"fluentlauncher/internal/winapi/volume"
	"fluentlauncher/internal/winapi/windowlist"
	"fluentlauncher/locales"
)

type App struct {
	dataDir string
	// workArea is the primary display's work area in DIPs, which the size
	// preview draws the panel on.
	workArea layout.WorkArea

	config       *store.ConfigStore
	themes       *store.ThemeStore
	db           *store.DB
	settings     settings.LauncherSettings
	settingsSnap atomic.Pointer[settings.LauncherSettings]
	profiles     map[string]model.Profile
	profilesMu   sync.Mutex

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
	// volumePinned keeps the debug overlay on screen, so that a build can be
	// checked without a wheel gesture.
	volumePinned bool

	input ui.Handle
	list  ui.ListState

	query          string
	results        []model.Result
	chosen         int
	selectedID     string
	resultsReady   bool
	notice         string
	status         string
	composing      bool
	settingsError  string
	settingsNotice string
	loading        atomic.Bool
	lastDiscovery  time.Time

	icons           map[string]*ui.Bitmap
	iconLoading     map[string]bool
	iconSize        int
	iconPixels      int
	scaleFactor     float64
	mem             string
	background      *ui.Bitmap
	backgroundImage backgroundLoader

	settingsPage   int
	settingsDraft  settings.LauncherSettings
	toolFilter     string
	toolRowHandles map[string]*ui.Handle
	everythingBusy bool
	appliedTheme   settings.ThemeSettings

	globalShortcuts   shortcuts.Registry
	shortcutEdit      shortcutEditor
	shortcutFilter    string
	pluginCommandBusy map[string]bool

	hotkeyActive string
	hotkeyNotice string
	pendingOpen  bool

	settingsHover int
	listHover     int
	listHoverNext int
	brandIcon     *ui.Bitmap

	selectOpen        string
	officialPick      string
	official          officialState
	lastPreview       settings.ThemeSettings
	blurSlider        float64
	blurSliderInt     int
	sizeDragging      bool
	sizeDragWidth     float64
	sizeDragHeight    float64
	sizeDragPanelX    float32
	sizeDragPanelY    float32
	trayClickAt       time.Time
	anchorTimer       *time.Timer
	settingsSaveTimer *time.Timer

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

	quitting     bool
	updates      updateState
	updateCancel context.CancelFunc

	queryCancel     context.CancelFunc
	fileIndexCancel context.CancelFunc
	queryGeneration atomic.Int64
	lastBatches     []search.Batch
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
	library, err := bundled.SQLiteLibrary(filepath.Join(data, "cache"))
	if err != nil {
		return err
	}
	if err := os.Setenv("MYGO_SQLITE_LIBRARY", library); err != nil {
		return err
	}
	mygo.App.SetName("FluentLauncher")
	// Follow the system's languages until the settings load and apply the
	// user's choice; the language also picks the fonts the text system draws
	// Han characters with, and the date and number formats.
	mygo.App.SetLocale("")
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
		a.notice = locales.AppCannotReadSettings(err)
	} else {
		a.settings = value
		if value.UpdateChannel == settings.UpdateChannelDefault {
			// Resolve once so a nightly user stays on preview updates even
			// after installing the next stable release.
			a.settings.UpdateChannel = value.EffectiveUpdateChannel(appVersion())
			if err := a.config.Save(a.settings); err != nil {
				a.notice = err.Error()
			}
		}
	}
	a.settingsDraft = a.settings
	a.publishSettings()
	a.applyLanguage()
	// Views rebuild themselves when the language changes; what the app set
	// once — window titles and the tray menu — it sets again here.
	mygo.App.OnLocaleChanged(func(string) {
		if a.settingsWin != nil {
			a.settingsWin.SetTitle(locales.SettingsWindowTitle())
		}
		if a.workspaceWin != nil {
			a.workspaceWin.SetTitle(locales.WorkspaceResultTitle())
		}
		if a.tray != nil {
			a.tray.SetMenu(a.trayMenu())
		}
	})

	a.loadDatabase()
	a.catalog = apps.NewCatalog(a.settings.Applications)
	a.catalog.SetChanged(a.catalogChanged)
	a.fileIndex = store.NewFileIndex(a.db)
	a.fileIndex.SetOnUpdate(func(entries []model.Entry) {
		// The engine is thread-safe; rebuilding up to MaxEntries entries on
		// the UI thread would stall typing for hundreds of milliseconds.
		a.fileEngine.ReplaceEntries(entries, a.profileMap())
		if a.panel != nil {
			a.panel.Update(func() { a.updateResults() })
		}
	})
	a.everything = everything.NewProvider(func() settings.FileIndexOptions { return a.currentSettings().Files }, func() {
		a.publishUpdate(func() {})
	})
	a.pluginRepo = plugin.NewRepository(filepath.Join(a.dataDir, "plugins"))
	a.plugins = plugin.NewManager()
	a.official.client = plugin.NewCatalogClient(filepath.Join(a.dataDir, "cache", "official-catalog.json"))
	// Plugins are only needed at the first query or when the settings page
	// opens; the directory scan stays off the path to the first frame.
	pluginOptions := a.settings.Plugins
	go func() {
		_ = a.pluginRepo.Refresh()
		a.plugins.Configure(a.pluginRepo.Installed(), pluginOptions)
		log.Printf("plugins installed=%d enabled=%v", len(a.pluginRepo.Installed()), pluginOptions.EnabledIDs)
	}()

	// The result rows draw the icons in DIPs; the shell renders them in device
	// pixels, so they are asked for at the display's scale: a 28 DIP icon on a
	// display at 150% needs 42 pixels, and asking for 28 and stretching them is
	// what made the icons look soft next to the desktop's.
	a.iconSize = 32
	a.iconPixels = 32
	a.scaleFactor = 1
	for _, display := range mygo.Screen.Displays() {
		if display.ScaleFactor > a.scaleFactor {
			a.scaleFactor = display.ScaleFactor
		}
	}
	a.iconPixels = int(float64(a.iconSize)*a.scaleFactor + 0.5)
	a.iconPixels = min(max(a.iconPixels, a.iconSize), 256)
	a.workArea = layout.WorkArea{Width: 1920, Height: 1080}
	if display := mygo.Screen.PrimaryDisplay(); display.WorkArea.Width > 0 && display.WorkArea.Height > 0 {
		a.workArea = layout.WorkArea{
			X: display.WorkArea.X, Y: display.WorkArea.Y,
			Width: display.WorkArea.Width, Height: display.WorkArea.Height,
		}
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
		search.SystemProvider{},
		windowProvider{},
		a.everything,
		a.plugins,
	}

	a.panel = mygo.NewWindow(mygo.WindowOptions{
		Panel:         true,
		Hidden:        true,
		Frameless:     true,
		Transparent:   true,
		Vibrancy:      mygo.VibrancyMica,
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
	a.panel.OnMove(a.scheduleAnchorCapture)
	a.createVolumeOverlay()
	a.setUpTray()
	a.plugins.OnCommandsChanged(func() { a.publishUpdate(func() { a.registerHotkey() }) })
	a.applyTheme()
	a.registerHotkey()
	a.applyVolumeSettings()
	a.applyOpenAtLogin()
	a.applyWindowSize()
	a.startUpdateChecks()

	diag.Mark("windows ready")
	a.loadFileCache()
	a.refreshApps()
	a.catalog.Watch(context.Background())
	if !a.settings.HideOnStartup && !mygo.App.WasOpenedAtLogin() {
		a.show()
	}
	if query := os.Getenv("FL_DEBUG_QUERY"); query != "" {
		// After show: opening the panel clears the query, so a query set
		// before it would never be seen.
		time.AfterFunc(2*time.Second, func() {
			a.panel.Update(func() {
				a.query = query
				a.updateResults()
			})
		})
	}
	if os.Getenv("FL_DEBUG_SHOW") == "1" {
		a.show()
	}
	if os.Getenv("FL_DEBUG_OPEN_SETTINGS") == "1" {
		if value := os.Getenv("FL_DEBUG_SETTINGS_PAGE"); value != "" {
			if page, err := strconv.Atoi(value); err == nil {
				a.settingsPage = min(max(page, 0), len(settingPages())-1)
			}
		}
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
	if value := os.Getenv("FL_DEBUG_VOLUME"); value != "" {
		if percent, err := strconv.Atoi(value); err == nil {
			a.volumePinned = true
			a.volumePct = min(max(percent, 0), 100)
			a.showVolumeOverlay()
		}
	}
	if value := os.Getenv("FL_DEBUG_EXIT_MS"); value != "" {
		if milliseconds, err := strconv.Atoi(value); err == nil && milliseconds > 0 {
			time.AfterFunc(time.Duration(milliseconds)*time.Millisecond, mygo.App.Quit)
		}
	}
	if os.Getenv("FL_DEBUG_MEMORY") == "1" {
		go a.memLoop()
	}
}

func (a *App) loadDatabase() {
	path := filepath.Join(a.dataDir, "data", "launcher.db")
	db, err := store.OpenDB(context.Background(), path)
	if err != nil {
		a.notice = locales.AppCannotOpenDatabase(err)
		return
	}
	a.db = db
	profiles, err := db.LoadProfiles(context.Background())
	if err != nil {
		a.notice = locales.AppCannotReadProfiles(err)
		return
	}
	a.profiles = profiles
}

func (a *App) currentSettings() settings.LauncherSettings {
	// Provider goroutines read the settings through here while the UI
	// thread replaces the whole struct on save; the atomic snapshot keeps
	// them from ever seeing a torn copy.
	if snap := a.settingsSnap.Load(); snap != nil {
		return *snap
	}
	return a.settings
}

// publishSettings makes the current settings visible to provider goroutines.
func (a *App) publishSettings() {
	snap := a.settings
	a.settingsSnap.Store(&snap)
}

// hiddenCount counts hidden profiles under the profiles lock, without
// copying the map the way profileMap would.
func (a *App) hiddenCount() int {
	a.profilesMu.Lock()
	defer a.profilesMu.Unlock()
	n := 0
	for _, profile := range a.profiles {
		if profile.Hidden {
			n++
		}
	}
	return n
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
	// Up to MaxEntries rows from SQLite plus a full engine rebuild: all of
	// it stays off the path to the first frame.
	fileOptions := a.settings.Files
	ctx := a.restartFileIndexContext()
	go func() {
		cached, err := a.fileIndex.LoadCached(ctx, fileOptions)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if a.panel != nil {
				a.panel.Update(func() { a.notice = locales.AppCannotReadFileIndex(err) })
			}
			return
		}
		a.fileEngine.ReplaceEntries(cached, a.profileMap())
		if a.panel != nil {
			a.panel.Update(func() { a.updateResults() })
		}
		if err := a.fileIndex.Configure(ctx, fileOptions); err != nil && ctx.Err() == nil {
			log.Printf("file index: %v", err)
		}
	}()
}

func (a *App) refreshApps() {
	// Runs on the catalog watcher's debounce timer. The guard and the flag
	// are atomic because the timer goroutine and the UI thread both touch
	// them; the index rebuild happens off the UI thread like the file one.
	if !a.loading.CompareAndSwap(false, true) {
		return
	}
	start := time.Now()
	go func() {
		entries, err := a.catalog.Discover(context.Background())
		ms := time.Since(start).Milliseconds()
		if err == nil {
			a.appEngine.ReplaceEntries(entries, a.profileMap())
			a.rememberEverything(entries)
		}
		if a.panel == nil || a.panel.IsDestroyed() {
			return
		}
		a.panel.Update(func() {
			a.loading.Store(false)
			a.lastDiscovery = time.Now()
			if err != nil {
				a.notice = locales.AppDiscoveryFailed(err)
				return
			}
			a.status = locales.AppDiscoveredApps(len(entries))
			log.Printf("apps: %d in %dms", len(entries), ms)
			a.updateResults()
		})
	}()
}

func (a *App) catalogChanged() {
	// Watcher goroutines call this; the timer lives on the UI thread.
	if a.panel == nil {
		return
	}
	a.panel.Update(func() {
		if a.catalogTimer != nil {
			a.catalogTimer.Stop()
		}
		a.catalogTimer = time.AfterFunc(2*time.Second, a.refreshApps)
	})
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
	a.updateResults()
}

func (a *App) hidePanel() {
	if a.queryCancel != nil {
		a.queryCancel()
		a.queryGeneration.Add(1)
	}
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
	a.positionPanelSize(a.settings.Width, a.settings.Height, a.settings.AnchorX, a.settings.AnchorY)
}

func (a *App) positionPanelSize(width, height int, anchorX, anchorY float64) {
	if a.panel == nil {
		return
	}
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
	}, 1, float64(width), float64(height), &layout.Anchor{X: anchorX, Y: anchorY})
	a.panel.SetBounds(mygo.Rectangle{X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height})
}

// previewPanelSize applies the draft size to the panel while the settings
// preview resizes it, so that the real window already has the size the
// preview shows when it next opens.
func (a *App) previewPanelSize() {
	if a.panel == nil {
		return
	}
	a.panel.SetSize(a.settingsDraft.Width, a.settingsDraft.Height)
	a.positionPanelSize(a.settingsDraft.Width, a.settingsDraft.Height, a.settingsDraft.AnchorX, a.settingsDraft.AnchorY)
}

// scheduleAnchorCapture stores the panel's place once the user stops moving
// it, so that the next one opens where this one was left. The settings
// preview moves the panel too, but its place belongs to the draft until it
// is saved.
func (a *App) scheduleAnchorCapture() {
	if a.panel == nil || (a.settingsWin != nil && a.settingsWin.IsVisible()) {
		return
	}
	if a.anchorTimer != nil {
		a.anchorTimer.Stop()
	}
	a.anchorTimer = time.AfterFunc(400*time.Millisecond, func() {
		if a.panel != nil {
			a.panel.Update(func() { a.rememberPanelAnchor() })
		}
	})
}

// rememberPanelAnchor records where the panel is as the share of the room
// around it in the work area, and saves the settings.
func (a *App) rememberPanelAnchor() {
	if a.panel == nil || (a.settingsWin != nil && a.settingsWin.IsVisible()) {
		return
	}
	bounds := a.panel.Bounds()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return
	}
	display := mygo.Screen.DisplayNearestPoint(mygo.Point{X: bounds.X + bounds.Width/2, Y: bounds.Y + bounds.Height/2})
	work := display.WorkArea
	if work.Width <= 0 || work.Height <= 0 {
		return
	}
	anchor := layout.Capture(
		layout.WorkArea{X: work.X, Y: work.Y, Width: work.Width, Height: work.Height},
		bounds.X, bounds.Y, bounds.Width, bounds.Height,
	)
	// A move of a few pixels is the rounding of a placement, not the user
	// dragging the panel: do not rewrite the settings for it.
	if math.Abs(anchor.X-a.settings.AnchorX) < 0.002 && math.Abs(anchor.Y-a.settings.AnchorY) < 0.002 {
		return
	}
	a.settings.AnchorX, a.settings.AnchorY = anchor.X, anchor.Y
	a.settingsDraft.AnchorX, a.settingsDraft.AnchorY = anchor.X, anchor.Y
	if err := a.config.Save(a.settings); err != nil {
		log.Printf("anchor: %v", err)
	}
}

func (a *App) applyWindowSize() {
	if a.panel == nil {
		return
	}
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
	// A double click on the icon shows the panel, as the notification area
	// of a launcher does; a right click opens the menu.
	a.tray.OnClick(a.trayClicked)
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.stopVolume()
		tray.Destroy()
	})
}

// trayMenu builds the notification area menu. The menu follows the app's
// light or dark appearance, which the theme tells the system (mygo forces
// its preferred app mode when the theme source changes). Checkable items are
// plain rows whose label states the value: a check box would reserve a mark
// column that leaves an empty gutter beside every item.
func (a *App) trayMenu() *mygo.Menu {
	loginLabel := locales.TrayRunAtLoginOff()
	if a.settings.RunAtLogin {
		loginLabel = locales.TrayRunAtLoginOn()
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.TrayShowSearch(), Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
		{Label: locales.TraySettings(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
		{Label: locales.UpdateCheck(), Click: func(*mygo.MenuItem, *mygo.Window) {
			a.settingsPage = len(settingPages()) - 1
			a.openSettings()
			a.checkForUpdates(false)
		}},
		mygo.Separator(),
		{Label: loginLabel, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.settings.RunAtLogin = !a.settings.RunAtLogin
			a.settingsDraft.RunAtLogin = a.settings.RunAtLogin
			_ = a.config.Save(a.settings)
			a.publishSettings()
			a.applyOpenAtLogin()
			if a.tray != nil {
				a.tray.SetMenu(a.trayMenu())
			}
		}},
		mygo.Separator(),
		{Label: locales.TrayQuit(), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

// trayClicked shows the panel on a double click of the notification area
// icon. A single click does nothing: it gives the double click a first click
// to pair with, and the menu keeps the right button.
func (a *App) trayClicked() {
	if isDoubleClick(&a.trayClickAt, time.Now()) {
		a.show()
	}
}

// trayDoubleClick is how long after a first click on the notification area
// icon a second click opens the panel.
const trayDoubleClick = 500 * time.Millisecond

// isDoubleClick reports whether now follows the click at *last closely
// enough to be a double click, keeping *last for the next click.
func isDoubleClick(last *time.Time, now time.Time) bool {
	if !last.IsZero() && now.Sub(*last) <= trayDoubleClick {
		*last = time.Time{}
		return true
	}
	*last = now
	return false
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
		if a.panel == nil || a.panel.IsDestroyed() {
			return
		}
		m := diag.Mem()
		a.panel.Update(func() { a.mem = m })
	}
}

// applyLanguage shows the app in the settings' language; the empty tag
// follows the system's languages again.
func (a *App) applyLanguage() {
	mygo.App.SetLocale(a.settings.Language)
}

// applySettings persists and applies the whole configuration.
func (a *App) applySettings(value settings.LauncherSettings) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if err := a.validateToolPluginConflicts(value); err != nil {
		return err
	}
	commands := a.shortcutCommands(value)
	for _, command := range commands {
		binding := shortcuts.Effective(command.Command, value.Shortcuts)
		if command.Available {
			for _, key := range binding.Keys {
				if _, err := shortcuts.ValidateKey(key, command.Scope); err != nil {
					return err
				}
			}
		}
	}
	shortcutsChanged := !reflect.DeepEqual(value.Shortcuts, a.settings.Shortcuts) || !reflect.DeepEqual(value.Plugins.EnabledIDs, a.settings.Plugins.EnabledIDs)
	if conflicts := shortcuts.Conflicts(commandDefinitions(commands), value.Shortcuts); shortcutsChanged && len(conflicts) > 0 {
		return fmt.Errorf("%s: %s (%s / %s)", locales.ShortcutsConflict(), conflicts[0].Key, conflicts[0].First, conflicts[0].Second)
	}
	if shortcutsChanged {
		if err := a.syncGlobalShortcuts(value, true); err != nil {
			return fmt.Errorf("%s: %w", locales.ShortcutsRegisterFailed(), err)
		}
	}
	if err := a.config.Save(value); err != nil {
		if shortcutsChanged {
			_ = a.syncGlobalShortcuts(a.settings, false)
		}
		return err
	}
	if a.settings.EffectiveUpdateChannel(appVersion()) != value.EffectiveUpdateChannel(appVersion()) {
		a.updates.available = nil
		a.updates.notesOpen = false
		a.updates.message = ""
	}
	previous := a.settings
	a.settings = value
	a.publishSettings()
	a.settingsDraft = cloneSettings(value)
	a.lastPreview = value.Theme
	if value.Language != previous.Language {
		a.applyLanguage()
	}
	if value.Applications.IncludeStoreApps != previous.Applications.IncludeStoreApps || value.Applications.IncludePath != previous.Applications.IncludePath || !slices.Equal(value.Applications.PortableDirectories, previous.Applications.PortableDirectories) {
		a.catalog.SetOptions(value.Applications)
		a.catalog.Close()
		a.catalog.Watch(context.Background())
		a.refreshApps()
	}
	if a.plugins != nil && !reflect.DeepEqual(value.Plugins, previous.Plugins) {
		a.plugins.Configure(a.pluginRepo.Installed(), value.Plugins)
	}
	if !reflect.DeepEqual(value.CustomItems, previous.CustomItems) || !reflect.DeepEqual(value.Workspaces, previous.Workspaces) {
		a.rebuildCustom()
	}
	if value.Theme != a.appliedTheme {
		a.applyTheme()
	}
	if value.Width != previous.Width || value.Height != previous.Height {
		a.applyWindowSize()
		a.loadBackground(value.Theme)
	}
	if value.Width != previous.Width || value.Height != previous.Height || value.Placement != previous.Placement || value.AnchorX != previous.AnchorX || value.AnchorY != previous.AnchorY {
		a.positionPanel()
	}
	if shortcutsChanged {
		_ = a.syncGlobalShortcuts(a.settings, false)
	}
	if value.VolumeWheel != previous.VolumeWheel {
		a.applyVolumeSettings()
	}
	if value.RunAtLogin != previous.RunAtLogin {
		a.applyOpenAtLogin()
	}
	if a.db != nil && !value.Files.SameIndex(previous.Files) {
		ctx := a.restartFileIndexContext()
		go func() {
			if err := a.fileIndex.Configure(ctx, value.Files); err != nil && ctx.Err() == nil {
				log.Printf("file index: %v", err)
			}
		}()
	}
	a.updateResults()
	return nil
}

// draftChanged reports whether the settings window shows changes that are not
// saved yet.
func (a *App) draftChanged() bool {
	return !reflect.DeepEqual(a.settingsDraft, a.settings)
}

// scheduleSettingsSave applies and saves the settings a moment after the last
// change: the settings window has no save button, so every control takes
// effect as it is set.
func (a *App) scheduleSettingsSave() {
	if a.settingsSaveTimer != nil {
		a.settingsSaveTimer.Stop()
	}
	a.settingsSaveTimer = time.AfterFunc(600*time.Millisecond, func() {
		if a.settingsWin != nil {
			a.settingsWin.Update(func() { a.flushSettingsDraft() })
		}
	})
}

// flushSettingsDraft applies the draft when it differs from the saved
// settings. Values that do not validate are reported instead of saved, and
// the saved ones are put back, so that the window never shows a look that was
// not kept.
func (a *App) flushSettingsDraft() {
	if !a.draftChanged() {
		return
	}
	if err := a.applySettings(a.settingsDraft); err != nil {
		a.settingsError = err.Error()
		a.settingsNotice = ""
		// Failed shortcut edits must not keep unregistering/retrying every
		// autosave tick. Keep the active bindings and unrelated draft edits.
		pluginEnableChanged := !reflect.DeepEqual(a.settingsDraft.Plugins.EnabledIDs, a.settings.Plugins.EnabledIDs)
		saved := cloneSettings(a.settings)
		a.settingsDraft.Shortcuts = saved.Shortcuts
		a.settingsDraft.Plugins.EnabledIDs = saved.Plugins.EnabledIDs
		if pluginEnableChanged && a.plugins != nil && a.pluginRepo != nil {
			a.plugins.Configure(a.pluginRepo.Installed(), a.settings.Plugins)
		}
		// Roll the preview back to the saved look — but only when it is not
		// there already: an invalid draft keeps failing every 600 ms, and
		// re-applying the same theme would re-read and re-decode the
		// background image on every cycle.
		if a.appliedTheme != a.settings.Theme {
			a.applyThemeFrom(a.settings.Theme)
		}
		a.applyWindowSize()
		a.positionPanel()
	} else {
		a.settingsError = ""
	}
	if a.settingsWin != nil {
		a.settingsWin.Update(func() {})
	}
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
	if a.appliedTheme == (settings.ThemeSettings{}) || backdropMaterial(value) != backdropMaterial(a.appliedTheme) || blursBehind(value) != blursBehind(a.appliedTheme) {
		a.applyBackdrop(value)
	}
	a.loadBackground(value)
	if a.panel != nil {
		a.panel.Update(func() {})
	}
	if a.settingsWin != nil {
		a.settingsWin.Update(func() {})
	}
	a.appliedTheme = value
}

// Mica uses the unmodified system backdrop. Clear live desktop blur before
// enabling it, and clear the system backdrop before enabling acrylic blur.
func (a *App) applyBackdrop(value settings.ThemeSettings) {
	blur := blursBehind(value)
	for _, win := range []*mygo.Window{a.panel, a.settingsWin, a.workspaceWin} {
		if win == nil {
			continue
		}
		if blur {
			win.SetVibrancy(mygo.VibrancyNone)
		}
		if err := glass.SetBlur(win.NativeHandle(), blur); err != nil {
			log.Printf("theme: %v", err)
		}
		if !blur {
			win.SetVibrancy(backdropMaterial(value))
		}
	}
}

func backdropMaterial(value settings.ThemeSettings) mygo.Vibrancy {
	if value.Background == settings.BackgroundMica {
		return mygo.VibrancyMica
	}
	return mygo.VibrancyNone
}

// blursBehind selects live desktop blur, independently of native mica.
func blursBehind(value settings.ThemeSettings) bool {
	return value.Background == settings.BackgroundAcrylic
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
		go func() { _ = a.db.SaveProfile(context.Background(), profile) }()
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
		go func() { _ = a.db.SaveProfile(context.Background(), profile) }()
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
	generation := a.queryGeneration.Add(1)
	if !a.panel.IsVisible() {
		cancel()
		return
	}
	a.resultsReady = false
	preferred := a.selectedID
	// The query goroutine must not read fields the UI thread replaces on
	// save; take everything it needs now.
	snap := a.currentSettings()
	providers := a.providers

	if strings.TrimSpace(a.query) == "" {
		results := search.RecentEntries(snap.ShowRecentEntries, a.profileSlice(), a.findEntry, snap.MaxResults)
		a.setResults(results, preferred)
		a.resultsReady = true
		a.panel.Update(func() {})
		return
	}
	q := search.RouteTools(search.ParseQuery(a.query), snap)
	log.Printf("query %q scope=%d", q.Text, q.Scope)
	go func() {
		batches := make([]search.Batch, 0, len(providers))
		for batch := range search.Run(ctx, providers, q, snap.MaxResults) {
			if generation != a.queryGeneration.Load() || ctx.Err() != nil {
				return
			}
			batches = append(batches, batch)
			matches := search.Merge(batches, snap.MaxResults)
			if len(matches) == 0 {
				continue
			}
			currentBatches := append([]search.Batch(nil), batches...)
			a.panel.Update(func() {
				if generation != a.queryGeneration.Load() {
					return
				}
				a.setResults(matches, preferred)
				a.resultsReady = true
				a.lastBatches = currentBatches
			})
		}
		a.panel.Update(func() {
			if generation != a.queryGeneration.Load() {
				return
			}
			if len(search.Merge(batches, snap.MaxResults)) == 0 {
				a.setResults(nil, preferred)
			}
			a.resultsReady = true
			a.lastBatches = batches
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

// statusParts composes the footer's middle section from the batches of the
// last query: the result count, provider errors and source warnings. It
// never reads its own previous output; the one-shot hints (app discovery,
// plugin actions) live in a.status and the footer prepends them.
func (a *App) statusParts(batches []search.Batch) []string {
	parts := []string{locales.StatusShowingResults(len(a.results))}
	for _, batch := range batches {
		if batch.Error != "" {
			parts = append(parts, batch.SourceID+": "+batch.Error)
		}
	}
	if a.catalog != nil {
		if warning := a.catalog.Warning(); warning != "" {
			parts = append(parts, warning)
		}
	}
	if a.fileIndex != nil {
		if warning := a.fileIndex.Warning(); warning != "" {
			parts = append(parts, warning)
		}
	}
	return parts
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
	if action == model.Open && strings.HasPrefix(entry.ID, "tool-query:") {
		a.query = entry.LaunchTarget
		a.updateResults()
		a.input.Focus()
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
			a.notice = locales.AppNoPathToCopy()
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
			a.notice = locales.AppNoPluginAction()
			return
		}
		a.executePluginAction(entry, entry.Actions[0])
		return
	}
	// The shell call waits for the launch to finish and, for runas, for the
	// UAC prompt; the UI thread only hears the outcome.
	a.hidePanel()
	go func() {
		outcome := apps.Launch(context.Background(), entry, action)
		if a.panel == nil {
			return
		}
		a.panel.Update(func() {
			if outcome.Succeeded {
				if (action == model.Open || action == model.RunAsAdministrator) && learnableKind(entry.Kind) {
					a.recordUse(entry.ID)
				}
				return
			}
			if outcome.Canceled {
				a.notice = locales.AppCanceled()
				return
			}
			a.notice = locales.AppCannotOpenEntry(entry.Title, outcome.Error)
			a.updateResults()
		})
	}()
}

// learnableKind mirrors search.learns: only real launch targets take part in
// the use-count learning, so window and system entries cannot fill the
// profile table with keys that never repeat.
func learnableKind(kind model.Kind) bool {
	switch kind {
	case model.Application, model.File, model.Directory:
		return true
	}
	return false
}

func (a *App) openSettings() {
	if !a.official.loaded {
		a.refreshOfficialCatalog(false)
	}
	// A plugin package copied into the directory while the app ran appears
	// when the settings open, without a restart.
	if a.pluginRepo != nil && a.plugins != nil {
		// Directory enumeration plus a manifest read per plugin; the page
		// shows last known list and refreshes when the scan lands.
		opts := a.settings.Plugins
		go func() {
			if err := a.pluginRepo.Refresh(); err == nil {
				a.plugins.Configure(a.pluginRepo.Installed(), opts)
			}
		}()
	}
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	a.settingsDraft = cloneSettings(a.settings)
	a.settingsError, a.settingsNotice = "", ""
	a.lastPreview = a.settingsDraft.Theme
	// No system title bar: the window draws its own, so that it matches the
	// theme instead of cutting a pale Windows frame across it. The view keeps
	// the resizing edges and the shadow of a titled window. The default is
	// wider than tall: the pages read as a column of cards, and a window the
	// height of the whole screen wasted the width.
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Hidden:            true,
		Title:             locales.SettingsWindowTitle(),
		Width:             1180,
		Height:            840,
		MinWidth:          820,
		MinHeight:         560,
		Transparent:       true,
		Vibrancy:          mygo.VibrancyMica,
		TitleBarStyle:     mygo.TitleBarHidden,
		HideWindowButtons: true,
		DisableMaximize:   true,
		StateKey:          "settings-layout-v3",
		Content:           ui.View(a.settingsView),
	})
	a.clampToWorkArea(a.settingsWin)
	a.settingsWin.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		// Everything applies as it is set; closing keeps what is valid.
		a.shortcutEdit = shortcutEditor{}
		a.flushSettingsDraft()
		a.settingsWin.Hide()
	})
	a.settingsWin.OnBlur(func() {
		if a.shortcutEdit.ID != "" {
			a.shortcutEdit = shortcutEditor{}
			a.settingsWin.Invalidate()
		}
	})
	if icon := windowIconPNG(); icon != nil {
		if err := a.settingsWin.SetIcon(icon); err != nil {
			log.Printf("settings icon: %v", err)
		}
	}
	a.applyBackdrop(a.displayTheme())
	a.settingsWin.Show()
}

// clampToWorkArea pulls a window back inside the work area of the display it
// sits on. The bounds a StateKey restores can name a size or a place of
// another monitor setup, which would open the window off the screen.
func (a *App) clampToWorkArea(win *mygo.Window) {
	b := win.Bounds()
	area := a.workArea
	for _, display := range mygo.Screen.Displays() {
		w := display.WorkArea
		cx, cy := b.X+b.Width/2, b.Y+b.Height/2
		if cx >= w.X && cx < w.X+w.Width && cy >= w.Y && cy < w.Y+w.Height {
			area = layout.WorkArea{X: w.X, Y: w.Y, Width: w.Width, Height: w.Height}
			break
		}
	}
	width := min(b.Width, area.Width)
	height := min(b.Height, area.Height)
	x := min(max(b.X, area.X), area.X+area.Width-width)
	y := min(max(b.Y, area.Y), area.Y+area.Height-height)
	if x != b.X || y != b.Y || width != b.Width || height != b.Height {
		win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: width, Height: height})
	}
}

func (a *App) close() {
	if a.fileIndexCancel != nil {
		a.fileIndexCancel()
	}
	if a.backgroundImage.cancel != nil {
		a.backgroundImage.cancel()
	}
	if a.globalShortcuts.Unregister != nil {
		_ = a.globalShortcuts.Apply(map[string]string{}, false)
	}
	if a.updateCancel != nil {
		a.updateCancel()
	}
	if a.queryCancel != nil {
		a.queryCancel()
	}
	if a.fileIndex != nil {
		a.fileIndex.Close()
	}
	if a.db != nil {
		a.db.Close()
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
