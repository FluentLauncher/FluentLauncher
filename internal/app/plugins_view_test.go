package app

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/plugin"
	"fluentlauncher/internal/plugin/hostservices"
	"fluentlauncher/internal/testutil"
	"fluentlauncher/locales"
	"github.com/egoist/mygo/ui"
)

func newPluginsApp(t *testing.T) *App {
	t.Helper()
	a := &App{settings: settings.Default(), settingsDraft: settings.Default(), settingsPage: pagePlugins, settingsHover: -1}
	root := t.TempDir()
	a.pluginRepo = plugin.NewRepository(filepath.Join(root, "plugins"))
	a.pluginData = hostservices.NewStore(root)
	if err := a.pluginRepo.Refresh(); err != nil {
		t.Fatal(err)
	}
	a.plugins = plugin.NewManager()
	t.Cleanup(a.plugins.Dispose)
	a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
	return a
}

func installExample(t *testing.T, a *App, id string) {
	t.Helper()
	if _, err := a.pluginRepo.Install(testutil.PluginFixture(t, id), false); err != nil {
		t.Fatal(err)
	}
	a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
}

func TestPluginsMarketplaceBrowseAndSearch(t *testing.T) {
	a := newPluginsApp(t)
	tt := ui.NewTester(a.settingsView, 1180, 840)
	for _, want := range []string{"插件市场", "发现插件", "剪贴板历史", "截图与贴图", "开发工具", "回显示例", "刷新", "本地安装"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q: %v", want, tt.Texts())
		}
	}
	if err := tt.Click(locales.MarketSearch()); err != nil {
		t.Fatal(err)
	}
	tt.Type("开发")
	if !tt.HasText("开发工具") || tt.HasText("剪贴板历史") {
		t.Fatalf("search did not filter: %v", tt.Texts())
	}
	a.market.query = "不存在的插件"
	tt.Frame()
	if !tt.HasText(locales.MarketNoMatch()) {
		t.Fatal("no empty search state")
	}
}

func TestMarketplaceInstallProgressSurvivesLanguageChange(t *testing.T) {
	setCatalogLocale(t, "zh-Hans")
	a := newPluginsApp(t)
	op, _ := a.beginPluginOperation("official.echo", "回显示例", "1.0.1")
	defer op.cancel()
	op.downloaded, op.total = 50, 100
	tt := ui.NewTester(a.settingsView, 1180, 900)
	if !tt.HasText("下载中 50%") {
		t.Fatal("download progress is missing")
	}
	setCatalogLocale(t, "en")
	tt.Frame()
	if !tt.HasText("Downloading 50%") {
		t.Fatalf("language change lost progress: %v", tt.Texts())
	}
}

func TestPluginsMarketplaceDetailsUseSwitches(t *testing.T) {
	a := newPluginsApp(t)
	installExample(t, a, "official.echo")
	tt := ui.NewTester(a.settingsView, 1180, 1000)
	if err := tt.Click("回显示例"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"official.echo · 版本 1.0.0", "功能介绍", "怎么使用", "启用插件", "参与普通搜索", "卸载", "运行诊断与包信息"} {
		if !tt.HasText(want) {
			t.Fatalf("detail missing %q: %v", want, tt.Texts())
		}
	}
	for _, label := range []string{locales.MarketEnable() + ": 回显示例", locales.MarketGlobal() + ": 回显示例"} {
		rect, ok := tt.Find(label)
		if !ok || rect.W <= rect.H {
			t.Fatalf("%q is not a switch: %+v", label, rect)
		}
	}
	if err := tt.Click(locales.MarketEnable() + ": 回显示例"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.plugins.Statuses()[0].Enabled {
		t.Fatal("enable switch did not update runtime")
	}
	if !tt.HasText(locales.PluginNotRunNote()) {
		t.Fatal("idle enabled plugin is not explained")
	}
	if err := tt.Click(locales.MarketGlobal() + ": 回显示例"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.plugins.Statuses()[0].GlobalQuery {
		t.Fatal("global switch did not update runtime")
	}
	if err := tt.Click(locales.MarketEnable() + ": 回显示例"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.plugins.Statuses()[0].Enabled {
		t.Fatal("disable switch did not persist")
	}
	if err := tt.Click(locales.MarketBack()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("发现插件") {
		t.Fatal("detail did not return to market")
	}
}

func TestPluginsMarketplaceInstallationFeedbackAndUpdateState(t *testing.T) {
	a := newPluginsApp(t)
	op, _ := a.beginPluginOperation("official.echo", "回显示例", "1.0.0")
	installExample(t, a, "official.echo")
	a.finishPluginOperation(op, nil)
	tt := ui.NewTester(a.settingsView, 1180, 900)
	if !tt.HasText(locales.MarketInstalledSuccess()) || !tt.HasText(locales.MarketUse()) || !a.plugins.Statuses()[0].Enabled {
		t.Fatal("installation has no usable success feedback")
	}
	a.setPluginEnabled("official.echo", false)
	op, _ = a.beginPluginOperation("official.echo", "回显示例", "1.0.0")
	a.finishPluginOperation(op, nil)
	tt.Frame()
	if a.plugins.Statuses()[0].Enabled || !tt.HasText(locales.MarketUpdatedSuccess()) {
		t.Fatal("update enabled a disabled plugin or lost feedback")
	}
	op, _ = a.beginPluginOperation("official.echo", "回显示例", "1.0.0")
	a.finishPluginOperation(op, context.Canceled)
	tt.Frame()
	if !tt.HasText(locales.MarketCancelled()) || len(a.pluginRepo.Installed()) != 1 {
		t.Fatal("cancel removed installed package")
	}
	tt.Frame()
}

func TestMarketplaceSuccessToastOverlay(t *testing.T) {
	for _, width := range []int{840, 1180} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("width-%d-dark-%t", width, dark), func(t *testing.T) {
				a := newPluginsApp(t)
				tt := ui.NewTester(a.settingsView, width, 900)
				tt.SetDark(dark)
				tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
				if err := tt.Click(locales.MarketSearch()); err != nil {
					t.Fatal(err)
				}
				before, ok := tt.Find(locales.MarketLocalInstall())
				if !ok {
					t.Fatal("missing marketplace toolbar")
				}
				op, _ := a.beginPluginOperation("official.echo", "回显示例", "1.0.0")
				installExample(t, a, "official.echo")
				a.finishPluginOperation(op, nil)
				message := a.market.feedback
				tt.Frame()
				tt.Advance(250 * time.Millisecond)
				toast, ok := tt.Find(message)
				if !ok || toast.Y < 810 || toast.X < 0 || toast.X+toast.W > float32(width) {
					t.Fatalf("success is not a floating toast near the window bottom: %+v", toast)
				}
				after, _ := tt.Find(locales.MarketLocalInstall())
				if before != after || !tt.Focused(locales.MarketSearch()) {
					t.Fatal("success toast moved the marketplace or took keyboard focus")
				}
				if tt.HasText(locales.MarketNoticeClose()) || tt.HasText(locales.ShortcutsConfigure()) {
					t.Fatal("success still shows the persistent feedback panel")
				}
				if dir := os.Getenv("FL_MARKET_ARTIFACT_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					saveMarketFrame(t, tt, filepath.Join(dir, fmt.Sprintf("toast-%d-dark-%t.png", width, dark)))
				}
				tt.Frame()
				tt.Advance(2749 * time.Millisecond)
				if !tt.HasText(message) {
					t.Fatal("toast closed before three seconds")
				}
				tt.Advance(time.Millisecond)
				if tt.HasText(message) {
					t.Fatal("toast did not close after three seconds")
				}
				tt.Advance(time.Second)
				if tt.HasText(message) {
					t.Fatal("a later rebuild showed the success toast again")
				}
			})
		}
	}
}

func TestMarketplaceRepeatedSuccessRestartsToast(t *testing.T) {
	a := newPluginsApp(t)
	installExample(t, a, "official.echo")
	op, _ := a.beginPluginOperation("official.echo", "回显示例", "1.0.0")
	a.finishPluginOperation(op, nil)
	message := a.market.feedback
	tt := ui.NewTester(a.settingsView, 1180, 900)
	tt.Advance(2 * time.Second)
	op, _ = a.beginPluginOperation("official.echo", "回显示例", "1.0.0")
	a.finishPluginOperation(op, nil)
	tt.Frame()
	tt.Advance(2 * time.Second)
	count := 0
	for _, text := range tt.Texts() {
		if text == message {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("repeated completion should restart one toast, found %d", count)
	}
	tt.Advance(time.Second)
	if tt.HasText(message) {
		t.Fatal("repeated success toast did not expire after its own three seconds")
	}
}

func TestMarketplaceFailureRemainsAfterToastExpires(t *testing.T) {
	a := newPluginsApp(t)
	a.official.client = plugin.NewCatalogClient(filepath.Join(t.TempDir(), "catalog.json"))
	a.official.catalog = &plugin.Catalog{SchemaVersion: 1, Plugins: []plugin.CatalogEntry{{ID: "official.echo", Version: "1.0.0", Name: map[string]string{"zh-Hans": "回显示例"}}}}
	op, _ := a.beginPluginOperation("official.echo", "回显示例", "1.0.0")
	a.finishPluginOperation(op, context.Canceled)
	message := a.market.feedback
	tt := ui.NewTester(a.settingsView, 1180, 900)
	tt.Advance(time.Second)
	op, _ = a.beginPluginOperation("official.echo", "回显示例", "1.0.0")
	a.finishPluginOperation(op, fmt.Errorf("package verification failed"))
	failure := a.market.feedback
	tt.Frame()
	tt.Advance(4 * time.Second)
	if tt.HasText(message) || !tt.HasText(failure) || !tt.HasText(locales.MarketRetry()) || !a.market.failed {
		t.Fatal("toast expiry removed the persistent error or retry action")
	}
}

func TestPluginsMarketplaceUninstallAndProblems(t *testing.T) {
	a := newPluginsApp(t)
	installExample(t, a, "official.echo")
	a.setPluginEnabled("official.echo", true)
	a.setPluginGlobalQuery("official.echo", true)
	a.finishPluginUninstall("official.echo", a.removePluginPackage("official.echo"))
	if len(a.pluginRepo.Installed()) != 0 || len(a.settingsDraft.Plugins.EnabledIDs) != 0 || len(a.settingsDraft.Plugins.GlobalQueryIDs) != 0 {
		t.Fatal("uninstall left settings or registration")
	}
	if err := os.MkdirAll(filepath.Join(a.pluginRepo.Dir(), "vendor.broken"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := a.pluginRepo.Refresh(); err != nil {
		t.Fatal(err)
	}
	a.market.view = 1
	tt := ui.NewTester(a.settingsView, 1180, 1000)
	if err := tt.Click(locales.SettingsPluginsProblemsCard()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if !tt.HasText("vendor.broken") || !tt.HasText("缺少 plugin.json") {
		t.Fatal("broken package cannot be diagnosed")
	}
}

func TestOfficialCatalogOffersIndependentUpgrade(t *testing.T) {
	a := newPluginsApp(t)
	installExample(t, a, "official.echo")
	a.official.client = plugin.NewCatalogClient(filepath.Join(t.TempDir(), "catalog.json"))
	a.official.catalog = &plugin.Catalog{SchemaVersion: 1, Plugins: []plugin.CatalogEntry{{ID: "official.echo", Version: "2.0.0", Name: map[string]string{"zh-Hans": "回显示例"}, Summary: map[string]string{"zh-Hans": "新版插件"}}}}
	tt := ui.NewTester(a.settingsView, 1180, 900)
	if !tt.HasText(locales.PluginCatalogUpgrade()) || !tt.HasText("1.0.0 → 2.0.0") {
		t.Fatal("independent update is missing")
	}
	a.official.catalog.Plugins[0].Version = "1.0.0"
	tt.Frame()
	if tt.HasText(locales.PluginCatalogUpgrade()) {
		t.Fatal("current plugin still offers an update")
	}
	a.market.view = 2
	tt.Frame()
	if !tt.HasText(locales.MarketNoUpdates()) {
		t.Fatal("missing up-to-date state")
	}
}

func TestPluginsMarketplaceLayouts(t *testing.T) {
	for _, width := range []int{840, 1180} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("width-%v-dark-%t", width, dark), func(t *testing.T) {
				a := newPluginsApp(t)
				a.settings.Theme.Background = settings.BackgroundSolid
				a.settings.Theme.Opacity = 1
				a.settingsDraft = a.settings
				installExample(t, a, "official.echo")
				tt := ui.NewTester(a.settingsView, width, 1000)
				tt.SetDark(dark)
				tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
				for _, label := range []string{"回显示例", "开发工具", locales.MarketLocalInstall()} {
					rect, ok := tt.Find(label)
					if !ok || rect.X < 208 || rect.X+rect.W > float32(width) {
						t.Fatalf("%q overflows: %+v", label, rect)
					}
				}
				if dir := os.Getenv("FL_MARKET_ARTIFACT_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					saveMarketFrame(t, tt, filepath.Join(dir, fmt.Sprintf("market-%v-dark-%t.png", width, dark)))
					a.market.detail = "official.echo"
					tt.Frame()
					saveMarketFrame(t, tt, filepath.Join(dir, fmt.Sprintf("detail-%v-dark-%t.png", width, dark)))
				}
			})
		}
	}
}

func saveMarketFrame(t *testing.T, tt *ui.Tester, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

// Optional cross-repository render of the actual packaged metadata and PNGs.
// It installs only into t.TempDir and never launches the screenshot process.
func TestMarketplaceReleasedAssetsPreview(t *testing.T) {
	release := os.Getenv("FL_TEST_OFFICIAL_RELEASE_DIR")
	output := os.Getenv("FL_MARKET_ARTIFACT_DIR")
	if release == "" || output == "" {
		t.Skip("set release and artifact directories for packaged previews")
	}
	data, err := os.ReadFile(filepath.Join(release, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog plugin.Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	a := newPluginsApp(t)
	a.settings.Theme.Background = settings.BackgroundSolid
	a.settings.Theme.Opacity = 1
	a.settingsDraft = a.settings
	a.official.catalog = &catalog
	a.official.client = plugin.NewCatalogClient(filepath.Join(t.TempDir(), "catalog-cache.json"))
	archives, err := filepath.Glob(filepath.Join(release, "official.screenshot-*-windows-amd64.zip"))
	if err != nil || len(archives) != 1 {
		t.Fatal(archives, err)
	}
	op, _ := a.beginPluginOperation("official.screenshot", "截图与贴图", "")
	registration, err := a.pluginRepo.Install(archives[0], false)
	if err != nil {
		t.Fatal(err)
	}
	op.version = registration.Manifest.Version
	a.finishPluginOperation(op, nil)
	a.market.icons = map[string]*ui.Bitmap{}
	for _, item := range a.marketItems() {
		if item.Official == nil || item.Official.Icon == nil {
			t.Fatal("missing released icon", item.ID)
		}
		asset := item.Official.Icon
		icon, err := os.ReadFile(filepath.Join(release, filepath.Base(asset.URL)))
		if err != nil {
			t.Fatal(err)
		}
		bitmap, err := ui.DecodeBitmap(icon)
		if err != nil {
			t.Fatal(err)
		}
		key := item.ID
		if item.Installed != nil {
			key += ":" + item.Manifest().Version + ":" + item.Manifest().Icon
		}
		key += ":" + asset.SHA256
		a.market.icons[key] = bitmap
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		a.market.detail = ""
		tt := ui.NewTester(a.settingsView, 1180, 960)
		tt.SetDark(dark)
		tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
		saveMarketFrame(t, tt, filepath.Join(output, fmt.Sprintf("released-market-dark-%t.png", dark)))
		a.openPluginDetails("official.screenshot")
		a.market.feedback = ""
		tt.Frame()
		saveMarketFrame(t, tt, filepath.Join(output, fmt.Sprintf("released-detail-dark-%t.png", dark)))
	}
}
