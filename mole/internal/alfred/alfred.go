// Package alfred writes Alfred Script Filter JSON.
package alfred

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// Icon is an item icon. Type "fileicon" shows the icon of the file at Path.
type Icon struct {
	Path string `json:"path"`
	Type string `json:"type,omitempty"`
}

// Mod overrides an item when a modifier key is held.
type Mod struct {
	Valid     bool              `json:"valid"`
	Arg       string            `json:"arg,omitempty"`
	Subtitle  string            `json:"subtitle,omitempty"`
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
	Type         string            `json:"type,omitempty"`
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

// Feedback is the Script Filter response. Rerun (0.1–5 seconds) makes
// Alfred run the script again, e.g. while a background refresh finishes.
type Feedback struct {
	Rerun float64 `json:"rerun,omitempty"`
	Items []Item  `json:"items"`
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

// CacheDir returns the workflow cache directory, creating it if needed.
func CacheDir() string {
	dir := os.Getenv("alfred_workflow_cache")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "alfred-mole")
	}
	_ = os.MkdirAll(dir, 0o700)
	return dir
}
