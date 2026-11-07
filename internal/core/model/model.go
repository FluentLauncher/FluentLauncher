// Package model holds the launcher's data types. It has no UI, storage or
// Windows dependency so that every source can produce the same values.
package model

import "time"

// Kind is what an entry opens or represents.
type Kind uint8

const (
	Application Kind = iota
	File
	Directory
	URL
	Calculation
	Plugin
	Command
	Window
	Workspace
	// Setting is a Windows settings page, control panel item or system tool.
	// LaunchTarget is a protocol URI (ms-settings:display) or a program, and
	// Arguments carries the applet or module parameters.
	Setting
)

// Action is a plugin-provided action shown in the result action panel.
type Action struct {
	ID    string
	Title string
	Kind  string
	Value string
}

// Entry is one searchable item.
type Entry struct {
	ID           string
	Title        string
	Subtitle     string
	LaunchTarget string

	Aliases      []string
	Kind         Kind
	SourceID     string
	IconPath     string
	AppID        string
	Arguments    string
	WorkingDir   string
	CopyText     string
	PluginID     string
	PluginResult string
	PreviewText  string
	Actions      []Action
	RunElevated  bool
}

// Profile is the user's personal rule for an entry.
type Profile struct {
	EntryID    string     `json:"EntryId"`
	Alias      string     `json:"Alias"`
	Pinned     bool       `json:"Pinned"`
	Hidden     bool       `json:"Hidden"`
	UseCount   int        `json:"UseCount"`
	LastUsedAt *time.Time `json:"LastUsedAt"`
}

// Result is an entry with its match score.
type Result struct {
	Entry Entry
	Score int
}

// Outcome is what launching an entry did.
type Outcome struct {
	Succeeded bool
	Error     string
	Canceled  bool
}

// EntryAction is the requested operation on an entry.
type EntryAction uint8

const (
	Open EntryAction = iota
	RunAsAdministrator
	OpenLocation
	CopyPath
	CopyText
)
