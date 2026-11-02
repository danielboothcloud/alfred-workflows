// Command alfred-apps is an Alfred Script Filter that finds and launches
// applications by walking the filesystem instead of relying on Spotlight.
//
//	alfred-apps filter [query]   Script Filter JSON
//	alfred-apps list             print the index (name<TAB>path)
//	alfred-apps reindex          rebuild the index now
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"alfred-apps/internal/alfred"
	"alfred-apps/internal/apps"
	"alfred-apps/internal/fuzzy"
)

var version = "dev"

func main() {
	log.SetFlags(0)
	cmd, args := "filter", os.Args[1:]
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "filter":
		err = filter(strings.Join(args, " "))
	case "list":
		err = list(false)
	case "reindex":
		err = list(true)
	case "version":
		fmt.Println(version)
	default:
		err = fmt.Errorf("usage: alfred-apps [filter <query> | list | reindex | version]")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func load(force bool) ([]apps.App, error) {
	file := filepath.Join(alfred.CacheDir(), "index.json")
	return apps.Load(file, roots(), apps.Extras, force)
}

// roots returns the default search folders plus any from the extra_dirs
// workflow variable (separated by colons or new lines, ~ allowed).
func roots() []string {
	r := apps.DefaultRoots()
	home, _ := os.UserHomeDir()
	for _, d := range strings.FieldsFunc(os.Getenv("extra_dirs"), func(c rune) bool {
		return c == ':' || c == '\n'
	}) {
		d = strings.TrimSpace(d)
		if d == "~" || strings.HasPrefix(d, "~/") {
			d = home + d[1:]
		}
		if d != "" {
			r = append(r, d)
		}
	}
	return r
}

func filter(query string) error {
	list, err := load(false)
	fb := alfred.Feedback{}
	if err != nil && len(list) == 0 {
		fb.Items = append(fb.Items, alfred.Item{Title: "Could not index apps", Subtitle: err.Error()})
		return fb.Write(os.Stdout)
	}
	for _, i := range match(query, list) {
		fb.Items = append(fb.Items, item(list[i]))
	}
	if len(fb.Items) == 0 {
		fb.Items = append(fb.Items, alfred.Item{
			Title:    fmt.Sprintf("No apps match “%s”", query),
			Subtitle: "Searched " + strings.Join(tildify(roots()), ", "),
		})
	}
	return fb.Write(os.Stdout)
}

// match ranks apps by their display name, also accepting the bundle file
// name (e.g. "VoiceMemos") when it differs.
func match(query string, list []apps.App) []int {
	if strings.TrimSpace(query) == "" {
		idx := make([]int, len(list))
		for i := range idx {
			idx[i] = i
		}
		return idx
	}
	names := make([]string, 0, len(list)*2)
	owner := make([]int, 0, len(list)*2)
	for i, a := range list {
		names, owner = append(names, a.Name), append(owner, i)
		if !strings.EqualFold(a.File, a.Name) {
			names, owner = append(names, a.File), append(owner, i)
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

func item(a apps.App) alfred.Item {
	return alfred.Item{
		UID:          a.Path,
		Type:         "file:skipcheck",
		Title:        a.Name,
		Subtitle:     tilde(filepath.Dir(a.Path)),
		Arg:          a.Path,
		Valid:        true,
		Autocomplete: a.Name,
		Icon:         &alfred.Icon{Type: "fileicon", Path: a.Path},
		QuickLookURL: a.Path,
		Text:         &alfred.Text{Copy: a.Path, LargeType: a.Name},
		Mods: map[string]alfred.Mod{
			"cmd": {Valid: true, Arg: a.Path, Subtitle: "Reveal in Finder", Variables: map[string]string{"action": "reveal"}},
		},
		Variables: map[string]string{"action": "open"},
	}
}

func list(force bool) error {
	l, err := load(force)
	if err != nil {
		return err
	}
	for _, a := range l {
		fmt.Printf("%s\t%s\n", a.Name, a.Path)
	}
	if force {
		fmt.Fprintf(os.Stderr, "indexed %d apps\n", len(l))
	}
	return nil
}

func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + p[len(home):]
	}
	return p
}

func tildify(ps []string) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = tilde(p)
	}
	return out
}
