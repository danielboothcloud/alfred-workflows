// Command alfred-zellij is an Alfred workflow for managing zellij sessions.
//
//	alfred-zellij filter [query]   Script Filter JSON
//	alfred-zellij run              perform the action in $zj_action
//	alfred-zellij terminals        debug: Ghostty terminal → session mapping
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"alfred-zellij/internal/alfred"
	"alfred-zellij/internal/attach"
	"alfred-zellij/internal/fuzzy"
	"alfred-zellij/internal/sessions"
	"alfred-zellij/internal/zellij"
)

var version = "dev"

const renamePrefix = ":rename "
const renameArrow = " → "

type config struct {
	keyword       string
	zellijPath    string
	defaultCwd    string
	defaultLayout string
	layoutDir     string
	openMode      string
	reopen        bool
}

func loadConfig() config {
	c := config{
		keyword:       envOr("keyword", "zj"),
		zellijPath:    strings.TrimSpace(os.Getenv("zellij_path")),
		defaultCwd:    envOr("default_cwd", "~"),
		defaultLayout: strings.TrimSpace(os.Getenv("default_layout")),
		layoutDir:     strings.TrimSpace(os.Getenv("layout_dir")),
		openMode:      envOr("open_mode", "window"),
		reopen:        envOr("reopen_after_action", "1") == "1",
	}
	c.defaultCwd = zellij.ExpandHome(c.defaultCwd)
	return c
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("[alfred-zellij] ")
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	ctx := context.Background()
	cfg := loadConfig()
	var err error
	switch cmd {
	case "filter":
		query := ""
		if len(os.Args) > 2 {
			query = os.Args[2]
		}
		err = filter(ctx, cfg, query).Write(os.Stdout)
	case "run":
		msg, runErr := run(ctx, cfg)
		if runErr != nil {
			// Printed so Alfred's notification shows it.
			fmt.Println("⚠️ " + runErr.Error())
			log.Print(runErr)
			return
		}
		if msg != "" {
			fmt.Println(msg)
		}
	case "previews":
		err = writePreviews(ctx, cfg)
	case "terminals":
		err = describe(ctx, cfg)
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintln(os.Stderr, "usage: alfred-zellij filter [query] | run | terminals | version")
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func newCLI(cfg config) (*zellij.CLI, error) {
	bin, err := zellij.FindBinary(cfg.zellijPath)
	if err != nil {
		return nil, err
	}
	return &zellij.CLI{Bin: bin}, nil
}

// ---------------------------------------------------------------- filter

func filter(ctx context.Context, cfg config, query string) *alfred.Feedback {
	fb := &alfred.Feedback{}
	z, err := newCLI(cfg)
	if err != nil {
		fb.Items = append(fb.Items, errorItem("zellij not found", err.Error()))
		return fb
	}
	cacheDir := alfred.CacheDir()
	infos, fresh, err := sessions.Load(ctx, z, cacheDir)
	if err != nil {
		fb.Items = append(fb.Items, errorItem("Could not list zellij sessions", err.Error()))
		return fb
	}
	if fresh {
		spawnPreviews()
	}

	if rest, ok := strings.CutPrefix(query, renamePrefix); ok {
		fb.SkipKnowledge = true
		fb.Items = renameItems(infos, rest)
		return fb
	}

	name, layoutQuery, wantsLayout := strings.Cut(query, "@")
	name = strings.TrimSpace(name)
	if wantsLayout {
		fb.SkipKnowledge = true
		fb.Items = layoutItems(cfg, infos, name, strings.TrimSpace(layoutQuery), query)
		return fb
	}

	names := make([]string, len(infos))
	for i, s := range infos {
		names[i] = s.Name
	}
	exact := false
	for _, i := range fuzzy.Filter(name, names) {
		fb.Items = append(fb.Items, sessionItem(infos[i], query))
		exact = exact || infos[i].Name == name
	}
	fb.Items = append(fb.Items, bulkItems(infos, name, query)...)
	if name != "" && !exact {
		fb.Items = append(fb.Items, createItem(cfg, name, cfg.defaultLayout, query))
	}

	if len(fb.Items) == 0 {
		fb.Items = append(fb.Items, alfred.Item{
			Title:    "No zellij sessions",
			Subtitle: "Type a name to create a session",
			Icon:     alfred.IconPath("new"),
		})
	}
	return fb
}

func vars(action, session, query string, extra ...string) map[string]string {
	v := map[string]string{"zj_action": action, "zj_session": session, "zj_query": query}
	for i := 0; i+1 < len(extra); i += 2 {
		v[extra[i]] = extra[i+1]
	}
	return v
}

func sessionItem(s sessions.Info, query string) alfred.Item {
	it := alfred.Item{
		UID:          "session:" + s.Name,
		Title:        s.Name,
		Arg:          s.Name,
		Valid:        true,
		Autocomplete: s.Name,
		Text:         &alfred.Text{Copy: s.Name, LargeType: s.Name},
		QuickLookURL: s.PreviewPath,
		Variables:    vars("open", s.Name, query, "zj_exists", "1"),
	}
	newWindow := alfred.Mod{
		Valid: true, Arg: s.Name,
		Subtitle:  "Open in a new Ghostty window",
		Variables: vars("open-new", s.Name, query, "zj_exists", "1"),
	}
	if s.Exited {
		it.Icon = alfred.IconPath("exited")
		it.Subtitle = joinNonEmpty(" · ", "Exited", ageText(s.AgeText), "↩ resurrect")
		del := alfred.Mod{
			Valid: true, Arg: s.Name,
			Subtitle:  "Delete exited session " + s.Name,
			Icon:      alfred.IconPath("delete"),
			Variables: vars("delete", s.Name, query),
		}
		// No cmd mods: Alfred uses ⌘ natively (⌘1-9, ⌘C, ⌘L, ⌘Y).
		it.Mods = map[string]alfred.Mod{
			"alt":  del,
			"ctrl": {Valid: false, Subtitle: "Only running sessions can be renamed"},
			"fn":   newWindow,
		}
		return it
	}

	state := "Detached"
	it.Icon = alfred.IconPath("running")
	if s.Clients > 0 {
		state = fmt.Sprintf("Attached (%d)", s.Clients)
		it.Icon = alfred.IconPath("attached")
	}
	it.Subtitle = joinNonEmpty(" · ", state, tabSummary(s), ageText(s.AgeText))
	it.Mods = map[string]alfred.Mod{
		"alt": {
			Valid: true, Arg: s.Name,
			Subtitle:  "Kill " + s.Name + " (stays resurrectable; ⌥↩ again to delete)",
			Icon:      alfred.IconPath("kill"),
			Variables: vars("kill", s.Name, query),
		},
		"ctrl": {
			Valid: true, Arg: s.Name,
			Subtitle:  "Rename " + s.Name + "…",
			Icon:      alfred.IconPath("rename"),
			Variables: vars("rename-prompt", s.Name, query),
		},
		"fn": newWindow,
	}
	return it
}

func ageText(age string) string {
	if age == "" {
		return ""
	}
	return age + " old"
}

func tabSummary(s sessions.Info) string {
	switch len(s.Tabs) {
	case 0:
		return ""
	case 1:
		return "1 tab: " + s.Tabs[0]
	}
	list := strings.Join(s.Tabs, ", ")
	if r := []rune(list); len(r) > 50 {
		list = string(r[:49]) + "…"
	}
	return fmt.Sprintf("%d tabs: %s", len(s.Tabs), list)
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

func createItem(cfg config, name, layout, query string) alfred.Item {
	if err := zellij.ValidateName(name); err != nil {
		return alfred.Item{Title: "Invalid session name", Subtitle: err.Error(), Icon: alfred.IconPath("error")}
	}
	sub := "↩ create & attach"
	if layout != "" {
		sub += " · layout " + layout
	}
	sub += " · in " + tildePath(cfg.defaultCwd)
	if !strings.Contains(query, "@") {
		sub += " · type @ to choose a layout"
	}
	return alfred.Item{
		Title:        fmt.Sprintf("Create session “%s”", name),
		Subtitle:     sub,
		Arg:          name,
		Valid:        true,
		Autocomplete: name + " @",
		Icon:         alfred.IconPath("new"),
		Variables:    vars("create", name, query, "zj_layout", layout),
		Mods: map[string]alfred.Mod{
			"alt": {Valid: true, Arg: name, Subtitle: "Create in the background without attaching",
				Variables: vars("create-bg", name, query, "zj_layout", layout)},
			"fn": {Valid: true, Arg: name, Subtitle: "Create in a new Ghostty window",
				Variables: vars("create-new", name, query, "zj_layout", layout)},
		},
	}
}

func layoutItems(cfg config, infos []sessions.Info, name, layoutQuery, query string) []alfred.Item {
	for _, s := range infos {
		if s.Name == name {
			return []alfred.Item{{
				Title:    fmt.Sprintf("Session “%s” already exists", name),
				Subtitle: "Layouts only apply to new sessions",
				Icon:     alfred.IconPath("error"),
			}}
		}
	}
	if name == "" {
		return []alfred.Item{{Title: "Type a session name before @", Subtitle: "e.g. myproject @compact", Icon: alfred.IconPath("new")}}
	}
	layouts := zellij.Layouts(cfg.layoutDir)
	var items []alfred.Item
	for _, i := range fuzzy.Filter(layoutQuery, layouts) {
		it := createItem(cfg, name, layouts[i], query)
		it.Title = fmt.Sprintf("Create “%s” with layout %s", name, layouts[i])
		it.Autocomplete = name + " @" + layouts[i]
		it.Icon = alfred.IconPath("layout")
		items = append(items, it)
	}
	if len(items) == 0 && layoutQuery != "" {
		// Allow layouts zellij can resolve that we did not list (paths, URLs).
		it := createItem(cfg, name, layoutQuery, query)
		it.Title = fmt.Sprintf("Create “%s” with layout %s", name, layoutQuery)
		items = append(items, it)
	}
	return items
}

func bulkItems(infos []sessions.Info, name, query string) []alfred.Item {
	q := strings.ToLower(name)
	if len(q) < 3 {
		return nil
	}
	running, exited := 0, 0
	for _, s := range infos {
		if s.Exited {
			exited++
		} else {
			running++
		}
	}
	var items []alfred.Item
	if running > 0 && strings.HasPrefix("kill all", q) {
		items = append(items, alfred.Item{
			Title:    fmt.Sprintf("Kill all running sessions (%d)", running),
			Subtitle: "Sessions stay resurrectable · ⌥↩ kill and delete everything",
			Valid:    true, Arg: "kill-all",
			Icon:      alfred.IconPath("kill"),
			Variables: vars("kill-all", "", query),
			Mods: map[string]alfred.Mod{"alt": {Valid: true, Arg: "delete-all-force",
				Subtitle:  fmt.Sprintf("Kill and delete all %d sessions", running+exited),
				Variables: vars("delete-all-force", "", query)}},
		})
	}
	if exited > 0 && strings.HasPrefix("delete all", q) {
		items = append(items, alfred.Item{
			Title:    fmt.Sprintf("Delete all exited sessions (%d)", exited),
			Subtitle: "Running sessions are kept",
			Valid:    true, Arg: "delete-all",
			Icon:      alfred.IconPath("delete"),
			Variables: vars("delete-all", "", query),
		})
	}
	return items
}

func renameItems(infos []sessions.Info, rest string) []alfred.Item {
	old, newName, _ := strings.Cut(rest, strings.TrimSpace(renameArrow))
	old, newName = strings.TrimSpace(old), strings.TrimSpace(newName)
	found := false
	for _, s := range infos {
		if s.Name == old && !s.Exited {
			found = true
		}
		if s.Name == newName && newName != old {
			return []alfred.Item{{Title: fmt.Sprintf("“%s” already exists", newName), Subtitle: "Choose another name", Icon: alfred.IconPath("error")}}
		}
	}
	if !found {
		return []alfred.Item{{Title: fmt.Sprintf("No running session “%s”", old), Icon: alfred.IconPath("error")}}
	}
	if newName == "" || newName == old {
		return []alfred.Item{{
			Title:    fmt.Sprintf("Rename “%s”", old),
			Subtitle: "Type the new name",
			Icon:     alfred.IconPath("rename"),
		}}
	}
	if err := zellij.ValidateName(newName); err != nil {
		return []alfred.Item{{Title: "Invalid session name", Subtitle: err.Error(), Icon: alfred.IconPath("error")}}
	}
	return []alfred.Item{{
		Title:     fmt.Sprintf("Rename “%s” → “%s”", old, newName),
		Subtitle:  "↩ rename",
		Arg:       newName,
		Valid:     true,
		Icon:      alfred.IconPath("rename"),
		Variables: vars("rename", old, "", "zj_new", newName),
	}}
}

func errorItem(title, sub string) alfred.Item {
	return alfred.Item{Title: title, Subtitle: sub, Icon: alfred.IconPath("error")}
}

func tildePath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// ---------------------------------------------------------------- run

func run(ctx context.Context, cfg config) (string, error) {
	action := os.Getenv("zj_action")
	session := os.Getenv("zj_session")
	query := os.Getenv("zj_query")
	z, err := newCLI(cfg)
	if err != nil {
		return "", err
	}
	cacheDir := alfred.CacheDir()
	defer sessions.Invalidate(cacheDir)

	opener := func() (*attach.Opener, error) {
		infos, err := sessions.Gather(ctx, z, cacheDir)
		if err != nil {
			return nil, err
		}
		return &attach.Opener{Z: z, Running: sessions.RunningNames(infos)}, nil
	}
	open := func(opts attach.Options) (string, error) {
		o, err := opener()
		if err != nil {
			return "", err
		}
		opts.Mode = cfg.openMode
		return "", o.Open(ctx, session, opts)
	}
	mutated := func(msg string, err error) (string, error) {
		if err != nil {
			return "", err
		}
		if cfg.reopen {
			sessions.Invalidate(cacheDir)
			reopenAlfred(cfg.keyword + " " + query)
		}
		return msg, nil
	}

	switch action {
	case "open":
		return open(attach.Options{Exists: true})
	case "open-new":
		return open(attach.Options{Exists: true, NewWindow: true})
	case "create", "create-new":
		if err := zellij.ValidateName(session); err != nil {
			return "", err
		}
		return open(attach.Options{
			Layout:    os.Getenv("zj_layout"),
			Cwd:       cfg.defaultCwd,
			NewWindow: action == "create-new",
		})
	case "create-bg":
		if err := zellij.ValidateName(session); err != nil {
			return "", err
		}
		return createBackground(ctx, z, cfg, session, os.Getenv("zj_layout"))
	case "kill":
		return mutated("Killed "+session, z.Kill(ctx, session))
	case "delete":
		return mutated("Deleted "+session, z.Delete(ctx, session, false))
	case "kill-delete":
		return mutated("Killed and deleted "+session, z.Delete(ctx, session, true))
	case "kill-all":
		return mutated("Killed all running sessions", z.KillAll(ctx))
	case "delete-all":
		return mutated("Deleted all exited sessions", z.DeleteAll(ctx))
	case "delete-all-force":
		return mutated("Killed and deleted all sessions", deleteAllForce(ctx, z))
	case "rename-prompt":
		reopenAlfred(cfg.keyword + " " + renamePrefix + session + renameArrow)
		return "", nil
	case "rename":
		newName := os.Getenv("zj_new")
		if err := zellij.ValidateName(newName); err != nil {
			return "", err
		}
		return mutated(fmt.Sprintf("Renamed %s → %s", session, newName), z.Rename(ctx, session, newName))
	default:
		return "", fmt.Errorf("unknown action %q", action)
	}
}

func deleteAllForce(ctx context.Context, z *zellij.CLI) error {
	if err := z.KillAll(ctx); err != nil && !strings.Contains(err.Error(), "No active") {
		return err
	}
	return z.DeleteAll(ctx)
}

func createBackground(ctx context.Context, z *zellij.CLI, cfg config, name, layout string) (string, error) {
	if err := z.CreateBackground(ctx, name, layout, cfg.defaultCwd); err != nil {
		return "", err
	}
	return "Created " + name + " in the background", nil
}

// reopenAlfred shows Alfred with query, e.g. to keep managing sessions.
func reopenAlfred(query string) {
	script := `on run argv
	tell application id "com.runningwithcrayons.Alfred" to search (item 1 of argv)
end run`
	if err := exec.Command("/usr/bin/osascript", "-e", script, query).Run(); err != nil {
		log.Printf("reopen alfred: %v", err)
	}
}

// spawnPreviews writes Quick Look previews in a detached process so the
// Script Filter returns without waiting for dump-screen calls.
func spawnPreviews() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(self, "previews") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err == nil {
		_ = cmd.Process.Release()
	}
}

func writePreviews(ctx context.Context, cfg config) error {
	z, err := newCLI(cfg)
	if err != nil {
		return err
	}
	cacheDir := alfred.CacheDir()
	infos, _, err := sessions.Load(ctx, z, cacheDir)
	if err != nil {
		return err
	}
	sessions.WritePreviews(ctx, z, cacheDir, infos)
	return nil
}

func describe(ctx context.Context, cfg config) error {
	z, err := newCLI(cfg)
	if err != nil {
		return err
	}
	infos, err := sessions.Gather(ctx, z, alfred.CacheDir())
	if err != nil {
		return err
	}
	for _, s := range infos {
		state := "running"
		if s.Exited {
			state = "exited"
		}
		fmt.Printf("session %-24s %-8s clients=%d tabs=%v age=%s preview=%s\n", s.Name, state, s.Clients, s.Tabs, s.AgeText, s.PreviewPath)
	}
	out, err := attach.Describe(ctx, sessions.RunningNames(infos))
	if err != nil {
		return err
	}
	fmt.Print("\nGhostty terminals (* = focused):\n" + out)
	return nil
}
