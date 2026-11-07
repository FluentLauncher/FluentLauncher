package settings

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/locales"
)

// CustomLaunchItem is a user-defined launcher entry.
type CustomLaunchItem struct {
	ID                 string `json:"Id"`
	Name               string `json:"Name"`
	Aliases            string `json:"Aliases"`
	Target             string `json:"Target"`
	Arguments          string `json:"Arguments"`
	WorkingDirectory   string `json:"WorkingDirectory"`
	RunAsAdministrator bool   `json:"RunAsAdministrator"`
	Enabled            bool   `json:"Enabled"`
}

const CustomSourceID = "custom"

func (c *CustomLaunchItem) UnmarshalJSON(data []byte) error {
	type alias CustomLaunchItem
	v := alias(CustomLaunchItem{Enabled: true})
	if err := jsonUnmarshal(data, &v); err != nil {
		return err
	}
	*c = CustomLaunchItem(v)
	return nil
}

func NewCustomItemID() string {
	return randomID()
}

func (c CustomLaunchItem) Validate() error {
	if !validID(c.ID) {
		return errors.New(locales.ValidateItemID())
	}
	if strings.TrimSpace(c.Name) == "" || len(c.Name) > 100 {
		return errors.New(locales.ValidateItemName())
	}
	if len(c.Aliases) > 500 {
		return errors.New(locales.ValidateItemAliases())
	}
	if len(c.Arguments) > 2000 {
		return errors.New(locales.ValidateItemArguments())
	}
	if c.WorkingDirectory != "" && !isLocalPath(c.WorkingDirectory) {
		return errors.New(locales.ValidateWorkingDirectory())
	}
	if strings.TrimSpace(c.Target) == "" || len(c.Target) > 2000 {
		return errors.New(locales.ValidateItemTarget())
	}
	if IsWebTarget(c.Target) {
		if c.RunAsAdministrator || c.Arguments != "" || c.WorkingDirectory != "" {
			return errors.New(locales.ValidateWebItemOptions())
		}
		return nil
	}
	if !isLocalPath(c.Target) {
		return errors.New(locales.ValidateItemTargetPath())
	}
	return nil
}

func IsWebTarget(target string) bool {
	u, err := url.Parse(target)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

func (c CustomLaunchItem) ToEntry() model.Entry {
	kind := model.File
	switch {
	case IsWebTarget(c.Target):
		kind = model.URL
	default:
		if info, err := os.Stat(c.Target); err == nil && info.IsDir() {
			kind = model.Directory
		} else {
			switch strings.ToLower(filepath.Ext(c.Target)) {
			case ".exe", ".lnk", ".appref-ms", ".bat", ".cmd":
				kind = model.Application
			}
		}
	}
	return model.Entry{
		ID:           CustomSourceID + ":" + c.ID,
		Title:        c.Name,
		Subtitle:     c.Target + ifAny(c.Arguments, " "+c.Arguments),
		LaunchTarget: c.Target,
		Aliases:      SplitAliases(c.Aliases),
		Kind:         kind,
		SourceID:     CustomSourceID,
		IconPath:     iconPathFor(c.Target),
		Arguments:    c.Arguments,
		WorkingDir:   c.WorkingDirectory,
		RunElevated:  c.RunAsAdministrator,
	}
}

func iconPathFor(target string) string {
	if IsWebTarget(target) {
		return ""
	}
	return target
}

func ifAny(s, suffix string) string {
	if s == "" {
		return ""
	}
	return suffix
}

func SplitAliases(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '；' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// WebSearchSite is a keyword-triggered web search.
type WebSearchSite struct {
	Keyword     string `json:"Keyword"`
	Name        string `json:"Name"`
	URLTemplate string `json:"UrlTemplate"`
	Enabled     bool   `json:"Enabled"`
}

var ReservedKeywords = []string{"app", "file", "web", "win"}

func (w *WebSearchSite) UnmarshalJSON(data []byte) error {
	type alias WebSearchSite
	v := alias(WebSearchSite{Enabled: true})
	if err := jsonUnmarshal(data, &v); err != nil {
		return err
	}
	*w = WebSearchSite(v)
	return nil
}

func (w WebSearchSite) Validate() error {
	if len(w.Keyword) < 1 || len(w.Keyword) > 32 || strings.ContainsAny(w.Keyword, " \t\r\n") ||
		strings.HasPrefix(w.Keyword, ">") {
		return errors.New(locales.ValidateSiteKeyword())
	}
	for _, reserved := range ReservedKeywords {
		if strings.EqualFold(w.Keyword, reserved) {
			return errors.New(locales.ValidateSiteKeywordReserved(w.Keyword))
		}
	}
	if strings.TrimSpace(w.Name) == "" || len(w.Name) > 100 {
		return errors.New(locales.ValidateSiteName())
	}
	if !ValidURLTemplate(w.URLTemplate) {
		return errors.New(locales.ValidateSiteUrl())
	}
	return nil
}

func ValidURLTemplate(template string) bool {
	if template == "" || len(template) > 2000 || !strings.Contains(template, "{query}") {
		return false
	}
	u, err := url.Parse(strings.ReplaceAll(template, "{query}", "test"))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

func (w WebSearchSite) BuildURL(query string) string {
	escaped := strings.ReplaceAll(url.QueryEscape(query), "+", "%20")
	return strings.ReplaceAll(w.URLTemplate, "{query}", escaped)
}

// MatchSite returns the first enabled site whose keyword starts text,
// followed by a space and a non-empty term.
func MatchSite(sites []WebSearchSite, text string) (WebSearchSite, string, bool) {
	for _, site := range sites {
		if !site.Enabled || !strings.HasPrefix(strings.ToLower(text), strings.ToLower(site.Keyword)) {
			continue
		}
		rest := text[len(site.Keyword):]
		if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		if term := strings.TrimSpace(rest); term != "" {
			return site, term, true
		}
	}
	return WebSearchSite{}, "", false
}

// SiteConflicts reports duplicate keywords and collisions with plugin
// keywords without silently dropping any source.
func SiteConflicts(sites []WebSearchSite, pluginKeywords []string) []string {
	var warnings []string
	plugins := map[string]bool{}
	for _, keyword := range pluginKeywords {
		plugins[strings.ToLower(keyword)] = true
	}
	seen := map[string]bool{}
	for _, site := range sites {
		key := strings.ToLower(site.Keyword)
		if seen[key] {
			warnings = append(warnings, locales.ValidateSiteKeywordDuplicate(site.Keyword))
		}
		if plugins[key] {
			warnings = append(warnings, locales.ValidateSiteKeywordPluginClash(site.Keyword))
		}
		seen[key] = true
	}
	return warnings
}

// Workspace opens a fixed list of custom items in order.
type Workspace struct {
	ID      string   `json:"Id"`
	Name    string   `json:"Name"`
	Aliases string   `json:"Aliases"`
	ItemIDs []string `json:"ItemIds"`
	Enabled bool     `json:"Enabled"`
}

const WorkspaceSourceID = "workspaces"

func (w *Workspace) UnmarshalJSON(data []byte) error {
	type alias Workspace
	v := alias(Workspace{Enabled: true})
	if err := jsonUnmarshal(data, &v); err != nil {
		return err
	}
	*w = Workspace(v)
	return nil
}

func NewWorkspaceID() string {
	return randomID()
}

func (w Workspace) Validate() error {
	if !validID(w.ID) {
		return errors.New(locales.ValidateWorkspaceID())
	}
	if strings.TrimSpace(w.Name) == "" || len(w.Name) > 100 {
		return errors.New(locales.ValidateWorkspaceName())
	}
	if len(w.Aliases) > 500 {
		return errors.New(locales.ValidateWorkspaceAliases())
	}
	if len(w.ItemIDs) < 1 || len(w.ItemIDs) > 50 {
		return errors.New(locales.ValidateWorkspaceItems())
	}
	seen := map[string]bool{}
	for _, id := range w.ItemIDs {
		if strings.TrimSpace(id) == "" {
			return errors.New(locales.ValidateWorkspaceItemIDs())
		}
		key := strings.ToLower(id)
		if seen[key] {
			return errors.New(locales.ValidateWorkspaceItemIDs())
		}
		seen[key] = true
	}
	return nil
}

func (w Workspace) ToEntry(itemCount int) model.Entry {
	return model.Entry{
		ID:           WorkspaceSourceID + ":" + w.ID,
		Title:        w.Name,
		Subtitle:     locales.ResultWorkspaceSubtitle(itemCount),
		LaunchTarget: w.ID,
		Kind:         model.Workspace,
		SourceID:     WorkspaceSourceID,
		Aliases:      SplitAliases(w.Aliases),
	}
}

type WorkspaceStep struct {
	ItemID    string
	Name      string
	Succeeded bool
	Error     string
}

func (s WorkspaceStep) Skipped() bool {
	return !s.Succeeded && s.Error == ""
}

// RunWorkspace launches the items in order. A missing or disabled item is a
// failed step; the rest continue. The caller decides whether to cancel.
func RunWorkspace(w Workspace, items []CustomLaunchItem,
	launch func(CustomLaunchItem) model.Outcome) []WorkspaceStep {
	byID := map[string]CustomLaunchItem{}
	for _, item := range items {
		byID[strings.ToLower(item.ID)] = item
	}
	steps := make([]WorkspaceStep, 0, len(w.ItemIDs))
	for _, id := range w.ItemIDs {
		item, ok := byID[strings.ToLower(id)]
		if !ok {
			steps = append(steps, WorkspaceStep{ItemID: id, Name: id, Error: locales.ResultWorkspaceItemDeleted()})
			continue
		}
		if !item.Enabled {
			steps = append(steps, WorkspaceStep{ItemID: id, Name: item.Name, Error: locales.ResultWorkspaceItemDisabled()})
			continue
		}
		outcome := launch(item)
		step := WorkspaceStep{ItemID: id, Name: item.Name, Succeeded: outcome.Succeeded}
		if !outcome.Succeeded {
			if outcome.Canceled {
				step.Error = locales.ResultWorkspaceCanceled()
			} else if outcome.Error != "" {
				step.Error = outcome.Error
			} else {
				step.Error = locales.ResultWorkspaceLaunchFailed()
			}
		}
		steps = append(steps, step)
	}
	return steps
}

func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func isLocalPath(path string) bool {
	return filepath.IsAbs(path) && !strings.HasPrefix(path, `\\`)
}
