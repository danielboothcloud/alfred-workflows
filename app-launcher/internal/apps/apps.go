// Package apps finds application bundles by walking the filesystem, so it
// works even when Spotlight's index is broken or incomplete.
package apps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// App is one application bundle.
type App struct {
	Name string `json:"name"` // localised display name, e.g. "Voice Memos"
	File string `json:"file"` // bundle file name without .app, e.g. "VoiceMemos"
	Path string `json:"path"`
}

// maxDepth is how many folder levels below a root are searched, e.g.
// /Applications/Adobe Photoshop 2025/Adobe Photoshop 2025.app is depth 1.
const maxDepth = 3

// DefaultRoots are the folders searched for apps, in priority order. When the
// same bundle is reachable twice (e.g. /Applications/Safari.app is a symlink
// into the Safari cryptex), the first path wins.
func DefaultRoots() []string {
	roots := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "Applications"))
	}
	return append(roots,
		"/System/Applications",
		"/System/Cryptexes/App/System/Applications",
		"/System/Library/CoreServices/Applications",
	)
}

// Extras are individual apps that live outside the roots but should be found.
var Extras = []string{"/System/Library/CoreServices/Finder.app"}

// Scan walks roots (and extras) and returns the apps sorted by name, plus the
// modification time of every directory read, so the result can be cached
// until something is installed, removed or renamed.
func Scan(roots, extras []string) ([]App, map[string]int64) {
	s := scanner{
		dirs:    map[string]int64{},
		visited: map[string]bool{},
		seen:    map[string]bool{},
	}
	for _, r := range roots {
		s.walk(r, 0)
	}
	for _, p := range extras {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			s.add(p)
		}
	}
	sort.SliceStable(s.apps, func(i, j int) bool {
		a, b := strings.ToLower(s.apps[i].Name), strings.ToLower(s.apps[j].Name)
		if a != b {
			return a < b
		}
		return s.apps[i].Path < s.apps[j].Path
	})
	return s.apps, s.dirs
}

type scanner struct {
	apps    []App
	dirs    map[string]int64 // dir as given -> mtime (0 if missing)
	visited map[string]bool  // real dir paths, guards symlink loops
	seen    map[string]bool  // real app paths, de-duplicates symlinks
}

func (s *scanner) walk(dir string, depth int) {
	s.dirs[dir] = mtime(dir)
	real, err := filepath.EvalSymlinks(dir)
	if err != nil || s.visited[real] {
		return
	}
	s.visited[real] = true
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		p := filepath.Join(dir, name)
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			fi, err := os.Stat(p)
			isDir = err == nil && fi.IsDir()
		}
		switch {
		case !isDir:
		case strings.HasSuffix(name, ".app"):
			s.add(p) // never descend into a bundle
		case depth < maxDepth:
			s.walk(p, depth+1)
		}
	}
}

func (s *scanner) add(path string) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		real = path
	}
	if s.seen[real] {
		return
	}
	s.seen[real] = true
	s.apps = append(s.apps, App{Name: DisplayName(path), File: stem(path), Path: path})
}

func mtime(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.ModTime().UnixNano()
}

func stem(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".app")
}

// cacheVersion invalidates old cache files when the format changes.
const cacheVersion = 1

// maxAge forces an occasional rescan to pick up in-place renames inside
// bundles (e.g. an update that changes an app's display name).
const maxAge = 24 * time.Hour

type index struct {
	Version int              `json:"version"`
	Roots   []string         `json:"roots"`
	Built   time.Time        `json:"built"`
	Dirs    map[string]int64 `json:"dirs"`
	Apps    []App            `json:"apps"`
}

// Load returns the cached app list from file if no searched folder has
// changed since it was built, otherwise rescans and rewrites the cache.
func Load(file string, roots, extras []string, force bool) ([]App, error) {
	key := append(append([]string{}, roots...), extras...)
	if !force {
		if idx, ok := readIndex(file); ok && idx.fresh(key) {
			return idx.Apps, nil
		}
	}
	apps, dirs := Scan(roots, extras)
	idx := index{Version: cacheVersion, Roots: key, Built: time.Now(), Dirs: dirs, Apps: apps}
	return apps, writeIndex(file, idx)
}

func readIndex(file string) (index, bool) {
	var idx index
	b, err := os.ReadFile(file)
	if err != nil || json.Unmarshal(b, &idx) != nil {
		return idx, false
	}
	return idx, true
}

func (idx index) fresh(roots []string) bool {
	if idx.Version != cacheVersion || time.Since(idx.Built) > maxAge || !equal(idx.Roots, roots) {
		return false
	}
	for dir, mt := range idx.Dirs {
		if mtime(dir) != mt {
			return false
		}
	}
	return true
}

func writeIndex(file string, idx index) error {
	b, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
