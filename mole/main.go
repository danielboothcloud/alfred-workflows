// Command alfred-mole is an Alfred workflow for Mole (https://mole.fit).
//
//	alfred-mole menu [query]         Script Filter: Mole commands
//	alfred-mole apps [query]         Script Filter: apps to uninstall
//	alfred-mole run <action> [arg]   run an action chosen in either filter
//	alfred-mole work <action> [arg]  run uninstall/clean unattended (background)
//	alfred-mole notification <text>  Alfred notification JSON for "title\nbody"
//	alfred-mole refresh              rebuild the cached app list now
//	alfred-mole list                 print the cached app list
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"alfred-mole/internal/alfred"
	"alfred-mole/internal/fuzzy"
	"alfred-mole/internal/ghostty"
	"alfred-mole/internal/mole"
)

var version = "dev"

// maxAge forces a refresh even when no app folder changed, so sizes and
// apps in deeper subfolders stay reasonably current.
const maxAge = 6 * time.Hour

func main() {
	log.SetFlags(0)
	cmd, args := "menu", os.Args[1:]
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "menu":
		err = menuFilter(strings.Join(args, " "))
	case "apps":
		err = appsFilter(strings.Join(args, " "))
	case "run":
		if len(args) == 0 {
			err = fmt.Errorf("usage: alfred-mole run <action> [arg]")
			break
		}
		arg := ""
		if len(args) > 1 {
			arg = args[1]
		}
		if err = run(args[0], arg); err != nil {
			notify("Mole", err.Error())
		}
	case "work":
		if len(args) == 0 {
			err = fmt.Errorf("usage: alfred-mole work <action> [arg]")
			break
		}
		arg := ""
		if len(args) > 1 {
			arg = args[1]
		}
		err = work(args[0], arg)
	case "notification":
		err = notification(strings.Join(args, " "))
	case "refresh":
		err = refresh(true)
	case "list":
		err = printList()
	case "version":
		fmt.Println(version)
	default:
		err = fmt.Errorf("usage: alfred-mole [menu <query> | apps <query> | run <action> [arg] | refresh | list | version]")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func env(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}

// ---- mole menu -------------------------------------------------------------

type entry struct {
	title, subtitle string
	action, arg     string
	keywords        string // extra words to match
	mods            map[string]alfred.Mod
}

func menuEntries() []entry {
	uninstallKW := env("uninstall_keyword", "uninstall")
	entries := []entry{
		{
			title:    "Open Mole",
			subtitle: "Mole's interactive menu (mo)",
			action:   "mole",
			keywords: "run menu mo",
		},
		{
			title:    "Clean",
			subtitle: "Free up disk space in the background; you'll get a notification (mo clean)",
			action:   "clean",
			keywords: "mo clean disk space cache",
			mods: map[string]alfred.Mod{
				"alt": {Valid: true, Arg: "clean", Subtitle: "Preview what would be removed (mo clean --dry-run)", Variables: map[string]string{"action": "clean-dry"}},
			},
		},
		{
			title:    "Uninstall an app…",
			subtitle: "Search your apps (" + uninstallKW + ")",
			action:   "search",
			arg:      uninstallKW + " ",
			keywords: "mo uninstall remove app",
			mods: map[string]alfred.Mod{
				"cmd": {Valid: true, Arg: "uninstall", Subtitle: "Open Mole's app picker (mo uninstall)", Variables: map[string]string{"action": "uninstall-picker"}},
			},
		},
	}
	if fi, err := os.Stat(logPath()); err == nil {
		entries = append(entries, entry{
			title:    "Last run log",
			subtitle: "Output of the last background run, " + fi.ModTime().Format("2 Jan 15:04"),
			action:   "log",
			arg:      logPath(),
			keywords: "log output error history",
		})
	}
	return entries
}

func menuFilter(query string) error {
	fb := alfred.Feedback{}
	entries := menuEntries()
	names := make([]string, 0, len(entries)*2)
	owner := make([]int, 0, len(entries)*2)
	for i, e := range entries {
		names, owner = append(names, e.title), append(owner, i)
		for _, k := range strings.Fields(e.keywords) {
			names, owner = append(names, k), append(owner, i)
		}
	}
	seen := map[int]bool{}
	for _, n := range fuzzy.Filter(query, names) {
		i := owner[n]
		if seen[i] {
			continue
		}
		seen[i] = true
		e := entries[i]
		arg := e.arg
		if arg == "" {
			arg = e.action
		}
		fb.Items = append(fb.Items, alfred.Item{
			Title:     e.title,
			Subtitle:  e.subtitle,
			Arg:       arg,
			Valid:     true,
			Mods:      e.mods,
			Variables: map[string]string{"action": e.action},
		})
	}
	if len(fb.Items) == 0 {
		fb.Items = append(fb.Items, alfred.Item{
			Title:    fmt.Sprintf("No Mole command matches “%s”", query),
			Subtitle: "Try: open, clean, uninstall",
		})
	}
	return fb.Write(os.Stdout)
}

// ---- uninstall -------------------------------------------------------------

func appsFilter(query string) error {
	fb := alfred.Feedback{}
	if _, err := mole.Find(os.Getenv("mole_path")); err != nil {
		fb.Items = append(fb.Items, alfred.Item{Title: "Mole is not installed", Subtitle: err.Error()})
		return fb.Write(os.Stdout)
	}

	cache := mole.Cache{Dir: alfred.CacheDir()}
	list, fetched, loadErr := cache.Load()
	home, _ := os.UserHomeDir()
	refreshing := cache.Refreshing()
	lastErr, errAt := cache.LastError()
	recentFailure := lastErr != "" && errAt.After(fetched) && time.Since(errAt) < time.Minute
	if !refreshing && !recentFailure && (loadErr != nil || mole.Stale(fetched, mole.SearchDirs(home), maxAge, time.Now())) {
		spawnRefresh()
		refreshing = true
	}
	if refreshing {
		fb.Rerun = 1
	}

	if loadErr != nil {
		switch {
		case refreshing:
			fb.Items = append(fb.Items, alfred.Item{
				Title:    "Loading apps from Mole…",
				Subtitle: "The first scan takes about 10–15 seconds",
			})
		case lastErr != "":
			fb.Items = append(fb.Items, alfred.Item{Title: "Could not list apps", Subtitle: lastErr})
		default:
			fb.Items = append(fb.Items, alfred.Item{Title: "Could not list apps", Subtitle: loadErr.Error()})
		}
		return fb.Write(os.Stdout)
	}

	list = present(list)
	targets := mole.Targets(list)
	for _, i := range matchApps(query, list) {
		fb.Items = append(fb.Items, appItem(list[i], targets[i]))
	}
	if len(fb.Items) == 0 {
		fb.Items = append(fb.Items, alfred.Item{
			Title:    fmt.Sprintf("No apps match “%s”", query),
			Subtitle: "Only apps Mole can uninstall are listed",
		})
	}
	return fb.Write(os.Stdout)
}

// present drops apps whose bundle is gone, e.g. one just uninstalled while
// the cached list is being refreshed.
func present(list []mole.App) []mole.App {
	out := list[:0:0]
	for _, a := range list {
		if _, err := os.Lstat(a.Path); err == nil {
			out = append(out, a)
		}
	}
	return out
}

// matchApps returns indexes into list: A–Z for an empty query, otherwise
// fuzzy-ranked on the display name and the bundle folder name.
func matchApps(query string, list []mole.App) []int {
	if strings.TrimSpace(query) == "" {
		idx := make([]int, len(list))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool {
			return strings.ToLower(list[idx[a]].Title()) < strings.ToLower(list[idx[b]].Title())
		})
		return idx
	}
	names := make([]string, 0, len(list)*2)
	owner := make([]int, 0, len(list)*2)
	for i, a := range list {
		names, owner = append(names, a.Title()), append(owner, i)
		if !strings.EqualFold(a.Dir(), a.Title()) {
			names, owner = append(names, a.Dir()), append(owner, i)
		}
	}
	var out []int
	seen := map[int]bool{}
	for _, n := range fuzzy.Filter(query, names) {
		if i := owner[n]; !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	return out
}

func appItem(a mole.App, target string) alfred.Item {
	var parts []string
	if a.Size != "" && a.Size != "--" {
		parts = append(parts, a.Size)
	}
	if a.Source == "Homebrew" {
		parts = append(parts, "Homebrew cask "+a.UninstallName)
	}
	parts = append(parts, tilde(filepath.Dir(a.Path)))
	info := strings.Join(parts, " · ")

	action, arg, subtitle := "uninstall", target, "Uninstall in the background · "+info
	dryMod := alfred.Mod{Valid: true, Arg: target, Subtitle: "Preview the uninstall (mo uninstall --dry-run)", Variables: map[string]string{"action": "uninstall-dry"}}
	if target == "" {
		// Another app has the same name, so pick it in Mole's own list.
		action, arg = "uninstall-picker", a.Path
		subtitle = "Name shared with another app: choose it in Mole's picker · " + info
		dryMod = alfred.Mod{Valid: false, Subtitle: "Dry run needs a unique app name"}
	}
	return alfred.Item{
		Type:         "file:skipcheck",
		Title:        a.Title(),
		Subtitle:     subtitle,
		Arg:          arg,
		Valid:        true,
		Autocomplete: a.Title(),
		Icon:         &alfred.Icon{Type: "fileicon", Path: a.Path},
		QuickLookURL: a.Path,
		Text:         &alfred.Text{Copy: a.Path, LargeType: a.Title()},
		Mods: map[string]alfred.Mod{
			"cmd": {Valid: true, Arg: a.Path, Subtitle: "Reveal in Finder", Variables: map[string]string{"action": "reveal"}},
			"alt": dryMod,
		},
		Variables: map[string]string{"action": action},
	}
}

// spawnRefresh rebuilds the list in a detached process, so the Script
// Filter returns at once and Alfred reruns it until the refresh is done.
func spawnRefresh() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(self, "refresh", "--background") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err == nil {
		_ = cmd.Process.Release()
	}
}

func refresh(verbose bool) error {
	bin, err := mole.Find(os.Getenv("mole_path"))
	if err != nil {
		return err
	}
	start := time.Now()
	apps, err := mole.Cache{Dir: alfred.CacheDir()}.Refresh(context.Background(), bin)
	if err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "cached %d apps in %s\n", len(apps), time.Since(start).Round(100*time.Millisecond))
	}
	return nil
}

func printList() error {
	list, fetched, err := mole.Cache{Dir: alfred.CacheDir()}.Load()
	if err != nil {
		return fmt.Errorf("no cached list (run `alfred-mole refresh`): %w", err)
	}
	targets := mole.Targets(list)
	for i, a := range list {
		fmt.Printf("%s\t%s\t%s\t%s\n", a.Title(), targets[i], a.Size, a.Path)
	}
	fmt.Fprintf(os.Stderr, "%d apps, fetched %s\n", len(list), fetched.Format(time.DateTime))
	return nil
}

// ---- actions ---------------------------------------------------------------

// moleArgs maps a terminal action to Mole's arguments.
func moleArgs(action, arg string) ([]string, error) {
	switch action {
	case "mole":
		return nil, nil
	case "clean":
		return []string{"clean"}, nil
	case "clean-dry":
		return []string{"clean", "--dry-run"}, nil
	case "uninstall-picker":
		return []string{"uninstall"}, nil
	case "uninstall", "uninstall-dry":
		if strings.TrimSpace(arg) == "" {
			return nil, fmt.Errorf("%s needs an app name", action)
		}
		if action == "uninstall-dry" {
			return []string{"uninstall", "--dry-run", arg}, nil
		}
		return []string{"uninstall", arg}, nil
	}
	return nil, fmt.Errorf("unknown action %q", action)
}

// pauseScript runs Mole ("$0" "$@") with Homebrew on PATH, then keeps the
// terminal open so the summary can be read.
const pauseScript = `export PATH="` + mole.ExtraPath + `:$PATH"
"$0" "$@"
status=$?
printf '\n\033[2m[mole exited with status %d. Press any key to close]\033[0m' "$status"
read -rsn1
exit "$status"`

// terminalCommand is the shell command Ghostty runs for Mole with args.
func terminalCommand(bin string, args []string) string {
	return shellJoin(append([]string{"/bin/bash", "-c", pauseScript, bin}, args...)...)
}

func run(action, arg string) error {
	ctx := context.Background()
	switch action {
	case "reveal":
		return exec.CommandContext(ctx, "/usr/bin/open", "-R", arg).Run()
	case "search":
		return alfredSearch(arg)
	case "log":
		return exec.CommandContext(ctx, "/usr/bin/open", "-t", arg).Run()
	}
	if background(action) {
		if _, err := moleArgs(action, arg); err != nil {
			return err
		}
		return spawnWork(action, arg)
	}
	args, err := moleArgs(action, arg)
	if err != nil {
		return err
	}
	bin, err := mole.Find(os.Getenv("mole_path"))
	if err != nil {
		return err
	}
	return ghostty.Open(ctx, terminalCommand(bin, args), env("open_mode", "window"))
}

// alfredSearch shows Alfred with query, e.g. "uninstall ".
func alfredSearch(query string) error {
	script := `on run argv
	tell application id "com.runningwithcrayons.Alfred" to search (item 1 of argv)
end run`
	return exec.Command("/usr/bin/osascript", "-e", script, query).Run()
}

// shellJoin quotes args for a POSIX shell.
func shellJoin(args ...string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if a != "" && strings.IndexFunc(a, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:@%+,", r))
		}) < 0 {
			quoted[i] = a
			continue
		}
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + p[len(home):]
	}
	return p
}
