package mole

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	out := []byte("Warning: something\n[\n" +
		`  {"name": "Zed", "bundle_id": "dev.zed.Zed", "source": "App", "uninstall_name": "Zed", "path": "/Applications/Zed.app", "size": "--"},` + "\n" +
		`  {"name": "Alfred 5", "bundle_id": "com.runningwithcrayons.Alfred", "source": "Homebrew", "uninstall_name": "alfred", "path": "/Applications/Alfred 5.app", "size": "12MB"}` +
		"\n]\n")
	apps, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 || apps[1].UninstallName != "alfred" || apps[1].Source != "Homebrew" {
		t.Fatalf("Parse = %+v", apps)
	}
	if empty, err := Parse([]byte("[]\n")); err != nil || len(empty) != 0 {
		t.Fatalf("Parse([]) = %v, %v", empty, err)
	}
	if _, err := Parse([]byte("No applications found.")); err == nil {
		t.Fatal("expected an error for non-JSON output")
	}
}

func TestTitleAndDir(t *testing.T) {
	a := App{Name: "\u200eWhatsApp", Path: "/Users/x/Applications/PlayCover/\u200eWhatsApp.app"}
	if a.Title() != "WhatsApp" {
		t.Errorf("Title = %q", a.Title())
	}
	if (App{Path: "/Applications/Self Service Client.APP"}).Dir() != "Self Service Client" {
		t.Error("Dir should strip .app case-insensitively")
	}
}

func TestTargets(t *testing.T) {
	apps := []App{
		{Name: "Authenticator", Path: "/Applications/Authenticator.app"},
		{Name: "Authenticator", Path: "/Users/x/Applications/PlayCover/Authenticator.app"},
		{Name: "Authenticator", Path: "/Users/x/Applications/PlayCover/Auth Two.app"},
		{Name: "Alfred 5", UninstallName: "alfred", Source: "Homebrew", Path: "/Applications/Alfred 5.app"},
		{Name: "Self Service", Path: "/Applications/Self Service Client.app"},
		{Name: "-dash", Path: "/Applications/Dash.app"},
		{Name: "alfred 5", Path: "/Applications/alfred 5.app"},
	}
	want := []string{
		"Authenticator",
		"",         // name and folder both match an earlier app exactly
		"Auth Two", // falls back to the unique bundle folder name
		"Alfred 5", // display name, never the cask token
		"Self Service",
		"Dash", // never pass something that looks like a flag
		"",     // case-insensitive clash with "Alfred 5"
	}
	if got := Targets(apps); !reflect.DeepEqual(got, want) {
		t.Errorf("Targets =\n %q\nwant\n %q", got, want)
	}
}

func TestStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	old := now.Add(-time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}
	dirs := []string{dir, filepath.Join(dir, "missing")}
	if Stale(now.Add(-time.Minute), dirs, 6*time.Hour, now) {
		t.Error("fresh list should not be stale")
	}
	if !Stale(now.Add(-2*time.Hour), dirs, 6*time.Hour, now) {
		t.Error("list older than a folder change should be stale")
	}
	if !Stale(now.Add(-7*time.Hour), nil, 6*time.Hour, now) {
		t.Error("list older than maxAge should be stale")
	}
}

func TestCacheRefresh(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "mo")
	script := "#!/bin/sh\n[ \"$1 $2\" = \"uninstall --list\" ] || exit 2\n" +
		`echo '[{"name": "Zed", "bundle_id": "dev.zed.Zed", "source": "App", "uninstall_name": "Zed", "path": "/Applications/Zed.app", "size": "1MB"}]'` + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c := Cache{Dir: filepath.Join(dir, "cache")}
	if _, err := c.Refresh(t.Context(), fake); err != nil {
		t.Fatal(err)
	}
	apps, _, err := c.Load()
	if err != nil || len(apps) != 1 || apps[0].Name != "Zed" {
		t.Fatalf("Load = %v, %v", apps, err)
	}
	if c.Refreshing() {
		t.Error("lock should be released")
	}

	// A held lock blocks a second refresh.
	unlock, err := c.lock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Refresh(t.Context(), fake); err != ErrBusy {
		t.Errorf("Refresh while locked = %v, want ErrBusy", err)
	}
	unlock()

	// Failures are recorded and the old list is kept.
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho boom >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Refresh(t.Context(), fake); err == nil {
		t.Fatal("expected failure")
	}
	if msg, _ := c.LastError(); msg != "mo uninstall --list: boom" {
		t.Errorf("LastError = %q", msg)
	}
	if apps, _, _ := c.Load(); len(apps) != 1 {
		t.Error("failed refresh should keep the old list")
	}
}
