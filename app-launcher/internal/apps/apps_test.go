package apps

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func paths(list []App) []string {
	var out []string
	for _, a := range list {
		out = append(out, a.Path)
	}
	return out
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	a := mkdir(t, root, "a")
	b := mkdir(t, root, "b")
	mkdir(t, a, "Zed.app", "Contents", "MacOS", "Helper.app") // nested helpers are ignored
	mkdir(t, a, "Vendor", "Suite", "Alpha.app")               // folders are searched
	mkdir(t, a, ".Hidden.app")                                // hidden bundles are ignored
	mkdir(t, a, "1", "2", "3", "4", "TooDeep.app")            // depth limit
	if err := os.WriteFile(filepath.Join(a, "notes.app"), nil, 0o644); err != nil {
		t.Fatal(err) // a file, not a bundle
	}
	// The same bundle reached through a symlink is listed once, first path wins.
	if err := os.Symlink(filepath.Join(a, "Zed.app"), filepath.Join(b, "Zed.app")); err != nil {
		t.Fatal(err)
	}
	// A symlinked folder is followed, and a loop does not hang.
	mkdir(t, root, "linked", "Beta.app")
	if err := os.Symlink(filepath.Join(root, "linked"), filepath.Join(b, "Nix Apps")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(b, filepath.Join(b, "loop")); err != nil {
		t.Fatal(err)
	}
	extra := mkdir(t, root, "elsewhere", "Finder.app")

	got, dirs := Scan([]string{a, b, filepath.Join(root, "missing")}, []string{extra, filepath.Join(root, "Nope.app")})
	want := []string{
		filepath.Join(a, "Vendor", "Suite", "Alpha.app"),
		filepath.Join(b, "Nix Apps", "Beta.app"),
		extra,
		filepath.Join(a, "Zed.app"),
	}
	if !reflect.DeepEqual(paths(got), want) {
		t.Errorf("Scan paths = %v\nwant %v", paths(got), want)
	}
	if got[0].Name != "Alpha" || got[0].File != "Alpha" {
		t.Errorf("name = %q file = %q", got[0].Name, got[0].File)
	}
	if mt, ok := dirs[filepath.Join(root, "missing")]; !ok || mt != 0 {
		t.Errorf("missing root should be recorded with mtime 0, got %v %v", mt, ok)
	}
}

func TestLoadRescansWhenFolderChanges(t *testing.T) {
	root := t.TempDir()
	apps := mkdir(t, root, "Applications")
	mkdir(t, apps, "One.app")
	cache := filepath.Join(root, "index.json")
	roots := []string{apps}

	l, err := Load(cache, roots, nil, false)
	if err != nil || len(l) != 1 {
		t.Fatalf("first load: %v %v", l, err)
	}

	// Unchanged folders: served from cache, even if the cache is stale on disk.
	idx, _ := readIndex(cache)
	idx.Apps = append(idx.Apps, App{Name: "Cached", Path: "/x"})
	if err := writeIndex(cache, idx); err != nil {
		t.Fatal(err)
	}
	if l, _ := Load(cache, roots, nil, false); len(l) != 2 {
		t.Errorf("expected cached result, got %v", l)
	}

	// Installing an app changes the folder mtime and triggers a rescan.
	mkdir(t, apps, "Two.app")
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(apps, future, future); err != nil {
		t.Fatal(err)
	}
	l, _ = Load(cache, roots, nil, false)
	if got := paths(l); !reflect.DeepEqual(got, []string{filepath.Join(apps, "One.app"), filepath.Join(apps, "Two.app")}) {
		t.Errorf("after install = %v", got)
	}

	// Different roots invalidate the cache too.
	if l, _ := Load(cache, append(roots, filepath.Join(root, "more")), nil, false); len(l) != 2 {
		t.Errorf("roots change = %v", l)
	}
}
