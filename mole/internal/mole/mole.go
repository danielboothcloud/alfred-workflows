// Package mole wraps the Mole CLI (https://mole.fit): finding the binary,
// listing uninstallable apps, and caching that list.
package mole

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// ExtraPath is prepended to PATH for Mole, which shells out to brew and
// other Homebrew tools. Alfred runs scripts with a minimal PATH.
const ExtraPath = "/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin"

// Find returns the path to the mo binary: explicit (if set), then PATH,
// then the Homebrew locations.
func Find(explicit string) (string, error) {
	if explicit != "" {
		if strings.HasPrefix(explicit, "~/") {
			home, _ := os.UserHomeDir()
			explicit = filepath.Join(home, explicit[2:])
		}
		if isExec(explicit) {
			return explicit, nil
		}
		return "", fmt.Errorf("mole_path %s is not executable", explicit)
	}
	for _, name := range []string{"mo", "mole"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, dir := range filepath.SplitList(ExtraPath) {
		for _, name := range []string{"mo", "mole"} {
			if p := filepath.Join(dir, name); isExec(p) {
				return p, nil
			}
		}
	}
	return "", errors.New("mole not found; install it with `brew install mole`")
}

func isExec(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// Env returns the environment for running Mole.
func Env() []string {
	env := os.Environ()
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + ExtraPath + ":" + kv[len("PATH="):]
			return env
		}
	}
	return append(env, "PATH="+ExtraPath+":/usr/bin:/bin:/usr/sbin:/sbin")
}

// App is one row of `mo uninstall --list`.
type App struct {
	Name          string `json:"name"`
	BundleID      string `json:"bundle_id"`
	Source        string `json:"source"` // "App" or "Homebrew"
	UninstallName string `json:"uninstall_name"`
	Path          string `json:"path"`
	Size          string `json:"size"` // e.g. "549.1MB", "--" when unknown
}

// Title is the name with invisible format characters (such as the
// left-to-right mark some PlayCover apps carry) removed.
func (a App) Title() string {
	t := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, a.Name))
	if t == "" {
		return a.Dir()
	}
	return t
}

// Dir is the bundle folder name without ".app".
func (a App) Dir() string {
	base := filepath.Base(a.Path)
	if ext := filepath.Ext(base); strings.EqualFold(ext, ".app") {
		base = base[:len(base)-len(ext)]
	}
	return base
}

// Parse decodes `mo uninstall --list` output, skipping any text printed
// before the JSON array.
func Parse(b []byte) ([]App, error) {
	start := -1
	if bytes.HasPrefix(b, []byte("[")) {
		start = 0
	} else if i := bytes.Index(b, []byte("\n[")); i >= 0 {
		start = i + 1
	}
	if start < 0 {
		return nil, errors.New("no JSON app list in mole output")
	}
	var apps []App
	if err := json.NewDecoder(bytes.NewReader(b[start:])).Decode(&apps); err != nil {
		return nil, fmt.Errorf("parse mole app list: %w", err)
	}
	return apps, nil
}

// Targets returns, for each app, the argument that makes
// `mo uninstall <arg>` select exactly that app, or "" when no such name
// exists.
//
// Mole lowercases the argument and walks its scan (the same order as
// --list), stopping at the first app whose display name or bundle folder
// name equals it; only when nothing is equal does it fall back to
// substring matching, which can select several apps. Homebrew cask tokens
// such as "alfred" (what --list reports as uninstall_name) are not names
// and so go through substring matching, so they are never used here.
func Targets(apps []App) []string {
	out := make([]string, len(apps))
	for i, a := range apps {
		for _, cand := range []string{a.Name, a.Dir()} {
			if strings.TrimSpace(cand) == "" || strings.HasPrefix(cand, "-") {
				continue
			}
			lc := strings.ToLower(cand)
			clash := false
			for _, b := range apps[:i] {
				if strings.ToLower(b.Name) == lc || strings.ToLower(b.Dir()) == lc {
					clash = true
					break
				}
			}
			if !clash {
				out[i] = cand
				break
			}
		}
	}
	return out
}

// SearchDirs are the folders whose modification time signals an app was
// installed or removed (Mole's own search roots plus common subfolders).
func SearchDirs(home string) []string {
	return []string{
		"/Applications",
		"/Applications/Utilities",
		filepath.Join(home, "Applications"),
		filepath.Join(home, "Applications", "PlayCover"),
		"/Library/Input Methods",
		filepath.Join(home, "Library", "Input Methods"),
	}
}

// Stale reports whether a list fetched at fetched is out of date: older
// than maxAge, or older than a change to any of dirs.
func Stale(fetched time.Time, dirs []string, maxAge time.Duration, now time.Time) bool {
	if now.Sub(fetched) > maxAge {
		return true
	}
	for _, d := range dirs {
		if fi, err := os.Stat(d); err == nil && fi.ModTime().After(fetched) {
			return true
		}
	}
	return false
}

// Cache stores the app list in Dir.
type Cache struct {
	Dir string
}

const (
	listFile  = "apps.json"
	errFile   = "refresh.err"
	lockFile  = "refresh.lock"
	lockTTL   = 5 * time.Minute
	listLimit = 4 * time.Minute
)

// ErrBusy means another refresh is running.
var ErrBusy = errors.New("refresh already running")

func (c Cache) path(name string) string { return filepath.Join(c.Dir, name) }

// Load returns the cached list and when it was fetched.
func (c Cache) Load() ([]App, time.Time, error) {
	f := c.path(listFile)
	fi, err := os.Stat(f)
	if err != nil {
		return nil, time.Time{}, err
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, time.Time{}, err
	}
	var apps []App
	if err := json.Unmarshal(b, &apps); err != nil {
		return nil, time.Time{}, err
	}
	return apps, fi.ModTime(), nil
}

// Refreshing reports whether a refresh holds the lock.
func (c Cache) Refreshing() bool {
	fi, err := os.Stat(c.path(lockFile))
	return err == nil && time.Since(fi.ModTime()) < lockTTL
}

// LastError returns the error of the last failed refresh and when it
// happened, or "" if the last refresh succeeded.
func (c Cache) LastError() (string, time.Time) {
	f := c.path(errFile)
	fi, err := os.Stat(f)
	if err != nil {
		return "", time.Time{}
	}
	b, _ := os.ReadFile(f)
	return strings.TrimSpace(string(b)), fi.ModTime()
}

func (c Cache) lock() (func(), error) {
	f := c.path(lockFile)
	for attempt := 0; attempt < 2; attempt++ {
		h, err := os.OpenFile(f, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(h, "%d\n", os.Getpid())
			h.Close()
			return func() { os.Remove(f) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if fi, serr := os.Stat(f); serr == nil && time.Since(fi.ModTime()) < lockTTL {
			return nil, ErrBusy
		}
		os.Remove(f) // abandoned by a crashed refresh
	}
	return nil, ErrBusy
}

// Refresh runs `mo uninstall --list` and replaces the cached list. Failures
// are recorded for LastError.
func (c Cache) Refresh(ctx context.Context, bin string) ([]App, error) {
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return nil, err
	}
	unlock, err := c.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	apps, err := list(ctx, bin)
	if err == nil {
		err = c.save(apps)
	}
	if err != nil {
		_ = os.WriteFile(c.path(errFile), []byte(err.Error()+"\n"), 0o600)
		return nil, err
	}
	os.Remove(c.path(errFile))
	return apps, nil
}

func list(ctx context.Context, bin string) ([]App, error) {
	ctx, cancel := context.WithTimeout(ctx, listLimit)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "uninstall", "--list") // stdout is a pipe, so Mole prints JSON
	cmd.Env = Env()
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("mo uninstall --list: %s", msg)
	}
	return Parse(stdout.Bytes())
}

func (c Cache) save(apps []App) error {
	if apps == nil {
		apps = []App{}
	}
	b, err := json.Marshal(apps)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.Dir, listFile+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), c.path(listFile))
}
