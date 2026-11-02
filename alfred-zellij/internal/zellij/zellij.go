// Package zellij wraps the zellij CLI.
package zellij

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Session is one entry from `zellij list-sessions`.
type Session struct {
	Name    string
	Age     time.Duration // time since creation
	AgeText string        // compact age, e.g. "6d 21h"
	Exited  bool          // exited sessions can be resurrected by attaching
}

// Tab is one entry from `zellij action list-tabs --json`.
type Tab struct {
	Position int    `json:"position"`
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	TabID    int    `json:"tab_id"`
}

// Pane is one entry from `zellij action list-panes --json`.
type Pane struct {
	ID         int    `json:"id"`
	IsPlugin   bool   `json:"is_plugin"`
	IsFocused  bool   `json:"is_focused"`
	IsFloating bool   `json:"is_floating"`
	Title      string `json:"title"`
	Exited     bool   `json:"exited"`
	TabID      int    `json:"tab_id"`
	TabName    string `json:"tab_name"`
}

// CLI runs zellij commands.
type CLI struct {
	Bin     string
	Timeout time.Duration
}

// FindBinary resolves the zellij executable. Alfred runs workflows with a
// minimal PATH, so common install locations are checked too.
func FindBinary(override string) (string, error) {
	if override != "" {
		p := expandHome(override)
		if isExecutable(p) {
			return p, nil
		}
		return "", fmt.Errorf("zellij_path %q is not executable", override)
	}
	if p, err := exec.LookPath("zellij"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/opt/homebrew/bin/zellij",
		"/usr/local/bin/zellij",
		filepath.Join(home, ".cargo/bin/zellij"),
		filepath.Join(home, ".local/bin/zellij"),
		filepath.Join(home, ".local/share/mise/shims/zellij"),
		filepath.Join(home, ".nix-profile/bin/zellij"),
		"/run/current-system/sw/bin/zellij",
	}
	for _, c := range candidates {
		if isExecutable(c) {
			return c, nil
		}
	}
	return "", errors.New("zellij not found; set the zellij_path workflow variable")
}

func isExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

// ExpandHome expands a leading ~ in p.
func ExpandHome(p string) string { return expandHome(p) }

// env returns the process environment without zellij session variables, so
// commands behave the same from Alfred and from inside a zellij pane.
func env() []string {
	var out []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "ZELLIJ=") || strings.HasPrefix(kv, "ZELLIJ_SESSION_NAME=") ||
			strings.HasPrefix(kv, "ZELLIJ_PANE_ID=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func (c *CLI) run(ctx context.Context, args ...string) (string, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// c.Bin comes from FindBinary (known install paths or the user's
	// zellij_path setting); args are passed as argv, never through a shell.
	cmd := exec.CommandContext(ctx, c.Bin, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.Env = env()
	cmd.Stdin = nil
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(StripANSI(stderr.String()))
		if msg == "" {
			msg = strings.TrimSpace(StripANSI(stdout.String()))
		}
		if msg != "" {
			return stdout.String(), fmt.Errorf("zellij %s: %s", strings.Join(args, " "), firstLine(msg))
		}
		return stdout.String(), fmt.Errorf("zellij %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)`)

// StripANSI removes terminal escape sequences.
func StripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// ListSessions returns running sessions first, then exited ones; each group
// is sorted newest first.
func (c *CLI) ListSessions(ctx context.Context) ([]Session, error) {
	out, err := c.run(ctx, "list-sessions", "--no-formatting")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no active zellij sessions") {
			return nil, nil
		}
		return nil, err
	}
	sessions := ParseSessions(out)
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].Exited != sessions[j].Exited {
			return !sessions[i].Exited
		}
		return sessions[i].Age < sessions[j].Age
	})
	return sessions, nil
}

var sessionLineRe = regexp.MustCompile(`^(.+?) \[Created (.*?) ago\](.*)$`)

// ParseSessions parses `zellij list-sessions --no-formatting` output.
func ParseSessions(out string) []Session {
	var sessions []Session
	for _, line := range strings.Split(StripANSI(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "No active zellij sessions") {
			continue
		}
		m := sessionLineRe.FindStringSubmatch(line)
		if m == nil {
			// Unknown format: keep the name so the session is still usable.
			sessions = append(sessions, Session{Name: line})
			continue
		}
		age := ParseAge(m[2])
		sessions = append(sessions, Session{
			Name:    m[1],
			Age:     age,
			AgeText: CompactAge(m[2]),
			Exited:  strings.Contains(m[3], "EXITED"),
		})
	}
	return sessions
}

var ageTokenRe = regexp.MustCompile(`(\d+)\s*([a-z]+)`)

// ParseAge parses zellij's "6days 21h 20m 41s" format.
func ParseAge(s string) time.Duration {
	var d time.Duration
	for _, m := range ageTokenRe.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		switch {
		case strings.HasPrefix(m[2], "d"):
			d += time.Duration(n) * 24 * time.Hour
		case strings.HasPrefix(m[2], "h"):
			d += time.Duration(n) * time.Hour
		case strings.HasPrefix(m[2], "m"):
			d += time.Duration(n) * time.Minute
		case strings.HasPrefix(m[2], "s"):
			d += time.Duration(n) * time.Second
		}
	}
	return d
}

// CompactAge keeps the two most significant units: "6days 21h 20m 41s" -> "6d 21h".
func CompactAge(s string) string {
	var parts []string
	for _, m := range ageTokenRe.FindAllStringSubmatch(s, -1) {
		parts = append(parts, m[1]+m[2][:1])
		if len(parts) == 2 {
			break
		}
	}
	return strings.Join(parts, " ")
}

// ListClients returns the number of clients attached to a running session.
func (c *CLI) ListClients(ctx context.Context, session string) (int, error) {
	out, err := c.run(ctx, "--session", session, "action", "list-clients")
	if err != nil {
		return 0, err
	}
	out = StripANSI(out)
	if !strings.HasPrefix(strings.TrimSpace(out), "CLIENT_ID") {
		return 0, fmt.Errorf("session %q not found", session)
	}
	n := 0
	for _, line := range strings.Split(out, "\n")[1:] {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n, nil
}

// ListTabs returns the tabs of a running session.
func (c *CLI) ListTabs(ctx context.Context, session string) ([]Tab, error) {
	out, err := c.run(ctx, "--session", session, "action", "list-tabs", "--json")
	if err != nil {
		return nil, err
	}
	var tabs []Tab
	if err := json.Unmarshal([]byte(out), &tabs); err != nil {
		return nil, fmt.Errorf("parse tabs: %w", err)
	}
	sort.SliceStable(tabs, func(i, j int) bool { return tabs[i].Position < tabs[j].Position })
	return tabs, nil
}

// ListPanes returns the panes of a running session.
func (c *CLI) ListPanes(ctx context.Context, session string) ([]Pane, error) {
	out, err := c.run(ctx, "--session", session, "action", "list-panes", "--json")
	if err != nil {
		return nil, err
	}
	var panes []Pane
	if err := json.Unmarshal([]byte(out), &panes); err != nil {
		return nil, fmt.Errorf("parse panes: %w", err)
	}
	return panes, nil
}

// PreviewPane picks the terminal pane a user most likely wants to see: the
// focused pane of the active tab, mirroring the shell session picker.
func PreviewPane(tabs []Tab, panes []Pane) (Pane, bool) {
	activeTab := -1
	for _, t := range tabs {
		if t.Active {
			activeTab = t.TabID
			break
		}
	}
	best, found := Pane{}, false
	score := func(p Pane) int {
		s := 0
		if p.TabID == activeTab {
			s += 2
		}
		if p.IsFocused {
			s++
		}
		return s
	}
	for _, p := range panes {
		if p.IsPlugin || p.Exited {
			continue
		}
		if !found || score(p) > score(best) {
			best, found = p, true
		}
	}
	return best, found
}

// DumpScreen returns the visible text of a terminal pane.
func (c *CLI) DumpScreen(ctx context.Context, session string, paneID int) (string, error) {
	return c.run(ctx, "--session", session, "action", "dump-screen", "--pane-id", fmt.Sprintf("terminal_%d", paneID))
}

// CreateBackground starts a detached session in cwd.
//
// The new server inherits this process's environment, and Alfred runs
// workflows with PATH=/usr/bin:/bin:/usr/sbin:/sbin. Plugins that run
// commands (e.g. zjstatus) would then fail, so PATH is extended with the
// usual user install locations.
func (c *CLI) CreateBackground(ctx context.Context, name, layout, cwd string) error {
	args := []string{"attach", "--create-background", name}
	if layout != "" {
		args = append(args, "options", "--default-layout", layout)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.Dir = cwd
	cmd.Env = ServerEnv(c.Bin)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create %s: %s", name, strings.TrimSpace(StripANSI(string(out))))
	}
	return nil
}

// ServerEnv is env() with PATH extended by common user bin directories.
func ServerEnv(bin string) []string {
	home, _ := os.UserHomeDir()
	extra := []string{
		filepath.Dir(bin),
		filepath.Join(home, ".local/share/mise/shims"),
		filepath.Join(home, ".local/bin"),
		filepath.Join(home, ".cargo/bin"),
		filepath.Join(home, ".nix-profile/bin"),
		"/opt/homebrew/bin", "/opt/homebrew/sbin",
		"/usr/local/bin", "/usr/local/sbin",
		"/usr/bin", "/bin", "/usr/sbin", "/sbin",
	}
	var out []string
	for _, kv := range env() {
		if !strings.HasPrefix(kv, "PATH=") {
			out = append(out, kv)
		}
	}
	return append(out, "PATH="+MergePath(os.Getenv("PATH"), extra))
}

// MergePath appends dirs missing from path, keeping path's order first.
func MergePath(path string, dirs []string) string {
	seen := map[string]bool{}
	var parts []string
	for _, d := range append(filepath.SplitList(path), dirs...) {
		if d != "" && !seen[d] {
			seen[d] = true
			parts = append(parts, d)
		}
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

// Kill stops a running session. With session serialization enabled it stays
// listed as exited and can be resurrected.
func (c *CLI) Kill(ctx context.Context, session string) error {
	_, err := c.run(ctx, "kill-session", session)
	return err
}

// Delete removes a session. force kills it first if it is running.
func (c *CLI) Delete(ctx context.Context, session string, force bool) error {
	args := []string{"delete-session"}
	if force {
		args = append(args, "--force")
	}
	_, err := c.run(ctx, append(args, session)...)
	return err
}

// KillAll kills every running session.
func (c *CLI) KillAll(ctx context.Context) error {
	_, err := c.run(ctx, "kill-all-sessions", "--yes")
	return err
}

// DeleteAll deletes every exited session.
func (c *CLI) DeleteAll(ctx context.Context) error {
	_, err := c.run(ctx, "delete-all-sessions", "--yes")
	return err
}

// Rename renames a running session.
func (c *CLI) Rename(ctx context.Context, session, newName string) error {
	_, err := c.run(ctx, "--session", session, "action", "rename-session", newName)
	return err
}

// SwitchOptions apply when the target session has to be created.
type SwitchOptions struct {
	Layout string
	Cwd    string
}

// SwitchSession moves a client of host to target. The client re-attaches
// with `attach --create`, so target may be running, exited, or new.
//
// zellij applies CLI actions to the host's last active client (the last one
// to send a keystroke). If no client has typed since joining host, nothing
// switches and no error is returned, so callers must verify the result.
func (c *CLI) SwitchSession(ctx context.Context, host, target string, opts SwitchOptions) error {
	args := []string{"--session", host, "action", "switch-session"}
	if opts.Layout != "" {
		args = append(args, "--layout", opts.Layout)
	}
	if opts.Cwd != "" {
		args = append(args, "--cwd", opts.Cwd)
	}
	_, err := c.run(ctx, append(args, target)...)
	return err
}

// AttachCommand returns a shell command that attaches to target in a new
// terminal, creating it (optionally with a layout) when missing.
func (c *CLI) AttachCommand(target, layout string, exists bool) string {
	if !exists && layout != "" {
		return ShellJoin(c.Bin, "--new-session-with-layout", layout, "--session", target)
	}
	return ShellJoin(c.Bin, "attach", "--create", target)
}

// ShellJoin quotes args for a POSIX shell.
func ShellJoin(args ...string) string {
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

// BuiltinLayouts ship with zellij.
var BuiltinLayouts = []string{"default", "compact", "classic", "strider", "disable-status-bar"}

// Layouts returns built-in layouts plus *.kdl files in the layout directory.
func Layouts(layoutDir string) []string {
	if layoutDir == "" {
		layoutDir = filepath.Join(ConfigDir(), "layouts")
	}
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	entries, _ := os.ReadDir(expandHome(layoutDir))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".kdl") && !strings.HasSuffix(e.Name(), ".swap.kdl") {
			add(strings.TrimSuffix(e.Name(), ".kdl"))
		}
	}
	for _, l := range BuiltinLayouts {
		add(l)
	}
	return out
}

// ConfigDir returns the zellij config directory.
func ConfigDir() string {
	if d := os.Getenv("ZELLIJ_CONFIG_DIR"); d != "" {
		return expandHome(d)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "zellij")
}

// ValidateName rejects names zellij cannot use.
func ValidateName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("session name is empty")
	case strings.ContainsAny(name, "/\n\t"):
		return errors.New("session name cannot contain '/', tabs or newlines")
	case len(name) > 60:
		return errors.New("session name is too long")
	}
	return nil
}
