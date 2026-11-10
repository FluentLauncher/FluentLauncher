package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/plugin"
	"fluentlauncher/internal/testutil"
)

// writeBrokenPackage leaves a directory with no plugin.json in the plugin
// root, which is what an interrupted copy or a wrong ZIP looks like.
func writeBrokenPackage(t *testing.T, root, name string) error {
	t.Helper()
	return os.MkdirAll(filepath.Join(root, name), 0o755)
}

// newPluginsApp is the settings window on the plugin page, with a real
// repository and manager over a temporary plugin directory.
func newPluginsApp(t *testing.T) *App {
	t.Helper()
	a := &App{
		settings:      settings.Default(),
		settingsDraft: settings.Default(),
	}
	a.settingsPage = pagePlugins
	a.pluginRepo = plugin.NewRepository(filepath.Join(t.TempDir(), "plugins"))
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
	source := testutil.PluginFixture(t, id)
	if _, err := a.pluginRepo.Install(source, false); err != nil {
		t.Fatal(err)
	}
	a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
}

// The empty page has to say what a plugin is for, not just that there is
// nothing to see.
func TestPluginsPageEmptyState(t *testing.T) {
	a := newPluginsApp(t)
	tt := ui.NewTester(a.settingsView, 1180, 900)
	for _, want := range []string{"还没有安装插件", "额外的搜索来源", "安装插件", "回显示例", "运行诊断"} {
		if !tt.HasText(want) {
			t.Fatalf("empty page does not show %q", want)
		}
	}
	if !tt.HasText("点击安装时下载") {
		t.Fatal("install card does not explain where plugins come from")
	}
	// The picker names one bundled plugin at a time; opening it lists the
	// rest, however many official plugins ship.
	if err := tt.Click("回显示例"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("开发工具") {
		t.Fatalf("the picker does not list the other official plugin: %q", tt.Texts())
	}
	// HasText matches substrings, and the card title holds "已安装" too.
	for _, text := range tt.Texts() {
		if text == "已安装" {
			t.Fatal("nothing is installed, but the page says something is")
		}
	}
}

// Every line of a plugin entry has its own place: name, id and version,
// description, keywords, runtime and state.
func TestPluginsPageEntryShowsEveryField(t *testing.T) {
	a := newPluginsApp(t)
	installExample(t, a, "official.echo")
	tt := ui.NewTester(a.settingsView, 1180, 900)
	for _, want := range []string{
		"回显示例",
		"official.echo · 版本 1.0.0",
		"演示查询结果和复制动作",
		"触发词：echo",
		"未启用",
		"卸载",
	} {
		if !tt.HasText(want) {
			t.Fatalf("entry does not show %q", want)
		}
	}
	// The card above owns the installed plugin, so the install row has no
	// button for it any more.
	if !tt.HasText("已安装") {
		t.Fatal("installed example still offers to install it")
	}
}

// Enabling a plugin from the page applies at once, without waiting for the
// settings save, so the next search already asks it.
func TestPluginsPageToggleAppliesImmediately(t *testing.T) {
	a := newPluginsApp(t)
	installExample(t, a, "official.echo")
	a.setPluginEnabled("official.echo", true)
	if !containsFold(a.settingsDraft.Plugins.EnabledIDs, "official.echo") {
		t.Fatal("the draft does not hold the enabled plugin")
	}
	statuses := a.plugins.Statuses()
	if len(statuses) != 1 || !statuses[0].Enabled {
		t.Fatalf("the runtime did not follow the switch: %+v", statuses)
	}
	tt := ui.NewTester(a.settingsView, 1180, 900)
	if tt.HasText("未启用") {
		t.Fatal("page still reports the plugin as disabled")
	}
	if !tt.HasText("尚未运行") {
		t.Fatal("page does not explain that the plugin starts on the next search")
	}
}

// Uninstalling forgets the plugin in the settings too, so a later install
// does not inherit a stale "enabled" entry.
func TestPluginsPageUninstallClearsSettings(t *testing.T) {
	a := newPluginsApp(t)
	installExample(t, a, "official.echo")
	a.setPluginEnabled("official.echo", true)
	a.setPluginGlobalQuery("official.echo", true)
	a.uninstallPlugin("official.echo")
	if len(a.pluginRepo.Installed()) != 0 {
		t.Fatalf("still installed: %+v", a.pluginRepo.Installed())
	}
	if containsFold(a.settingsDraft.Plugins.EnabledIDs, "official.echo") ||
		containsFold(a.settingsDraft.Plugins.GlobalQueryIDs, "official.echo") {
		t.Fatalf("settings still name the removed plugin: %+v", a.settingsDraft.Plugins)
	}
	tt := ui.NewTester(a.settingsView, 1180, 900)
	if !tt.HasText("还没有安装插件") {
		t.Fatal("page did not return to the empty state")
	}
}

// Broken packages get a card of their own, one line per directory, instead of
// one joined warning that only showed the last one.
func TestPluginsPageListsBrokenPackages(t *testing.T) {
	a := newPluginsApp(t)
	if err := writeBrokenPackage(t, a.pluginRepo.Dir(), "vendor.broken"); err != nil {
		t.Fatal(err)
	}
	if err := a.pluginRepo.Refresh(); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.settingsView, 1180, 900)
	for _, want := range []string{"无法加载的插件包", "vendor.broken", "缺少 plugin.json"} {
		if !tt.HasText(want) {
			t.Fatalf("problem card does not show %q", want)
		}
	}
}

func TestOfficialCatalogOffersIndependentUpgrade(t *testing.T) {
	a := newPluginsApp(t)
	installExample(t, a, "official.echo")
	a.official.catalog = &plugin.Catalog{SchemaVersion: 1, Plugins: []plugin.CatalogEntry{{ID: "official.echo", Version: "2.0.0", Name: map[string]string{"zh-Hans": "回显示例"}, Summary: map[string]string{"zh-Hans": "新版插件"}}}}
	tt := ui.NewTester(a.settingsView, 1180, 900)
	if !tt.HasText("更新插件") || !tt.HasText("2.0.0") {
		t.Fatal("new official version does not offer an upgrade")
	}
	a.official.installing = true
	tt.Frame()
	_ = tt.Click("更新插件")
	tt.Frame()
	if !a.official.installing {
		t.Fatal("busy download accepted a duplicate operation")
	}
	a.official.installing = false
	a.official.catalog.Plugins[0].Version = "1.0.0"
	tt.Frame()
	if tt.HasText("更新插件") {
		t.Fatal("current version still offers an upgrade")
	}
}
