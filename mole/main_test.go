package main

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"alfred-mole/internal/alfred"
	"alfred-mole/internal/mole"
	"alfred-mole/internal/runner"
)

func TestMoleArgs(t *testing.T) {
	cases := map[string][]string{
		"mole":             nil,
		"clean":            {"clean"},
		"clean-dry":        {"clean", "--dry-run"},
		"uninstall":        {"uninstall", "Ente Auth"},
		"uninstall-dry":    {"uninstall", "--dry-run", "Ente Auth"},
		"uninstall-picker": {"uninstall"},
	}
	for action, want := range cases {
		got, err := moleArgs(action, "Ente Auth")
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("moleArgs(%q) = %q, %v; want %q", action, got, err, want)
		}
	}
	if _, err := moleArgs("uninstall", " "); err == nil {
		t.Error("uninstall without a name should fail")
	}
	if _, err := moleArgs("optimize", ""); err == nil {
		t.Error("unknown actions should fail")
	}
}

// The terminal command must hand Mole each argument intact, however odd
// the app name, and keep the exit status.
func TestTerminalCommand(t *testing.T) {
	name := `It's "odd" $HOME; rm -rf ~`
	cmd := terminalCommand("/bin/echo", []string{"uninstall", name})
	cmd = strings.Replace(cmd, "read -rsn1", "true", 1)
	out, err := exec.Command("/bin/sh", "-c", cmd).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.SplitN(string(out), "\n", 2)[0]; got != "uninstall "+name {
		t.Errorf("mole saw %q", got)
	}
}

func TestMatchApps(t *testing.T) {
	list := []mole.App{
		{Name: "Zed", Path: "/Applications/Zed.app"},
		{Name: "Self Service", Path: "/Applications/Self Service Client.app"},
		{Name: "alfred 5", Path: "/Applications/Alfred 5.app"},
	}
	titles := func(idx []int) (out []string) {
		for _, i := range idx {
			out = append(out, list[i].Title())
		}
		return out
	}
	if got := titles(matchApps("", list)); !reflect.DeepEqual(got, []string{"alfred 5", "Self Service", "Zed"}) {
		t.Errorf("empty query = %v", got)
	}
	if got := titles(matchApps("client", list)); !reflect.DeepEqual(got, []string{"Self Service"}) {
		t.Errorf("bundle folder name should match: %v", got)
	}
}

// Every dry run is on ⌥↩, and only dry runs are.
func TestDryRunsOnAlt(t *testing.T) {
	check := func(where string, mods map[string]alfred.Mod) {
		for key, m := range mods {
			isDry := strings.HasSuffix(m.Variables["action"], "-dry")
			if isDry != (key == "alt") && !(key == "alt" && !m.Valid) {
				t.Errorf("%s: %s mod has action %q", where, key, m.Variables["action"])
			}
		}
	}
	for _, e := range menuEntries() {
		check(e.title, e.mods)
	}
	app := mole.App{Name: "Zed", Path: "/Applications/Zed.app"}
	check("app", appItem(app, "Zed").Mods)
	check("app without target", appItem(app, "").Mods)
	if appItem(app, "Zed").Mods["alt"].Variables["action"] != "uninstall-dry" {
		t.Error("app ⌥↩ should be a dry run")
	}
	var clean entry
	for _, e := range menuEntries() {
		if e.action == "clean" {
			clean = e
		}
	}
	if clean.mods["alt"].Variables["action"] != "clean-dry" {
		t.Error("Clean ⌥↩ should be a dry run")
	}
}

// Uninstall and clean run unattended; dry runs and Mole's own UI stay in
// the terminal.
func TestBackground(t *testing.T) {
	for action, want := range map[string]bool{
		"uninstall": true, "clean": true,
		"uninstall-dry": false, "clean-dry": false, "uninstall-picker": false, "mole": false,
	} {
		if background(action) != want {
			t.Errorf("background(%q) = %v", action, !want)
		}
	}
}

func TestOutcome(t *testing.T) {
	div := strings.Repeat("=", 70)
	ok := runner.Result{Output: "stuff\n" + div + "\nUninstall complete\nRemoved 1 app · 8KB: Zed\n" + div + "\nUpdate 1.70.0 available, run mo update\n"}
	if title, body := outcome("uninstall", "Zed", ok, nil); title != "Uninstalled Zed" || body != "Removed 1 app · 8KB: Zed" {
		t.Errorf("success = %q / %q", title, body)
	}
	clean := runner.Result{Output: div + "\nCleanup complete\nRemoved: 1.2GB · 300 items\nFree space now: 50GB\n" + div + "\n"}
	if title, body := outcome("clean", "", clean, nil); title != "Cleanup complete" || body != "Removed: 1.2GB · 300 items\nFree space now: 50GB" {
		t.Errorf("clean = %q / %q", title, body)
	}
	missing := runner.Result{ExitCode: 1, Output: "Warning: No application found matching 'X'\nNo matching applications found.\nUpdate 1.70.0 available, run mo update\n"}
	if title, body := outcome("uninstall", "X", missing, nil); title != "Uninstall X failed" || body != "No matching applications found." {
		t.Errorf("failure = %q / %q", title, body)
	}
	denied := runner.Result{ExitCode: 1, Output: "☻ Admin access denied\n"}
	if _, body := outcome("uninstall", "X", denied, nil); body != "Admin access denied" {
		t.Errorf("error line = %q", body)
	}
	if title, _ := outcome("uninstall", "X", runner.Result{Cancelled: true}, nil); title != "Uninstall X cancelled" {
		t.Errorf("cancelled = %q", title)
	}
}

func TestMenuHasNoOptimize(t *testing.T) {
	for _, e := range menuEntries() {
		if strings.Contains(strings.ToLower(e.title+e.subtitle+e.keywords), "optimi") {
			t.Errorf("menu should not offer optimize: %+v", e)
		}
	}
}
