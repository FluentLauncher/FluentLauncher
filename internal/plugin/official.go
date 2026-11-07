package plugin

import "fluentlauncher/locales"

// OfficialPlugin describes one of the example plugins that ship with the
// launcher. The install area lists them by name and one line of purpose before
// any of them is installed, which is text the page cannot take from the
// packages themselves: reading plugin.json for every frame would be file I/O
// on the UI thread.
type OfficialPlugin struct {
	// ID is both the manifest id and the package directory under
	// resources/examples.
	ID string
	// Name is what the page shows; it repeats the manifest's name.
	Name string
	// Summary is one sentence about what the plugin answers.
	Summary string
}

// OfficialPlugins returns the bundled examples, in the order the settings page
// lists them. A test keeps this catalog in step with the packages.
func OfficialPlugins() []OfficialPlugin {
	return []OfficialPlugin{
		{
			ID:      "official.echo",
			Name:    locales.PluginOfficialEchoName(),
			Summary: locales.PluginOfficialEchoSummary(),
		},
		{
			ID:      "official.devtools",
			Name:    locales.PluginOfficialDevtoolsName(),
			Summary: locales.PluginOfficialDevtoolsSummary(),
		},
	}
}
