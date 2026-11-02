// Package alfred writes Alfred Script Filter JSON.
package alfred

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// Icon is an item icon.
type Icon struct {
	Path string `json:"path"`
	Type string `json:"type,omitempty"`
}

// Mod overrides an item when a modifier key is held.
type Mod struct {
	Valid     bool              `json:"valid"`
	Arg       string            `json:"arg,omitempty"`
	Subtitle  string            `json:"subtitle,omitempty"`
	Icon      *Icon             `json:"icon,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

// Text sets copy (⌘C) and Large Type (⌘L) content.
type Text struct {
	Copy      string `json:"copy,omitempty"`
	LargeType string `json:"largetype,omitempty"`
}

// Item is one Script Filter result.
type Item struct {
	UID          string            `json:"uid,omitempty"`
	Title        string            `json:"title"`
	Subtitle     string            `json:"subtitle,omitempty"`
	Arg          string            `json:"arg,omitempty"`
	Valid        bool              `json:"valid"`
	Autocomplete string            `json:"autocomplete,omitempty"`
	Icon         *Icon             `json:"icon,omitempty"`
	Mods         map[string]Mod    `json:"mods,omitempty"`
	Text         *Text             `json:"text,omitempty"`
	QuickLookURL string            `json:"quicklookurl,omitempty"`
	Variables    map[string]string `json:"variables,omitempty"`
}

// Feedback is the Script Filter response.
type Feedback struct {
	SkipKnowledge bool   `json:"skipknowledge,omitempty"`
	Items         []Item `json:"items"`
}

// Write encodes feedback as JSON.
func (f *Feedback) Write(w io.Writer) error {
	if f.Items == nil {
		f.Items = []Item{}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(f)
}

// IconPath returns an icon bundled in the workflow's icons directory.
func IconPath(name string) *Icon {
	return &Icon{Path: filepath.Join("icons", name+".png")}
}

// CacheDir returns the workflow cache directory, creating it if needed.
func CacheDir() string {
	dir := os.Getenv("alfred_workflow_cache")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "alfred-zellij")
	}
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

// BundleID returns the workflow bundle ID.
func BundleID() string {
	if id := os.Getenv("alfred_workflow_bundleid"); id != "" {
		return id
	}
	return "com.danielbooth.alfred-zellij"
}
